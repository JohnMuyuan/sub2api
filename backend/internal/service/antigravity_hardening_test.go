package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsAntigravityRetryableTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled", context.Canceled, false},
		{"dial op error", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"proxyconnect op error", &net.OpError{Op: "proxyconnect", Err: errors.New("x")}, true},
		{"read op error (request may be sent)", &net.OpError{Op: "read", Err: errors.New("reset")}, false},
		{"dns error", &net.DNSError{Err: "no such host", Name: "x"}, true},
		{"wrapped dial string", fmt.Errorf("upstream: %v", "dial tcp 1.2.3.4:443: connect: connection refused"), true},
		{"tls handshake timeout", errors.New("net/http: TLS handshake timeout"), true},
		{"response header timeout", errors.New("net/http: timeout awaiting response headers"), false},
		{"unexpected EOF", io.ErrUnexpectedEOF, false},
		{"connection reset mid-response", errors.New("read tcp: connection reset by peer"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isAntigravityRetryableTransportError(tt.err))
		})
	}
}

func newHardeningAccount() *Account {
	return &Account{ID: 4242, Name: "hardening", Platform: PlatformAntigravity, Type: AccountTypeOAuth, Concurrency: 1}
}

func runHardeningRetryLoop(t *testing.T, upstream HTTPUpstream) (*antigravityRetryLoopResult, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	svc := &AntigravityGatewayService{}
	return svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:          context.Background(),
		prefix:       "[test]",
		account:      newHardeningAccount(),
		accessToken:  "token",
		action:       "streamGenerateContent",
		body:         []byte(`{"model":"claude-sonnet-4-5"}`),
		c:            c,
		httpUpstream: upstream,
		handleError: func(context.Context, string, *Account, int, http.Header, []byte, string, int64, string, bool) *handleModelRateLimitResult {
			return nil
		},
	})
}

// 回归：响应阶段的网络错误（请求可能已到达上游）不得原样重发生成请求。
func TestAntigravityRetryLoop_DoesNotResendAfterPossiblyDeliveredRequest(t *testing.T) {
	upstream := &queuedHTTPUpstreamStub{errors: []error{
		errors.New("read tcp 10.0.0.1:1234->1.2.3.4:443: connection reset by peer"),
	}}
	_, err := runHardeningRetryLoop(t, upstream)
	require.Error(t, err)
	require.Equal(t, 1, upstream.callCount, "non-idempotent generation request must not be resent")
}

func TestAntigravityRetryLoop_RetriesDialFailure(t *testing.T) {
	upstream := &queuedHTTPUpstreamStub{
		errors: []error{&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, nil},
		responses: []*http.Response{nil, {
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("ok")),
		}},
	}
	result, err := runHardeningRetryLoop(t, upstream)
	require.NoError(t, err)
	require.NotNil(t, result.resp)
	defer func() { _ = result.resp.Body.Close() }()
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Equal(t, 2, upstream.callCount)
}

func resetModelCapacityState(t *testing.T) {
	t.Helper()
	modelCapacityExhaustedMu.Lock()
	modelCapacityExhaustedUntil = make(map[string]time.Time)
	modelCapacityRetryInFlight = make(map[string]struct{})
	modelCapacityExhaustedMu.Unlock()
}

func TestModelCapacityRetry_SingleFlight(t *testing.T) {
	resetModelCapacityState(t)
	defer resetModelCapacityState(t)

	const workers = 64
	var winners int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if acquireModelCapacityRetry("gemini-3-pro-high", time.Now()) {
				atomic.AddInt32(&winners, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	require.Equal(t, int32(1), winners, "exactly one request may run the capacity retry loop")

	// 不同模型互不影响
	require.True(t, acquireModelCapacityRetry("gemini-2.5-flash", time.Now()))
	releaseModelCapacityRetry("gemini-2.5-flash", false, time.Now())
}

func TestModelCapacityRetry_CooldownLifecycle(t *testing.T) {
	resetModelCapacityState(t)
	defer resetModelCapacityState(t)
	now := time.Now()
	const model = "m"

	require.True(t, acquireModelCapacityRetry(model, now))
	releaseModelCapacityRetry(model, true, now)
	require.False(t, acquireModelCapacityRetry(model, now.Add(time.Second)), "cooldown must block new retries")
	require.True(t, acquireModelCapacityRetry(model, now.Add(antigravityModelCapacityCooldown+time.Millisecond)), "cooldown must expire")
	releaseModelCapacityRetry(model, false, now)
	require.True(t, acquireModelCapacityRetry(model, now), "success clears cooldown and in-flight flag")
	releaseModelCapacityRetry(model, false, now)
}

func TestModelCapacityRetry_BackoffIsBounded(t *testing.T) {
	wait := antigravityModelCapacityRetryWait
	total := time.Duration(0)
	for i := 0; i < antigravityModelCapacityRetryMaxAttempts; i++ {
		total += wait
		wait = nextModelCapacityRetryWait(wait)
		require.LessOrEqual(t, wait, antigravityModelCapacityRetryMaxWait)
	}
	require.LessOrEqual(t, antigravityModelCapacityRetryMaxAttempts, 10, "capacity retries must stay bounded")
	require.LessOrEqual(t, total, 60*time.Second)
}

func TestIsNonRetryableAntigravityOAuthError_TypedStatus(t *testing.T) {
	require.True(t, isNonRetryableAntigravityOAuthError(&antigravity.TokenEndpointError{Op: "x", StatusCode: 400, Body: "{}"}))
	require.True(t, isNonRetryableAntigravityOAuthError(fmt.Errorf("wrap: %w", &antigravity.TokenEndpointError{Op: "x", StatusCode: 401})))
	require.False(t, isNonRetryableAntigravityOAuthError(&antigravity.TokenEndpointError{Op: "x", StatusCode: 503}))
	require.False(t, isNonRetryableAntigravityOAuthError(&antigravity.TokenEndpointError{Op: "x", StatusCode: 429}))
	require.True(t, isNonRetryableAntigravityOAuthError(errors.New("token 刷新失败 (HTTP 400): invalid_grant")))
}

func TestAntigravityOAuthRefreshToken_RespectsContextDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	// RefreshToken/loadProjectIDWithRetry 的退避等待均使用 sleepWithContext；
	// 直接调用真实 RefreshToken 会访问 Google，因此这里验证等待在 ctx 结束时立即返回。
	err := sleepWithContext(ctx, 10*time.Second)
	require.Error(t, err)
	require.Less(t, time.Since(start), 2*time.Second)
}

func TestAntigravitySessionScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	acc := &Account{ID: 7}
	require.Equal(t, "", antigravitySessionScope(nil, nil))
	require.Equal(t, "account:7", antigravitySessionScope(nil, acc))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{ID: 9})
	require.Equal(t, "account:7|api_key:9", antigravitySessionScope(c, acc))

	ctx := withAntigravitySessionScope(context.Background(), "account:7|api_key:9")
	require.Equal(t, "account:7|api_key:9", antigravitySessionScopeFromContext(ctx))
	opts := (&AntigravityGatewayService{}).getClaudeTransformOptions(ctx)
	require.Equal(t, "account:7|api_key:9", opts.SessionScope)
}

// 回归：下游断开（context canceled）时不应被记为请求级上游错误，也不应重试。
func TestAntigravityRetryLoop_ClientCancelIsNotUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(context.Background())
	upstream := &queuedHTTPUpstreamStub{}
	upstream.onCall = func(*http.Request, *queuedHTTPUpstreamStub) { cancel() }
	upstream.errors = []error{fmt.Errorf("Post \"https://example.invalid\": %w", context.Canceled)}

	svc := &AntigravityGatewayService{}
	_, err := svc.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:          ctx,
		prefix:       "[test]",
		account:      newHardeningAccount(),
		accessToken:  "token",
		action:       "streamGenerateContent",
		body:         []byte(`{}`),
		c:            c,
		httpUpstream: upstream,
		handleError: func(context.Context, string, *Account, int, http.Header, []byte, string, int64, string, bool) *handleModelRateLimitResult {
			return nil
		},
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, upstream.callCount)
	_, attributed := c.Get(OpsUpstreamErrorMessageKey)
	require.False(t, attributed, "client cancellation must not be attributed as a request-level upstream error")
}
