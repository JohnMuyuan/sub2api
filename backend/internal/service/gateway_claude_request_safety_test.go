//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type claudeSafetyUpstream struct {
	HTTPUpstream
	calls int
	send  func(*http.Request, *tlsfingerprint.Profile) (*http.Response, error)
}

func (u *claudeSafetyUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	if u.send != nil {
		return u.send(req, profile)
	}
	return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func claudeSafetyFixture(t *testing.T) (*GatewayService, *claudeSafetyUpstream, *gin.Context, *Account) {
	t.Helper()
	u := &claudeSafetyUpstream{}
	s := &GatewayService{httpUpstream: u}
	c := newTransportErrorTestGin(t)
	return s, u, c, &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
}

func claudeSafetyRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{"stream":true}`))
	setHeaderRaw(req.Header, "x-stainless-retry-count", "99")
	return req
}

func assertClaudeProtection(t *testing.T, err error, stop bool) {
	t.Helper()
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, claudeRetryProtectionReason, failover.Reason)
	require.Equal(t, !stop, failover.ShouldRetryNextAccount())
	require.False(t, failover.ShouldReportAccountScheduleFailure())
	require.Same(t, failover, (&GatewayService{}).handleUpstreamTransportError(context.Background(), nil, nil, err, OpsUpstreamErrorEvent{}))
}

func TestClaudeSafetySharedBudgetAndRetryHeaders(t *testing.T) {
	s, u, c, account := claudeSafetyFixture(t)
	profile := &tlsfingerprint.Profile{}
	otherProfile := &tlsfingerprint.Profile{}
	var counts []string
	var profiles []*tlsfingerprint.Profile
	u.send = func(req *http.Request, p *tlsfingerprint.Profile) (*http.Response, error) {
		counts = append(counts, getHeaderRaw(req.Header, "x-stainless-retry-count"))
		profiles = append(profiles, p)
		return &http.Response{StatusCode: 400, Header: make(http.Header), Body: http.NoBody}, nil
	}
	for i := 0; i < maxRetryAttempts; i++ {
		p := otherProfile
		if i == 0 {
			p = profile
		}
		// Simulate both rectifiers and handler failover entering the same helper.
		if i == 3 {
			account = &Account{ID: 43, Platform: PlatformAnthropic, Type: AccountTypeSetupToken}
		}
		resp, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", p)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.Equal(t, []string{"0", "1", "2", "0", "1"}, counts)
	require.Same(t, profile, profiles[2], "pin the first TLS profile for this account during one client request")
	_, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
	assertClaudeProtection(t, err, true)
	require.Equal(t, maxRetryAttempts, u.calls)
}

func TestClaudeSafetyRejectedAccountNotReplayed(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s, u, c, account := claudeSafetyFixture(t)
			u.send = func(*http.Request, *tlsfingerprint.Profile) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: http.NoBody}, nil
			}
			require.False(t, s.shouldRetryUpstreamError(account, status))
			resp, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
			require.NoError(t, err)
			require.Equal(t, status, resp.StatusCode)
			_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
			assertClaudeProtection(t, err, false)
			require.Equal(t, 1, u.calls)
			account.ID++
			_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
			require.NoError(t, err)
			require.Equal(t, 2, u.calls)
		})
	}
}

func TestClaudeSafetyRetryAfter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	require.Equal(t, now.Add(5*time.Second), claudeRetryAfter("5", now))
	require.Equal(t, now.Add(5*time.Second), claudeRetryAfter(now.Add(5*time.Second).Format(http.TimeFormat), now))
	require.Equal(t, now, claudeRetryAfter("0", now))
	for _, value := range []string{"NaN", "-1", "1.5", ""} {
		require.Equal(t, now.Add(time.Second), claudeRetryAfter(value, now))
	}
	require.True(t, claudeRetryAfter("99999999999999999999999999", now).After(now.Add(maxRetryElapsed)))
	for _, status := range []int{429, 503, 529} {
		s, u, c, account := claudeSafetyFixture(t)
		u.send = func(*http.Request, *tlsfingerprint.Profile) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"120"}}, Body: http.NoBody}, nil
		}
		_, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
		require.NoError(t, err)
		_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
		assertClaudeProtection(t, err, false)
		require.Equal(t, 1, u.calls, "must not shorten Retry-After to fit the local budget")
	}
}

func TestClaudeSafetyWaitAndCancellation(t *testing.T) {
	s, u, c, account := claudeSafetyFixture(t)
	_, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
	require.NoError(t, err)
	state := claudeSafety(c)
	state.accounts[account.ID].notBefore = time.Now().Add(20 * time.Millisecond)
	start := time.Now()
	_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
	require.NoError(t, err)
	require.False(t, time.Now().Before(state.accounts[account.ID].notBefore))
	require.Less(t, time.Since(start), time.Second)
	state.accounts[account.ID].notBefore = time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()
	_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, u.calls)
}

func TestClaudeSafetyStreamAndElapsedBudget(t *testing.T) {
	for _, stream := range []bool{true, false} {
		s, u, c, account := claudeSafetyFixture(t)
		// Existing queue keepalive must not block the FIRST upstream request.
		_, err := c.Writer.Write([]byte(": keepalive\n\n"))
		require.NoError(t, err)
		_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
		require.NoError(t, err)
		if stream {
			_, err = c.Writer.Write([]byte("data: message\n\n"))
			require.NoError(t, err)
		} else {
			claudeSafety(c).started = time.Now().Add(-maxRetryElapsed)
		}
		_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
		assertClaudeProtection(t, err, true)
		require.Equal(t, 1, u.calls)
	}
	s, u, c, account := claudeSafetyFixture(t)
	u.send = func(req *http.Request, _ *tlsfingerprint.Profile) (*http.Response, error) {
		claudeSafety(c).started = time.Now().Add(-time.Minute)
		require.NoError(t, req.Context().Err(), "retry deadline must not cancel a healthy long response")
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}
	_, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
	require.NoError(t, err)
}

func TestClaudeSafetyTransportReplayDecision(t *testing.T) {
	for _, stage := range []string{"unknown", "preflight", "written", "redirect_then_dial_failure", "canceled"} {
		t.Run(stage, func(t *testing.T) {
			s, u, c, account := claudeSafetyFixture(t)
			original := errors.New("transport unavailable")
			u.send = func(req *http.Request, _ *tlsfingerprint.Profile) (*http.Response, error) {
				trace := httptrace.ContextClientTrace(req.Context())
				switch stage {
				case "preflight":
					trace.ConnectDone("tcp", "example:443", original)
				case "written":
					trace.WroteRequest(httptrace.WroteRequestInfo{})
				case "redirect_then_dial_failure":
					trace.GotFirstResponseByte()
					trace.ConnectDone("tcp", "redirect:443", original)
				case "canceled":
					return nil, context.Canceled
				}
				return nil, original
			}
			_, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
			if stage == "preflight" {
				require.ErrorIs(t, err, original)
			} else if stage == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				assertClaudeProtection(t, err, true)
				account.ID++
				_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
				assertClaudeProtection(t, err, true)
				require.Equal(t, 1, u.calls)
			}
		})
	}
}

func TestClaudeSafetyAPIKeyUnchanged(t *testing.T) {
	s, u, c, account := claudeSafetyFixture(t)
	account.Type = AccountTypeAPIKey
	u.send = func(req *http.Request, _ *tlsfingerprint.Profile) (*http.Response, error) {
		require.Equal(t, "99", getHeaderRaw(req.Header, "x-stainless-retry-count"))
		return nil, io.EOF
	}
	for i := 0; i < 6; i++ {
		_, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
		require.ErrorIs(t, err, io.EOF)
	}
	require.Equal(t, 6, u.calls)
	_, exists := c.Get(claudeRequestSafetyKey)
	require.False(t, exists)
}

func TestClaudeConsistencyDiagnosticsRedactedAndReadOnly(t *testing.T) {
	_, _, c, account := claudeSafetyFixture(t)
	req := claudeSafetyRequest()
	req.Header.Set("User-Agent", "claude-cli/2.1.100 (external, cli)")
	req.Header.Set("X-Claude-Code-Session-Id", "private-session-header")
	req.Header.Set("Authorization", "Bearer private-token")
	req.Header.Set("X-Api-Key", "private-api-key")
	req.Header.Set("X-Stainless-Helper-Method", "stream")
	req.Header["user-agent"] = []string{"private-duplicate"}
	body := []byte(`{"stream":false,"metadata":{"user_id":"{\"device_id\":\"private-device\",\"session_id\":\"private-session-body\"}"},"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.99; private-prompt"}]}`)
	headersBefore := req.Header.Clone()
	bodyBefore := string(body)
	issues := claudeConsistencyIssues(req, body)
	require.ElementsMatch(t, []string{"session_id_mismatch", "cli_billing_version_mismatch", "stream_helper_mismatch", "multiple_auth_methods", "duplicate_user-agent"}, issues)
	diagnoseClaudeRequest(c, req, body, account)
	diagnoseClaudeRequest(c, req, body, account)
	require.Len(t, claudeSafety(c).reported, len(issues))
	debug := buildClaudeMimicDebugLine(req, body, &Account{ID: 42, Name: "private-account"}, "oauth", true)
	require.NotContains(t, debug, "private-")
	require.NotContains(t, strings.Join(issues, ","), "private-")
	require.True(t, reflect.DeepEqual(headersBefore, req.Header))
	require.Equal(t, bodyBefore, string(body))
	delete(req.Header, "user-agent")
	req.Header.Del("X-Api-Key")
	req.Header.Set("X-Claude-Code-Session-Id", "private-session-body")
	require.Empty(t, claudeConsistencyIssues(req, []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "2.1.99", "2.1.100"), `"stream":false`, `"stream":true`))))
	// count_tokens has no stream field; a carried helper header is not an error.
	require.Empty(t, claudeConsistencyIssues(req, []byte(`{"messages":[]}`)))
}

func TestClaudeSafetyAcceptedResponseCannotReplayOnBodyFailure(t *testing.T) {
	s, u, c, account := claudeSafetyFixture(t)
	u.send = func(*http.Request, *tlsfingerprint.Profile) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}
	_, err := s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
	require.NoError(t, err)
	account.ID++
	_, err = s.doClaudeRequest(c, account, claudeSafetyRequest(), "", nil)
	assertClaudeProtection(t, err, true)
	require.Equal(t, 1, u.calls)
}

func TestClaudeSafetyForward403OnlySentOnce(t *testing.T) {
	svc := newForwardPartialUsageServiceForTest(nil)
	u := &claudeSafetyUpstream{send: func(*http.Request, *tlsfingerprint.Profile) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"permission_error","message":"Forbidden"}}`))}, nil
	}}
	svc.httpUpstream = u
	account := newAnthropicOAuthAccountForPartialUsageTest()
	authRepo := &claudeSafetyAuthRepo{}
	svc.rateLimitService.accountRepo = authRepo
	svc.accountRepo = &gatewayForwardErrorPolicyRepoStub{}
	c := newTransportErrorTestGin(t)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(`{"model":"claude-3-5-sonnet-latest","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`)), PlatformAnthropic)
	require.NoError(t, err)
	_, err = svc.Forward(context.Background(), c, account, parsed)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, 403, failover.StatusCode)
	require.False(t, failover.RetryableOnSameAccount)
	require.Equal(t, 1, u.calls)
}

type claudeSafetyAuthRepo struct {
	AccountRepository
}

func (*claudeSafetyAuthRepo) SetError(context.Context, int64, string) error { return nil }
