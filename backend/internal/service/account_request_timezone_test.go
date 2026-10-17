package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountRequestTimezoneValidation(t *testing.T) {
	for _, value := range []any{nil, "", "auto", "Asia/Taipei", "America/New_York"} {
		require.NoError(t, ValidateAccountRequestTimezoneExtra(map[string]any{"request_timezone": value}))
	}
	for _, value := range []any{8, true, "Unknown/Timezone", " Asia/Taipei"} {
		require.Error(t, ValidateAccountRequestTimezoneExtra(map[string]any{"request_timezone": value}))
	}
}

func TestAccountRequestTimezoneProtocols(t *testing.T) {
	const env = `<environment_context><timezone>Asia/Shanghai</timezone><current_date>2001-01-01</current_date></environment_context>`
	for name, body := range map[string]string{
		"anthropic":   `{"system":[{"type":"text","text":"` + env + `"}]}`,
		"chat":        `{"messages":[{"role":"user","content":"` + env + `"}]}`,
		"responses":   `{"input":[{"role":"user","content":[{"type":"input_text","text":"` + env + `"}]}]}`,
		"gemini":      `{"contents":[{"role":"user","parts":[{"text":"` + env + `"}]}]}`,
		"antigravity": `{"request":{"systemInstruction":{"parts":[{"text":"` + env + `"}]}}}`,
		"web search":  `{"tools":[{"type":"web_search_20250305","user_location":{"city":"Taipei","timezone":"Asia/Shanghai"}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			out := normalizeAccountRequestTimezoneBody([]byte(body), "Asia/Taipei")
			require.Equal(t, strings.ReplaceAll(body, "Asia/Shanghai", "Asia/Taipei"), string(out))
			require.Equal(t, out, normalizeAccountRequestTimezoneBody(out, "Asia/Taipei"))
		})
	}
	for _, body := range []string{
		`{"messages":[{"role":"user","content":"Please explain Asia/Shanghai"}]}`,
		`{"messages":[{"role":"assistant","content":"` + env + `"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"tool_result","content":"` + env + `"}]}]}`,
		`{"tools":[{"input_schema":{"example":"` + env + `"}}]}`,
		`{"system":"<environment_context><timezone>Asia/Shanghai</timezone>"}`,
		`invalid JSON`,
	} {
		require.Equal(t, body, string(normalizeAccountRequestTimezoneBody([]byte(body), "Asia/Taipei")))
	}
}

func TestAccountRequestTimezoneActualEgressAndReplay(t *testing.T) {
	const body = `{"messages":[{"role":"user","content":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}`
	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timezone", "Asia/Shanghai")
	req.Header.Set("Accept-Language", "zh-TW")
	account := &Account{Platform: PlatformAnthropic, Extra: map[string]any{"request_timezone": "auto"}}
	var exits []string
	bound := WithAccountRequestTimezone(req, account, func(_ context.Context, proxy string) string {
		exits = append(exits, proxy)
		if proxy == "first" {
			return "Asia/Taipei"
		}
		return "America/New_York"
	})
	for _, proxy := range []string{"first", "fallback"} {
		out, err := BindOpenAIRequestTimezone(bound, proxy)
		require.NoError(t, err)
		data, err := io.ReadAll(out.Body)
		require.NoError(t, err)
		target := "Asia/Taipei"
		if proxy == "fallback" {
			target = "America/New_York"
		}
		require.Contains(t, gjson.GetBytes(data, "messages.0.content").String(), target)
		require.Equal(t, target, out.Header.Get("X-Timezone"))
		require.Equal(t, "zh-TW", out.Header.Get("Accept-Language"))
		require.Empty(t, out.Header.Get("Oai-Client-Timezone"))
		require.Equal(t, int64(len(data)), out.ContentLength)
		replay, err := out.GetBody()
		require.NoError(t, err)
		replayed, err := io.ReadAll(replay)
		require.NoError(t, err)
		require.Equal(t, data, replayed)
		_ = replay.Close()
	}
	require.Equal(t, []string{"first", "fallback"}, exits)
	require.Equal(t, "Asia/Shanghai", req.Header.Get("X-Timezone"))
	original, _ := io.ReadAll(req.Body)
	require.Equal(t, body, string(original))
}

func TestAccountRequestTimezonePreservesUnsupportedAndFailures(t *testing.T) {
	const body = `{"messages":[{"role":"user","content":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}`
	for _, mode := range []string{"", "auto", "invalid"} {
		req, _ := http.NewRequest(http.MethodPost, "https://example.com", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		bound := WithAccountRequestTimezone(req, &Account{Platform: PlatformGemini, Extra: map[string]any{"request_timezone": mode}}, func(context.Context, string) string { return "" })
		out, err := BindAccountRequestTimezone(bound, "proxy")
		require.NoError(t, err)
		require.Same(t, bound, out)
	}
	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformGrok, PlatformOpenAI} {
		req, _ := http.NewRequest(http.MethodPost, "https://example.com", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "AWS4-HMAC-SHA256 signed")
		bound := WithAccountRequestTimezone(req, &Account{Platform: platform, Extra: map[string]any{"request_timezone": "Asia/Taipei"}}, nil)
		out, err := BindAccountRequestTimezone(bound, "")
		require.NoError(t, err)
		require.Same(t, bound, out)
	}
}

type accountTimezoneTestProber struct{ calls int }

func (p *accountTimezoneTestProber) ProbeProxy(_ context.Context, proxy string) (*ProxyExitInfo, int64, error) {
	p.calls++
	if proxy == "failed" {
		return nil, 0, errors.New("offline")
	}
	return &ProxyExitInfo{IP: "203.0.113.1", Timezone: "Asia/Taipei"}, 0, nil
}

func TestAccountRequestTimezoneResolverCache(t *testing.T) {
	prober := &accountTimezoneTestProber{}
	resolve := NewAccountRequestTimezoneResolver(prober)
	require.Equal(t, "Asia/Taipei", resolve(context.Background(), "proxy-a"))
	require.Equal(t, "Asia/Taipei", resolve(context.Background(), "proxy-a"))
	require.Equal(t, 1, prober.calls)
	require.Equal(t, "Asia/Taipei", resolve(context.Background(), "proxy-b"))
	require.Equal(t, 2, prober.calls)
	require.Empty(t, resolve(context.Background(), "failed"))
}
