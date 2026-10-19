//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type windowPrimeRepoStub struct {
	mockAccountRepoForGemini
	mu                 sync.Mutex
	accounts           map[string][]Account
	extraUpdates       map[int64]map[string]any
	sessionWindowCalls int
}

func (r *windowPrimeRepoStub) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Account(nil), r.accounts[platform]...), nil
}

func (r *windowPrimeRepoStub) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.extraUpdates == nil {
		r.extraUpdates = map[int64]map[string]any{}
	}
	r.extraUpdates[id] = updates
	return nil
}

func (r *windowPrimeRepoStub) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionWindowCalls++
	return nil
}

type windowPrimeSend struct {
	accountID int64
	model     string
	prompt    string
}

func newWindowPrimeTestRunner(t *testing.T, repo *windowPrimeRepoStub, settings string, now time.Time) (*accountWindowPrimeRunner, *[]windowPrimeSend) {
	t.Helper()
	settingsRepo := newMockSettingRepo()
	if settings != "" {
		settingsRepo.data[SettingKeyAccountWindowPrime] = settings
	}
	rl := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))
	runner := newAccountWindowPrimeRunner(nil, rl, nil)
	runner.now = func() time.Time { return now }
	var mu sync.Mutex
	sends := []windowPrimeSend{}
	runner.send = func(_ context.Context, account *Account, model, prompt string) (http.Header, error) {
		mu.Lock()
		defer mu.Unlock()
		sends = append(sends, windowPrimeSend{accountID: account.ID, model: model, prompt: prompt})
		return nil, nil
	}
	return runner, &sends
}

func openAIWindowPrimeAccount(id int64, reset5h time.Time, extra map[string]any) Account {
	values := map[string]any{
		"codex_5h_used_percent": 12.0,
		"codex_5h_reset_at":     reset5h.UTC().Format(time.RFC3339),
	}
	for key, value := range extra {
		values[key] = value
	}
	return Account{
		ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Extra: values,
	}
}

func TestAccountWindowPrime_NormalizeSettings(t *testing.T) {
	defaults := DefaultAccountWindowPrimeSettings()
	require.Equal(t, 1, defaults.DelayMinMinutes)
	require.Equal(t, 5, defaults.DelayMaxMinutes)
	require.False(t, defaults.ActiveHoursEnabled)
	require.Equal(t, []string{"Hi"}, defaults.Prompts)
	require.False(t, defaults.Platforms[PlatformOpenAI].Enabled)

	valid := defaults
	valid.Platforms = map[string]AccountWindowPrimePlatformSettings{PlatformAnthropic: {Enabled: true, Model: " claude-x "}}
	valid.Prompts = []string{"  hello ", "", "继续"}
	valid.ActiveHoursEnabled = true
	valid.ActiveHoursStart, valid.ActiveHoursEnd = "22:00", "06:30"
	normalized, err := NormalizeAccountWindowPrimeSettings(valid)
	require.NoError(t, err)
	require.Equal(t, "claude-x", normalized.Platforms[PlatformAnthropic].Model)
	require.False(t, normalized.Platforms[PlatformOpenAI].Enabled)
	require.Equal(t, []string{"hello", "继续"}, normalized.Prompts)

	empty := defaults
	empty.Prompts = []string{" "}
	normalized, err = NormalizeAccountWindowPrimeSettings(empty)
	require.NoError(t, err)
	require.Equal(t, []string{"Hi"}, normalized.Prompts)

	for name, mutate := range map[string]func(*AccountWindowPrimeSettings){
		"platform": func(s *AccountWindowPrimeSettings) {
			s.Platforms = map[string]AccountWindowPrimePlatformSettings{"grok": {Enabled: true}}
		},
		"delay":     func(s *AccountWindowPrimeSettings) { s.DelayMinMinutes, s.DelayMaxMinutes = 6, 5 },
		"delay max": func(s *AccountWindowPrimeSettings) { s.DelayMaxMinutes = 121 },
		"clock":     func(s *AccountWindowPrimeSettings) { s.ActiveHoursStart = "8:00" },
		"start 24":  func(s *AccountWindowPrimeSettings) { s.ActiveHoursStart = "24:00" },
		"long":      func(s *AccountWindowPrimeSettings) { s.Prompts = []string{string(make([]rune, 201))} },
		"too many": func(s *AccountWindowPrimeSettings) {
			s.Prompts = make([]string, 51)
			for i := range s.Prompts {
				s.Prompts[i] = "x"
			}
		},
	} {
		input := DefaultAccountWindowPrimeSettings()
		mutate(&input)
		_, err := NormalizeAccountWindowPrimeSettings(input)
		require.Error(t, err, name)
	}

	require.Equal(t, defaults, parseAccountWindowPrimeSettings("{broken"))
}

func TestAccountWindowPrime_ResolvePlan(t *testing.T) {
	settings := DefaultAccountWindowPrimeSettings()
	settings.Prompts = []string{"a", "b"}
	account := openAIWindowPrimeAccount(1, time.Now(), nil)

	_, ok := resolveAccountWindowPrimePlan(&account, settings)
	require.False(t, ok, "platform default is off")

	settings.Platforms[PlatformOpenAI] = AccountWindowPrimePlatformSettings{Enabled: true}
	plan, ok := resolveAccountWindowPrimePlan(&account, settings)
	require.True(t, ok)
	require.Equal(t, "gpt-5.4-mini", plan.Model)
	require.Equal(t, []string{"a", "b"}, plan.Prompts)

	account.Extra[AccountWindowPrimeExtraKey] = map[string]any{"mode": "off"}
	_, ok = resolveAccountWindowPrimePlan(&account, settings)
	require.False(t, ok)

	settings.Platforms[PlatformOpenAI] = AccountWindowPrimePlatformSettings{Enabled: false, Model: "gpt-platform"}
	account.Extra[AccountWindowPrimeExtraKey] = map[string]any{"mode": "on"}
	plan, ok = resolveAccountWindowPrimePlan(&account, settings)
	require.True(t, ok)
	require.Equal(t, "gpt-platform", plan.Model)

	account.Extra[AccountWindowPrimeExtraKey] = map[string]any{"mode": "on", "model": "gpt-own", "prompts": []any{"own", " "}}
	plan, _ = resolveAccountWindowPrimePlan(&account, settings)
	require.Equal(t, "gpt-own", plan.Model)
	require.Equal(t, []string{"own"}, plan.Prompts)

	apiKey := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{AccountWindowPrimeExtraKey: map[string]any{"mode": "on"}}}
	_, ok = resolveAccountWindowPrimePlan(&apiKey, settings)
	require.False(t, ok)

	parent := int64(1)
	shadow := openAIWindowPrimeAccount(3, time.Now(), map[string]any{AccountWindowPrimeExtraKey: map[string]any{"mode": "on"}})
	shadow.ParentAccountID = &parent
	_, ok = resolveAccountWindowPrimePlan(&shadow, settings)
	require.False(t, ok)

	anthropic := Account{ID: 4, Platform: PlatformAnthropic, Type: AccountTypeSetupToken, Extra: map[string]any{AccountWindowPrimeExtraKey: map[string]any{"mode": "on"}}}
	plan, ok = resolveAccountWindowPrimePlan(&anthropic, settings)
	require.True(t, ok)
	require.Equal(t, "claude-haiku-4-5-20251001", plan.Model)
}

func TestAccountWindowPrime_IdleWindows(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	account := openAIWindowPrimeAccount(1, now.Add(-30*time.Minute), map[string]any{
		"codex_7d_used_percent": 40.0,
		"codex_7d_reset_at":     now.Add(48 * time.Hour).Format(time.RFC3339),
	})
	idle := accountWindowPrimeIdleWindows(&account, now)
	require.Equal(t, []accountWindowPrimeIdleWindow{{Window: "5h", ResetAt: now.Add(-30 * time.Minute)}}, idle)

	// A reset time without a usage snapshot is not trusted.
	noSnapshot := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_5h_reset_at": now.Add(-time.Hour).Format(time.RFC3339)}}
	require.Empty(t, accountWindowPrimeIdleWindows(&noSnapshot, now))

	sessionEnd := now.Add(-2 * time.Hour)
	anthropic := Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth, SessionWindowEnd: &sessionEnd,
		Extra: map[string]any{"passive_usage_7d_reset": float64(now.Add(-time.Hour).Unix())}}
	trigger, windows := accountWindowPrimeTrigger(accountWindowPrimeIdleWindows(&anthropic, now))
	require.Equal(t, sessionEnd, trigger)
	require.ElementsMatch(t, []string{"5h", "7d"}, windows)
}

func TestAccountWindowPrime_DueAtIsStableWithinDelay(t *testing.T) {
	settings := DefaultAccountWindowPrimeSettings()
	trigger := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	seen := map[time.Duration]bool{}
	for id := int64(1); id <= 200; id++ {
		due := accountWindowPrimeDueAt(id, trigger, settings)
		require.Equal(t, due, accountWindowPrimeDueAt(id, trigger, settings))
		delay := due.Sub(trigger)
		require.GreaterOrEqual(t, delay, time.Minute)
		require.LessOrEqual(t, delay, 5*time.Minute)
		seen[delay] = true
	}
	require.Greater(t, len(seen), 50)

	settings.DelayMinMinutes, settings.DelayMaxMinutes = 0, 0
	require.Equal(t, trigger, accountWindowPrimeDueAt(7, trigger, settings))
}

func TestAccountWindowPrime_ActiveHours(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 19, hour, minute, 0, 0, loc) }
	settings := DefaultAccountWindowPrimeSettings()
	require.True(t, accountWindowPrimeWithinActiveHours(settings, at(3, 0), loc), "disabled by default")

	settings.ActiveHoursEnabled = true
	settings.ActiveHoursStart, settings.ActiveHoursEnd = "08:00", "24:00"
	require.False(t, accountWindowPrimeWithinActiveHours(settings, at(7, 59), loc))
	require.True(t, accountWindowPrimeWithinActiveHours(settings, at(8, 0), loc))
	require.True(t, accountWindowPrimeWithinActiveHours(settings, at(23, 59), loc))
	// The check uses the site timezone, not UTC.
	require.False(t, accountWindowPrimeWithinActiveHours(settings, at(3, 0).UTC(), loc))

	settings.ActiveHoursStart, settings.ActiveHoursEnd = "22:00", "06:00"
	require.True(t, accountWindowPrimeWithinActiveHours(settings, at(23, 0), loc))
	require.True(t, accountWindowPrimeWithinActiveHours(settings, at(5, 59), loc))
	require.False(t, accountWindowPrimeWithinActiveHours(settings, at(12, 0), loc))
}

func TestAccountWindowPrime_RetryBudget(t *testing.T) {
	trigger := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	now := trigger.Add(time.Hour)
	require.True(t, accountWindowPrimeShouldAttempt(accountWindowPrimeState{}, trigger, now))
	require.True(t, accountWindowPrimeShouldAttempt(accountWindowPrimeState{TriggerResetUnix: trigger.Add(-5 * time.Hour).Unix(), Status: "succeeded"}, trigger, now))
	same := accountWindowPrimeState{TriggerResetUnix: trigger.Unix()}
	require.False(t, accountWindowPrimeShouldAttempt(func() accountWindowPrimeState { s := same; s.Status = "succeeded"; return s }(), trigger, now))
	failed := same
	failed.Status, failed.Attempts, failed.LastAttemptAt = "failed", 1, now.Add(-2*time.Minute)
	require.False(t, accountWindowPrimeShouldAttempt(failed, trigger, now))
	failed.LastAttemptAt = now.Add(-6 * time.Minute)
	require.True(t, accountWindowPrimeShouldAttempt(failed, trigger, now))
	failed.Attempts = 3
	require.False(t, accountWindowPrimeShouldAttempt(failed, trigger, now))
}

func TestAccountWindowPrime_RunnerPrimesDueAccountsOnce(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	settings := `{"platforms":{"openai":{"enabled":true}},"delay_min_minutes":1,"delay_max_minutes":5,"active_hours_start":"08:00","active_hours_end":"24:00","prompts":["p1","p2"]}`
	paused := openAIWindowPrimeAccount(3, now.Add(-time.Hour), nil)
	// IsSchedulable compares against the real clock.
	until := time.Now().Add(time.Hour)
	paused.RateLimitResetAt = &until
	repo := &windowPrimeRepoStub{accounts: map[string][]Account{PlatformOpenAI: {
		openAIWindowPrimeAccount(1, now.Add(-time.Hour), nil),      // due
		openAIWindowPrimeAccount(2, now.Add(-30*time.Second), nil), // inside the delay
		paused, // rate limited
		openAIWindowPrimeAccount(4, now.Add(time.Hour), nil), // window still running
		openAIWindowPrimeAccount(5, now.Add(-time.Hour), map[string]any{ // opted out
			AccountWindowPrimeExtraKey: map[string]any{"mode": "off"},
		}),
	}}}
	runner, sends := newWindowPrimeTestRunner(t, repo, settings, now)

	runner.runOnce(context.Background())
	require.Len(t, *sends, 1)
	require.Equal(t, int64(1), (*sends)[0].accountID)
	require.Equal(t, "gpt-5.4-mini", (*sends)[0].model)
	require.Contains(t, []string{"p1", "p2"}, (*sends)[0].prompt)

	state := repo.extraUpdates[1][AccountWindowPrimeStateExtraKey].(map[string]any)
	require.Equal(t, "succeeded", state["status"])
	require.Equal(t, now.Add(-time.Hour).Unix(), state["trigger_reset_unix"])
	require.Equal(t, []string{"5h"}, state["windows"])

	// The saved state stops a second send for the same reset.
	repo.accounts[PlatformOpenAI][0].Extra[AccountWindowPrimeStateExtraKey] = map[string]any{
		"trigger_reset_unix": float64(now.Add(-time.Hour).Unix()), "status": "succeeded", "attempts": float64(1),
		"last_attempt_at": now.Format(time.RFC3339),
	}
	runner.runOnce(context.Background())
	require.Len(t, *sends, 1)
}

func TestAccountWindowPrime_RunnerRecordsFailureAndAnthropicWindow(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	sessionEnd := now.Add(-time.Hour)
	repo := &windowPrimeRepoStub{accounts: map[string][]Account{PlatformAnthropic: {{
		ID: 9, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		SessionWindowEnd: &sessionEnd, Extra: map[string]any{AccountWindowPrimeExtraKey: map[string]any{"mode": "on", "prompts": []any{"only"}}},
	}}}}
	runner, _ := newWindowPrimeTestRunner(t, repo, "", now)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-status", "allowed")
	headers.Set("anthropic-ratelimit-unified-5h-reset", "1792000000")
	runner.send = func(_ context.Context, _ *Account, model, prompt string) (http.Header, error) {
		require.Equal(t, "claude-haiku-4-5-20251001", model)
		require.Equal(t, "only", prompt)
		return headers, errors.New("upstream said no")
	}

	runner.runOnce(context.Background())
	state := repo.extraUpdates[9][AccountWindowPrimeStateExtraKey].(map[string]any)
	require.Equal(t, "failed", state["status"])
	require.Equal(t, "upstream said no", state["error"])
	require.Equal(t, 1, state["attempts"])
	require.Equal(t, 1, repo.sessionWindowCalls)
}

func TestAccountWindowPrime_SkipsOutsideActiveHours(t *testing.T) {
	now := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	settings := `{"platforms":{"openai":{"enabled":true}},"delay_min_minutes":1,"delay_max_minutes":5,"active_hours_enabled":true,"active_hours_start":"08:00","active_hours_end":"09:00","prompts":["Hi"]}`
	repo := &windowPrimeRepoStub{accounts: map[string][]Account{PlatformOpenAI: {openAIWindowPrimeAccount(1, now.Add(-time.Hour), nil)}}}
	runner, sends := newWindowPrimeTestRunner(t, repo, settings, now)
	runner.runOnce(context.Background())
	require.Empty(t, *sends)
}

func TestAccountWindowPrime_TestPayloadPrompt(t *testing.T) {
	ctx := withAccountWindowPrimeRequest(context.Background(), accountWindowPrimeRequest{prompt: "早上好"})
	openAIPayload := createOpenAITestPayload("gpt-5.4-mini", true)
	applyAccountWindowPrimeTestPayload(ctx, openAIPayload, "input")
	require.Equal(t, "早上好", openAIPayload["input"].([]map[string]any)[0]["content"].([]map[string]any)[0]["text"])

	claudePayload, err := createTestPayload("claude-haiku-4-5-20251001")
	require.NoError(t, err)
	applyAccountWindowPrimeTestPayload(ctx, claudePayload, "messages")
	require.Equal(t, "早上好", claudePayload["messages"].([]map[string]any)[0]["content"].([]map[string]any)[0]["text"])

	untouched := createOpenAITestPayload("gpt-5.4-mini", true)
	applyAccountWindowPrimeTestPayload(context.Background(), untouched, "input")
	require.Equal(t, "hi", untouched["input"].([]map[string]any)[0]["content"].([]map[string]any)[0]["text"])

	var got http.Header
	notifyAccountWindowPrimeHeaders(withAccountWindowPrimeRequest(context.Background(), accountWindowPrimeRequest{onHeaders: func(h http.Header) { got = h }}), http.Header{"X": {"1"}})
	require.Equal(t, "1", got.Get("X"))
}

func TestAccountWindowPrime_SettingServiceRoundTrip(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})
	require.Equal(t, DefaultAccountWindowPrimeSettings(), svc.GetAccountWindowPrimeSettings(context.Background()))

	input := DefaultAccountWindowPrimeSettings()
	input.Platforms[PlatformOpenAI] = AccountWindowPrimePlatformSettings{Enabled: true, Model: "gpt-x"}
	input.Prompts = []string{"one", "two"}
	saved, err := svc.SetAccountWindowPrimeSettings(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, saved, svc.GetAccountWindowPrimeSettings(context.Background()))

	input.DelayMinMinutes = -1
	_, err = svc.SetAccountWindowPrimeSettings(context.Background(), input)
	require.Error(t, err)
}
