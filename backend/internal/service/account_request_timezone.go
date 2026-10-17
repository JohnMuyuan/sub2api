package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// An empty setting preserves the request. "auto" resolves the actual HTTP egress.
const accountRequestTimezoneKey = "request_timezone"

func ValidateAccountRequestTimezoneExtra(extra map[string]any) error {
	value, exists := extra[accountRequestTimezoneKey]
	if !exists || value == nil {
		return nil
	}
	name, ok := value.(string)
	if !ok || (name != "" && name != "auto" && !isAllowedOpenAIRequestTimezone(name)) {
		return infraerrors.New(http.StatusBadRequest, "INVALID_REQUEST_TIMEZONE", "request_timezone must be empty, auto, or an allowed IANA timezone")
	}
	return nil
}

type accountRequestTimezoneContextKey struct{}
type accountRequestTimezoneBinding struct {
	account *Account
	resolve func(context.Context, string) string
}

// WithAccountRequestTimezone binds an opt-in account setting. Existing OpenAI
// settings and its WebSocket/automatic egress implementation remain authoritative.
func WithAccountRequestTimezone(req *http.Request, account *Account, resolve func(context.Context, string) string) *http.Request {
	if req == nil || account == nil || account.IsOpenAI() || account.getExtraString(accountRequestTimezoneKey) == "" {
		return req
	}
	return req.WithContext(context.WithValue(req.Context(), accountRequestTimezoneContextKey{}, accountRequestTimezoneBinding{account, resolve}))
}

// Reuses the same bounded egress cache/prober used by Codex, without coupling
// other providers to the Codex global switches.
func NewAccountRequestTimezoneResolver(prober ProxyExitInfoProber) func(context.Context, string) string {
	cache := &openAIRequestTimezoneEgress{}
	return func(ctx context.Context, proxy string) string { return cache.resolve(ctx, prober, proxy) }
}

func BindAccountRequestTimezone(req *http.Request, proxy string) (*http.Request, error) {
	if req == nil {
		return req, nil
	}
	b, ok := req.Context().Value(accountRequestTimezoneContextKey{}).(accountRequestTimezoneBinding)
	if !ok || b.account == nil || !accountTimezoneRequestSupported(req) {
		return req, nil
	}
	name := b.account.getExtraString(accountRequestTimezoneKey)
	if name == "auto" {
		if b.resolve == nil {
			return req, nil
		}
		name = b.resolve(req.Context(), proxy)
	}
	if !isAllowedOpenAIRequestTimezone(name) {
		return req, nil
	}
	clone := req.Clone(req.Context())
	// Only generic timezone headers already present are rewritten.
	for key := range clone.Header {
		if strings.EqualFold(key, "x-timezone") || strings.EqualFold(key, "x-client-timezone") {
			clone.Header[key] = []string{name}
		}
	}
	if req.GetBody == nil {
		return clone, nil
	}
	reader, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return nil, err
	}
	updated := normalizeAccountRequestTimezoneBody(body, name)
	if bytes.Equal(body, updated) {
		return clone, nil
	}
	// The existing Anthropic billing fingerprint depends on the first user text.
	if b.account.Platform == PlatformAnthropic {
		updated = syncBillingHeaderVersion(updated, req.Header.Get("User-Agent"))
	}
	clone.Body = &requestTimezoneBody{Reader: bytes.NewReader(updated), original: req.Body}
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(updated)), nil }
	clone.ContentLength = int64(len(updated))
	clone.Header.Del("Content-Length")
	return clone, nil
}

func accountTimezoneRequestSupported(req *http.Request) bool {
	if req.Method != http.MethodPost || req.Header.Get("Content-Encoding") != "" {
		return false
	}
	// Signed cloud requests cannot be changed after signing.
	if strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-") || req.Header.Get("X-Amz-Date") != "" || req.Header.Get("Content-MD5") != "" || req.Header.Get("Digest") != "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/json")
}

func normalizeAccountRequestTimezoneBody(body []byte, timezone string) []byte {
	if !gjson.ValidBytes(body) || !isAllowedOpenAIRequestTimezone(timezone) {
		return body
	}
	// Visit only protocol text positions. Never traverse arbitrary tool arguments,
	// tool results, thinking blocks, attachments, or ordinary user prose.
	rewriteText := func(path string, text gjson.Result) {
		if text.Type != gjson.String {
			return
		}
		next, _, reason := rewriteOpenAIRequestEnvironment(text.String(), timezone)
		if reason == "replaced" {
			if updated, err := sjson.SetBytes(body, path, next); err == nil {
				body = updated
			}
		}
	}
	content := func(path string, value gjson.Result) {
		if value.Type == gjson.String {
			rewriteText(path, value)
			return
		}
		for i, part := range value.Array() {
			kind := part.Get("type").String()
			if kind == "text" || kind == "input_text" {
				rewriteText(path+"."+strconv.Itoa(i)+".text", part.Get("text"))
			}
		}
	}
	content("system", gjson.GetBytes(body, "system"))
	for _, field := range []string{"messages", "input"} {
		for i, message := range gjson.GetBytes(body, field).Array() {
			role := message.Get("role").String()
			if role == "user" || role == "system" || role == "developer" {
				content(field+"."+strconv.Itoa(i)+".content", message.Get("content"))
			}
		}
	}
	// Gemini native and Antigravity's nested Gemini request.
	for _, prefix := range []string{"", "request."} {
		for i, part := range gjson.GetBytes(body, prefix+"systemInstruction.parts").Array() {
			rewriteText(prefix+"systemInstruction.parts."+strconv.Itoa(i)+".text", part.Get("text"))
		}
		for i, message := range gjson.GetBytes(body, prefix+"contents").Array() {
			if message.Get("role").String() != "user" {
				continue
			}
			for j, part := range message.Get("parts").Array() {
				rewriteText(prefix+"contents."+strconv.Itoa(i)+".parts."+strconv.Itoa(j)+".text", part.Get("text"))
			}
		}
	}
	// Anthropic and OpenAI-compatible web search tools document this field.
	for i, tool := range gjson.GetBytes(body, "tools").Array() {
		kind := tool.Get("type").String()
		if kind != "web_search" && !strings.HasPrefix(kind, "web_search_") {
			continue
		}
		if value := tool.Get("user_location.timezone"); value.Type == gjson.String && value.String() != timezone {
			if updated, err := sjson.SetBytes(body, "tools."+strconv.Itoa(i)+".user_location.timezone", timezone); err == nil {
				body = updated
			}
		}
	}
	return body
}
