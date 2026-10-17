package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type timezoneAccountReader struct {
	service.AccountRepository
	account *service.Account
}

func TestHTTPUpstreamAccountTimezonePreservesRoutingWrapper(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.CodexGatewayPin.Enabled = true
	upstream := NewHTTPUpstreamWithAccountTimezone(cfg, nil, nil, nil)
	routed, ok := upstream.(*astraRoutingUpstream)
	require.True(t, ok, "timezone support must preserve the configured routing wrapper")
	delegate, ok := routed.delegate.(*httpUpstreamService)
	require.True(t, ok)
	require.NotNil(t, delegate.accountTimezones)
}

func (r timezoneAccountReader) GetByID(context.Context, int64) (*service.Account, error) {
	return r.account, nil
}

type timezoneEgressProber struct{ proxy string }

func (p *timezoneEgressProber) ProbeProxy(_ context.Context, proxy string) (*service.ProxyExitInfo, int64, error) {
	p.proxy = proxy
	return &service.ProxyExitInfo{IP: "203.0.113.1", Timezone: "Asia/Taipei"}, 0, nil
}

func TestHTTPUpstreamAccountTimezoneUsesRegionalEgress(t *testing.T) {
	received := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	prober := &timezoneEgressProber{}
	cfg := regionalHTTPConfig(t, proxy.URL, "api.vendor.test")
	account := &service.Account{ID: 1, Platform: service.PlatformAnthropic, Extra: map[string]any{"request_timezone": "auto"}}
	upstream := NewHTTPUpstreamWithAccountTimezone(cfg, nil, timezoneAccountReader{account: account}, prober)
	req, err := http.NewRequest(http.MethodPost, "http://api.vendor.test/v1/messages", strings.NewReader(`{"messages":[{"role":"user","content":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := upstream.Do(req, "", account.ID, 1)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Contains(t, <-received, "Asia/Taipei")
	require.Equal(t, proxy.URL, prober.proxy)
}

func TestHTTPUpstreamAccountTimezoneReachesWire(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	account := &service.Account{ID: 1, Platform: service.PlatformGemini, Extra: map[string]any{"request_timezone": "Asia/Taipei"}}
	upstream := NewHTTPUpstreamWithAccountTimezone(cfg, nil, timezoneAccountReader{account: account}, nil)
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}]}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := upstream.Do(req, "", account.ID, 1)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Contains(t, <-received, "Asia/Taipei")
}
