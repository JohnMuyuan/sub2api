package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/gin-gonic/gin"
)

const (
	accountWindowPrimeTickInterval   = time.Minute
	accountWindowPrimeRequestTimeout = 2 * time.Minute
	accountWindowPrimeWorkers        = 4
	accountWindowPrimeLockTTL        = 10 * time.Minute
	accountWindowPrimeErrorMaxRunes  = 300
)

// accountWindowPrimeRunner checks every minute for idle windows and primes
// them. It rides on the scheduled-test runner's lifecycle.
type accountWindowPrimeRunner struct {
	accountTestSvc *AccountTestService
	rateLimitSvc   *RateLimitService
	lock           LeaderLockCache
	now            func() time.Time
	send           func(ctx context.Context, account *Account, model, prompt string) (http.Header, error)

	running  atomic.Bool
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func newAccountWindowPrimeRunner(accountTestSvc *AccountTestService, rateLimitSvc *RateLimitService, lock LeaderLockCache) *accountWindowPrimeRunner {
	r := &accountWindowPrimeRunner{
		accountTestSvc: accountTestSvc,
		rateLimitSvc:   rateLimitSvc,
		lock:           lock,
		now:            time.Now,
		stopCh:         make(chan struct{}),
	}
	r.send = r.sendPrimeRequest
	return r
}

func (r *accountWindowPrimeRunner) start() {
	if r == nil {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(accountWindowPrimeTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.tick()
			}
		}
	}()
}

func (r *accountWindowPrimeRunner) stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.stopCh) })
	r.wg.Wait()
}

// tick skips while a previous tick is still sending.
func (r *accountWindowPrimeRunner) tick() {
	if !r.running.CompareAndSwap(false, true) {
		return
	}
	defer r.running.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	r.runOnce(ctx)
}

func (r *accountWindowPrimeRunner) runOnce(ctx context.Context) {
	if r.rateLimitSvc == nil || r.rateLimitSvc.accountRepo == nil || r.rateLimitSvc.settingService == nil {
		return
	}
	settings := r.rateLimitSvc.settingService.GetAccountWindowPrimeSettings(ctx)
	now := r.now()
	if !accountWindowPrimeWithinActiveHours(settings, now, timezone.Location()) {
		return
	}

	var due []*Account
	for _, platform := range AccountWindowPrimePlatforms {
		accounts, err := r.rateLimitSvc.accountRepo.ListByPlatform(ctx, platform)
		if err != nil {
			slog.Warn("account_window_prime_list_failed", "platform", platform, "error", err)
			continue
		}
		for i := range accounts {
			if account := &accounts[i]; r.isDue(account, settings, now) {
				due = append(due, account)
			}
		}
	}

	slots := make(chan struct{}, accountWindowPrimeWorkers)
	var wg sync.WaitGroup
	for _, account := range due {
		slots <- struct{}{}
		wg.Add(1)
		go func(account *Account) {
			defer func() { <-slots; wg.Done() }()
			r.prime(ctx, account, settings, now)
		}(account)
	}
	wg.Wait()
}

func (r *accountWindowPrimeRunner) isDue(account *Account, settings AccountWindowPrimeSettings, now time.Time) bool {
	if _, enabled := resolveAccountWindowPrimePlan(account, settings); !enabled || !account.IsSchedulable() {
		return false
	}
	trigger, _ := accountWindowPrimeTrigger(accountWindowPrimeIdleWindows(account, now))
	if trigger.IsZero() || now.Before(accountWindowPrimeDueAt(account.ID, trigger, settings)) {
		return false
	}
	return accountWindowPrimeShouldAttempt(readAccountWindowPrimeState(account), trigger, now)
}

func (r *accountWindowPrimeRunner) prime(ctx context.Context, account *Account, settings AccountWindowPrimeSettings, now time.Time) {
	plan, _ := resolveAccountWindowPrimePlan(account, settings)
	trigger, windows := accountWindowPrimeTrigger(accountWindowPrimeIdleWindows(account, now))
	if !r.acquire(ctx, account.ID, trigger) {
		return
	}
	state := readAccountWindowPrimeState(account)
	attempts := 1
	if state.TriggerResetUnix == trigger.Unix() {
		attempts = state.Attempts + 1
	}

	reqCtx, cancel := context.WithTimeout(ctx, accountWindowPrimeRequestTimeout)
	headers, err := r.send(reqCtx, account, plan.Model, pickAccountWindowPrimePrompt(plan.Prompts))
	cancel()
	if headers != nil && account.IsAnthropicOAuthOrSetupToken() {
		// The Claude test path does not record windows on its own.
		r.rateLimitSvc.UpdateSessionWindow(ctx, account, headers)
	}

	record := map[string]any{
		"trigger_reset_unix": trigger.Unix(),
		"windows":            windows,
		"model":              plan.Model,
		"attempts":           attempts,
		"last_attempt_at":    r.now().UTC().Format(time.RFC3339),
		"status":             accountWindowPrimeStatusSucceeded,
	}
	if err != nil {
		record["status"] = accountWindowPrimeStatusFailed
		record["error"] = truncateAccountWindowPrimeError(err.Error())
		slog.Warn("account_window_prime_failed", "account_id", account.ID, "windows", windows, "attempt", attempts, "error", err)
	} else {
		slog.Info("account_window_prime_succeeded", "account_id", account.ID, "windows", windows, "model", plan.Model)
	}
	if updateErr := r.rateLimitSvc.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{AccountWindowPrimeStateExtraKey: record}); updateErr != nil {
		slog.Warn("account_window_prime_state_save_failed", "account_id", account.ID, "error", updateErr)
	}
}

// acquire lets one instance prime a given reset; without a lock it is a no-op.
// The lock is never released: its TTL covers the whole attempt.
func (r *accountWindowPrimeRunner) acquire(ctx context.Context, accountID int64, trigger time.Time) bool {
	if r.lock == nil {
		return true
	}
	key := fmt.Sprintf("window_prime:%d:%d", accountID, trigger.Unix())
	ok, err := r.lock.TryAcquireLeaderLock(ctx, key, "window-prime", accountWindowPrimeLockTTL)
	if err != nil {
		slog.Warn("account_window_prime_lock_failed", "account_id", accountID, "error", err)
		return false
	}
	return ok
}

func truncateAccountWindowPrimeError(message string) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > accountWindowPrimeErrorMaxRunes {
		return string(runes[:accountWindowPrimeErrorMaxRunes]) + "..."
	}
	return string(runes)
}

// sendPrimeRequest runs a background account test with the chosen prompt and
// returns the upstream response headers.
func (r *accountWindowPrimeRunner) sendPrimeRequest(ctx context.Context, account *Account, model, prompt string) (http.Header, error) {
	if r.accountTestSvc == nil {
		return nil, fmt.Errorf("account test service unavailable")
	}
	var headers http.Header
	ctx = withAccountWindowPrimeRequest(ctx, accountWindowPrimeRequest{
		prompt:    prompt,
		onHeaders: func(h http.Header) { headers = h.Clone() },
	})
	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = (&http.Request{Header: make(http.Header)}).WithContext(ctx)
	ginCtx.Set(accountTestBackgroundKey, true)

	testErr := r.accountTestSvc.TestAccountConnection(ginCtx, account.ID, model, prompt, AccountTestModeDefault)
	if _, errMsg := parseTestSSEOutput(w.Body.String()); errMsg != "" {
		return headers, fmt.Errorf("%s", errMsg)
	}
	return headers, testErr
}

type accountWindowPrimeRequestKey struct{}

// accountWindowPrimeRequest carries the prompt into the test request and hands
// the response headers back.
type accountWindowPrimeRequest struct {
	prompt    string
	onHeaders func(http.Header)
}

func withAccountWindowPrimeRequest(ctx context.Context, req accountWindowPrimeRequest) context.Context {
	return context.WithValue(ctx, accountWindowPrimeRequestKey{}, req)
}

func accountWindowPrimeRequestFrom(ctx context.Context) (accountWindowPrimeRequest, bool) {
	req, ok := ctx.Value(accountWindowPrimeRequestKey{}).(accountWindowPrimeRequest)
	return req, ok
}

// applyAccountWindowPrimeTestPayload swaps the test prompt for the priming one
// in a payload whose first message holds a text content block.
func applyAccountWindowPrimeTestPayload(ctx context.Context, payload map[string]any, messagesKey string) {
	req, ok := accountWindowPrimeRequestFrom(ctx)
	if !ok || strings.TrimSpace(req.prompt) == "" {
		return
	}
	messages, ok := payload[messagesKey].([]map[string]any)
	if !ok || len(messages) == 0 {
		return
	}
	if content, ok := messages[0]["content"].([]map[string]any); ok && len(content) > 0 {
		content[0]["text"] = req.prompt
	}
}

func notifyAccountWindowPrimeHeaders(ctx context.Context, headers http.Header) {
	if req, ok := accountWindowPrimeRequestFrom(ctx); ok && req.onHeaders != nil {
		req.onHeaders(headers)
	}
}
