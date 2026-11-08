package antigravity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTokenEndpointError_Retryable(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusRequestTimeout, true},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusServiceUnavailable, true},
	}
	for _, tt := range tests {
		e := &TokenEndpointError{Op: "token 刷新失败", StatusCode: tt.status}
		if got := e.Retryable(); got != tt.want {
			t.Errorf("status %d: Retryable=%v want %v", tt.status, got, tt.want)
		}
	}
}

func TestRefreshToken_ReturnsTypedErrorWithoutRetry(t *testing.T) {
	var calls int32
	c := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`)),
			Header:     http.Header{},
		}, nil
	})}}

	_, err := c.RefreshToken(context.Background(), "rt")
	var tokenErr *TokenEndpointError
	if !errors.As(err, &tokenErr) {
		t.Fatalf("expected *TokenEndpointError, got %T %v", err, err)
	}
	if tokenErr.StatusCode != http.StatusBadRequest || tokenErr.Retryable() {
		t.Fatalf("unexpected error: %+v", tokenErr)
	}
	if !strings.Contains(err.Error(), "token 刷新失败 (HTTP 400)") {
		t.Fatalf("error message format changed: %s", err)
	}
	if strings.Contains(err.Error(), "rt") && strings.Contains(err.Error(), "refresh_token=") {
		t.Fatalf("refresh token leaked into error: %s", err)
	}
	if calls != 1 {
		t.Fatalf("client must not retry internally, calls=%d", calls)
	}
}

func TestNewClient_ReusesTransportPerProxy(t *testing.T) {
	a := newHardeningTestClient(t, "http://reuse-proxy.example.com:8080")
	b := newHardeningTestClient(t, "http://reuse-proxy.example.com:8080")
	c := newHardeningTestClient(t, "http://other-proxy.example.com:8080")
	if a.httpClient.Transport != b.httpClient.Transport {
		t.Fatal("same proxy URL should share one Transport")
	}
	if a.httpClient.Transport == c.httpClient.Transport {
		t.Fatal("different proxies must not share a Transport")
	}
	tr := a.httpClient.Transport.(*http.Transport)
	if tr.IdleConnTimeout <= 0 {
		t.Fatal("proxy transport must have an idle timeout to avoid connection leaks")
	}
}

func TestNewClient_ConcurrentSameProxy(t *testing.T) {
	var wg sync.WaitGroup
	transports := make([]http.RoundTripper, 32)
	for i := range transports {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cl, err := NewClient("http://concurrent-proxy.example.com:3128")
			if err != nil {
				t.Errorf("NewClient: %v", err)
				return
			}
			transports[i] = cl.httpClient.Transport
		}(i)
	}
	wg.Wait()
	for _, tr := range transports[1:] {
		if tr != transports[0] {
			t.Fatal("concurrent NewClient calls returned different transports for one proxy")
		}
	}
}

func TestLoadCodeAssist_NoFallbackAfterContextCanceled(t *testing.T) {
	var secondHits int32
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-release
	}))
	defer first.Close()
	defer close(release)
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondHits, 1)
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"p"}`))
	}))
	defer second.Close()

	orig := BaseURLs
	BaseURLs = []string{first.URL, second.URL}
	defer func() { BaseURLs = orig }()

	c := newHardeningTestClient(t, "")
	_, _, err := c.LoadCodeAssist(ctx, "token")
	if err == nil {
		t.Fatal("expected error after cancellation")
	}
	if atomic.LoadInt32(&secondHits) != 0 {
		t.Fatal("must not fall back to the next endpoint after the caller canceled")
	}
}

func TestLoadCodeAssist_FallsBackOn5xx(t *testing.T) {
	var firstHits, secondHits int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&firstHits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&secondHits, 1)
		if r.Header.Get("Authorization") != "Bearer token" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"p-1"}`))
	}))
	defer second.Close()

	orig := BaseURLs
	BaseURLs = []string{first.URL, second.URL}
	defer func() { BaseURLs = orig }()

	resp, _, err := newHardeningTestClient(t, "").LoadCodeAssist(context.Background(), "token")
	if err != nil || resp.CloudAICompanionProject != "p-1" {
		t.Fatalf("unexpected result: %+v %v", resp, err)
	}
	if firstHits != 1 || secondHits != 1 {
		t.Fatalf("each endpoint should be hit exactly once, got %d/%d", firstHits, secondHits)
	}
}

func newHardeningTestClient(t *testing.T, proxyURL string) *Client {
	t.Helper()
	c, err := NewClient(proxyURL)
	if err != nil {
		t.Fatalf("NewClient(%q): %v", proxyURL, err)
	}
	return c
}
