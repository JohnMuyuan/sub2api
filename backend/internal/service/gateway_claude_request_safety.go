package service

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const claudeRequestSafetyKey = "claude.request_safety"
const claudeRetryProtectionReason GatewayFailureReason = "claude_retry_protection"

// Owned by one Gin request; shared by account failover and 400 rectifiers.
// No account/session identities or prompts are retained beyond this request.
type claudeRequestSafety struct {
	started    time.Time
	attempts   int
	writerSize int
	accounts   map[int64]*claudeAccountAttempts
	reported   map[string]bool
	terminal   string
}

type claudeAccountAttempts struct {
	attempts  int
	blocked   bool
	notBefore time.Time
	profile   *tlsfingerprint.Profile
}

func claudeSafety(c *gin.Context) *claudeRequestSafety {
	if c != nil {
		if value, ok := c.Get(claudeRequestSafetyKey); ok {
			if state, ok := value.(*claudeRequestSafety); ok {
				return state
			}
		}
	}
	state := &claudeRequestSafety{accounts: make(map[int64]*claudeAccountAttempts), reported: make(map[string]bool)}
	if c != nil {
		c.Set(claudeRequestSafetyKey, state)
	}
	return state
}

func claudeSafetyError(accountID int64, reason string, stop bool) error {
	slog.Warn("claude.retry_protection", "account_id", accountID, "reason", reason)
	action := NextAccountRetry
	scope := GatewayFailureScopeAccount
	if stop {
		action = NextAccountStop
		scope = GatewayFailureScopeRequest
	}
	return &UpstreamFailoverError{
		StatusCode: http.StatusBadGateway, ResponseBody: gatewayTransportFailoverBody,
		Reason: claudeRetryProtectionReason, Scope: scope, NextAccountAction: action,
	}
}

// Retry-After supports delta-seconds and HTTP dates. Huge positive deltas must
// reject this request's retry, rather than overflow into an immediate retry.
func claudeRetryAfter(value string, now time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value != "" && strings.Trim(value, "0123456789") == "" {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > uint64((24*time.Hour)/time.Second) {
			return now.Add(24 * time.Hour)
		}
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if date, err := http.ParseTime(value); err == nil {
		return date
	}
	return now.Add(time.Second)
}

// doClaudeRequest is the single admission point for Claude subscription sends.
// The time limit only gates additional sends; a healthy long generation keeps
// its original lifetime. Other providers and API-key accounts are unchanged.
func (s *GatewayService) doClaudeRequest(c *gin.Context, account *Account, req *http.Request, proxyURL string, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if !account.IsAnthropicOAuthOrSetupToken() {
		return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, profile)
	}
	state := claudeSafety(c)
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	if c != nil && c.Request != nil {
		if err := c.Request.Context().Err(); err != nil {
			return nil, err
		}
	}
	if state.terminal != "" {
		return nil, claudeSafetyError(account.ID, state.terminal, true)
	}
	now := time.Now()
	if state.attempts > 0 {
		if c != nil && c.Writer != nil && c.Writer.Size() > state.writerSize {
			state.terminal = "response_already_started"
		} else if state.attempts >= maxRetryAttempts || now.Sub(state.started) >= maxRetryElapsed {
			state.terminal = "request_retry_budget_exhausted"
		}
		if state.terminal != "" {
			return nil, claudeSafetyError(account.ID, state.terminal, true)
		}
	}
	attempt := state.accounts[account.ID]
	if attempt == nil {
		attempt = &claudeAccountAttempts{profile: profile}
		state.accounts[account.ID] = attempt
	}
	if attempt.blocked {
		return nil, claudeSafetyError(account.ID, "account_rejected_request", false)
	}
	if now.Before(attempt.notBefore) {
		if !attempt.notBefore.Before(state.started.Add(maxRetryElapsed)) {
			return nil, claudeSafetyError(account.ID, "retry_after_exceeds_budget", false)
		}
		// A detached streaming context must not keep a canceled client retrying.
		waitCtx, cancelWait := context.WithCancel(req.Context())
		stopClientWatch := func() bool { return false }
		if c != nil && c.Request != nil {
			stopClientWatch = context.AfterFunc(c.Request.Context(), cancelWait)
		}
		waitErr := sleepWithContext(waitCtx, time.Until(attempt.notBefore))
		stopClientWatch()
		cancelWait()
		if waitErr != nil {
			return nil, waitErr
		}
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if time.Since(state.started) >= maxRetryElapsed {
			state.terminal = "request_retry_budget_exhausted"
			return nil, claudeSafetyError(account.ID, state.terminal, true)
		}
	}
	if state.attempts == 0 {
		state.started = time.Now()
		if c != nil && c.Writer != nil {
			state.writerSize = c.Writer.Size()
		}
	}
	if getHeaderRaw(req.Header, "x-stainless-retry-count") != "" {
		for key := range req.Header {
			if strings.EqualFold(key, "x-stainless-retry-count") {
				delete(req.Header, key)
			}
		}
		setHeaderRaw(req.Header, resolveWireCasing("x-stainless-retry-count"), strconv.Itoa(attempt.attempts))
	}
	state.attempts++
	attempt.attempts++
	// Only explicit connection-stage failure before ANY acquired connection is
	// proof that replay cannot duplicate a generation. EOF/timeouts with no
	// trace (including custom transports) are conservatively ambiguous.
	var connectionSeen, preflightFailed atomic.Bool
	trace := &httptrace.ClientTrace{
		GotConn:              func(httptrace.GotConnInfo) { connectionSeen.Store(true) },
		WroteHeaderField:     func(string, []string) { connectionSeen.Store(true) },
		WroteHeaders:         func() { connectionSeen.Store(true) },
		WroteRequest:         func(httptrace.WroteRequestInfo) { connectionSeen.Store(true) },
		GotFirstResponseByte: func() { connectionSeen.Store(true) },
		DNSDone: func(info httptrace.DNSDoneInfo) {
			if info.Err != nil {
				preflightFailed.Store(true)
			}
		},
		ConnectDone: func(_, _ string, err error) {
			if err != nil {
				preflightFailed.Store(true)
			}
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err != nil {
				preflightFailed.Store(true)
			}
		},
	}
	traced := req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := s.httpUpstream.DoWithTLS(traced, proxyURL, account.ID, account.Concurrency, attempt.profile)
	if err != nil {
		if req.Context().Err() != nil {
			return resp, req.Context().Err()
		}
		if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
			return resp, c.Request.Context().Err()
		}
		if errors.Is(err, context.Canceled) {
			return resp, err
		}
		if resp != nil || !preflightFailed.Load() || connectionSeen.Load() {
			state.terminal = "ambiguous_transport_result"
			return resp, claudeSafetyError(account.ID, state.terminal, true)
		}
		return resp, err
	}
	if resp != nil {
		// A 2xx/3xx response proves upstream acceptance. If reading/parsing that
		// response later fails, do not replay a potentially completed generation.
		if resp.StatusCode < 400 {
			state.terminal = "upstream_response_already_accepted"
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			attempt.blocked = true
		case http.StatusTooManyRequests, http.StatusServiceUnavailable, 529:
			attempt.notBefore = claudeRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		}
	}
	return resp, nil
}

// Fixed issue codes only: never return/log a header value, metadata identifier,
// URL, or system text. Missing optional fields are not treated as mismatches.
func claudeConsistencyIssues(req *http.Request, body []byte) []string {
	var issues []string
	uid := ParseMetadataUserID(gjson.GetBytes(body, "metadata.user_id").String())
	if session := getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"); session != "" && uid != nil && session != uid.SessionID {
		issues = append(issues, "session_id_mismatch")
	}
	version := ExtractCLIVersion(getHeaderRaw(req.Header, "User-Agent"))
	system := gjson.GetBytes(body, "system")
	blocks := system.Array()
	if system.Type == gjson.String {
		blocks = []gjson.Result{system}
	}
	for _, block := range blocks {
		text := block.String()
		if block.IsObject() {
			text = block.Get("text").String()
		}
		if version != "" && strings.HasPrefix(text, "x-anthropic-billing-header") {
			if billing := ccVersionInBillingRe.FindString(text); billing != "" && strings.TrimPrefix(billing, "cc_version=") != version {
				issues = append(issues, "cli_billing_version_mismatch")
				break
			}
		}
	}
	if stream := gjson.GetBytes(body, "stream"); stream.Exists() && !stream.Bool() && getHeaderRaw(req.Header, "X-Stainless-Helper-Method") == "stream" {
		issues = append(issues, "stream_helper_mismatch")
	}
	if getHeaderRaw(req.Header, "Authorization") != "" && getHeaderRaw(req.Header, "X-Api-Key") != "" {
		issues = append(issues, "multiple_auth_methods")
	}
	for _, key := range []string{"User-Agent", "Authorization", "X-Api-Key", "X-Claude-Code-Session-Id", "X-Stainless-Retry-Count", "X-Stainless-Helper-Method"} {
		count := 0
		for name, values := range req.Header {
			if strings.EqualFold(name, key) {
				count += len(values)
			}
		}
		if count > 1 {
			issues = append(issues, "duplicate_"+strings.ToLower(key))
		}
	}
	return issues
}

func diagnoseClaudeRequest(c *gin.Context, req *http.Request, body []byte, account *Account) {
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
		return
	}
	state := claudeSafety(c)
	for _, issue := range claudeConsistencyIssues(req, body) {
		if !state.reported[issue] {
			state.reported[issue] = true
			slog.Warn("claude.request_consistency", "account_id", account.ID, "issue", issue)
		}
	}
}

// Propagate acceptance/ambiguous-result protection to the outer scheduler as
// well. An alternate account may be API-key/Bedrock and bypass subscription
// send admission, so checking only the next doClaudeRequest is insufficient.
func protectClaudeReplay(c *gin.Context, err error) error {
	if c == nil || err == nil {
		return err
	}
	value, exists := c.Get(claudeRequestSafetyKey)
	if !exists {
		return err
	}
	state, ok := value.(*claudeRequestSafety)
	if !ok || state.terminal == "" {
		return err
	}
	var failover *UpstreamFailoverError
	if !errors.As(err, &failover) || !failover.ShouldRetryNextAccount() {
		return err
	}
	protected := *failover
	protected.NextAccountAction = NextAccountStop
	protected.RetryableOnSameAccount = false
	slog.Warn("claude.retry_protection", "reason", state.terminal)
	return &protected
}
