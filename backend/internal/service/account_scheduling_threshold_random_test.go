//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func randomThresholdTestAccount(id int64, r map[string]any, extra map[string]any) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"account_scheduling_threshold":               r["min"],
			AccountSchedulingThresholdRangeCredentialKey: r,
		},
		Extra: extra,
	}
}

// findResetDrawing returns a 5h reset time after start whose draw equals want.
func findResetDrawing(t *testing.T, account *Account, r AccountSchedulingThresholdRange, start time.Time, want int) time.Time {
	t.Helper()
	free := r
	free.FullMonthlyLimit = 0
	for i := 0; i < 2000; i++ {
		until := start.Add(time.Duration(i) * time.Hour)
		if drawAccountSchedulingThreshold(account, free, "5h", "", until, time.UTC).percent == want {
			return until
		}
	}
	t.Fatalf("no window drew %d", want)
	return time.Time{}
}

func TestRandomSchedulingThreshold_DrawIsStableWithinWindowAndInRange(t *testing.T) {
	account := &Account{ID: 42}
	r := AccountSchedulingThresholdRange{Min: 90, Max: 97}
	start := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	seen := map[int]bool{}
	for i := 0; i < 500; i++ {
		until := start.Add(time.Duration(i) * 5 * time.Hour)
		first := drawAccountSchedulingThreshold(account, r, "5h", "", until, time.UTC).percent
		require.GreaterOrEqual(t, first, 90)
		require.LessOrEqual(t, first, 97)
		// Reset times rebuilt from "seconds until reset" drift by seconds.
		for _, drift := range []time.Duration{-3 * time.Second, 2 * time.Second, 20 * time.Minute} {
			require.Equal(t, first, drawAccountSchedulingThreshold(account, r, "5h", "", until.Add(drift), time.UTC).percent)
		}
		seen[first] = true
	}
	require.Len(t, seen, 8, "every value in 90..97 should come up across windows")
}

func TestRandomSchedulingThreshold_WindowsAndAccountsDrawIndependently(t *testing.T) {
	r := AccountSchedulingThresholdRange{Min: 1, Max: 100}
	until := time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC)
	values := map[int]bool{}
	for id := int64(1); id <= 50; id++ {
		values[drawAccountSchedulingThreshold(&Account{ID: id}, r, "5h", "", until, time.UTC).percent] = true
	}
	require.Greater(t, len(values), 20)
}

func TestRandomSchedulingThreshold_FullDrawMonthlyLimit(t *testing.T) {
	account := &Account{ID: 7}
	r := AccountSchedulingThresholdRange{Min: 95, Max: 100, FullMonthlyLimit: 1}
	start := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	first := findResetDrawing(t, account, r, start, 100)
	second := findResetDrawing(t, account, r, first.Add(5*time.Hour), 100)
	require.Equal(t, first.Format("2006-01"), second.Format("2006-01"), "test needs two full draws in one month")

	// No record yet: the first full draw is granted and must be saved.
	draw := drawAccountSchedulingThreshold(account, r, "5h", "", first, time.UTC)
	require.Equal(t, 100, draw.percent)
	require.Equal(t, "scheduling_threshold_full_draws_5h", draw.recordKey)
	require.Equal(t, 1, draw.fullDrawsUsed)

	account.Extra = map[string]any{draw.recordKey: map[string]any{
		"month":   "2026-10",
		"windows": []any{float64(accountSchedulingThresholdWindowBucketOf(first))},
	}}
	// Same window again: still 100 and nothing new to save.
	again := drawAccountSchedulingThreshold(account, r, "5h", "", first, time.UTC)
	require.Equal(t, 100, again.percent)
	require.Empty(t, again.recordKey)

	// Limit used up: another full draw this month is re-drawn below 100.
	limited := drawAccountSchedulingThreshold(account, r, "5h", "", second, time.UTC)
	require.GreaterOrEqual(t, limited.percent, 95)
	require.LessOrEqual(t, limited.percent, 99)
	require.Empty(t, limited.recordKey)
	require.Equal(t, 1, limited.fullDrawsUsed)

	// Other window kinds keep their own count.
	weekly := drawAccountSchedulingThreshold(account, AccountSchedulingThresholdRange{Min: 100, Max: 100, FullMonthlyLimit: 1}, "7d", "", second, time.UTC)
	require.Equal(t, 100, weekly.percent)
	require.Equal(t, "scheduling_threshold_full_draws_7d", weekly.recordKey)

	// A new calendar month starts from zero.
	next := findResetDrawing(t, account, r, time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), 100)
	fresh := drawAccountSchedulingThreshold(account, r, "5h", "", next, time.UTC)
	require.Equal(t, 100, fresh.percent)
	require.Equal(t, map[string]any{"month": "2026-11", "windows": []int64{accountSchedulingThresholdWindowBucketOf(next)}}, fresh.recordValue)
}

func TestRandomSchedulingThreshold_UnlimitedFullDrawsAreNotRecorded(t *testing.T) {
	account := &Account{ID: 9}
	r := AccountSchedulingThresholdRange{Min: 99, Max: 100}
	until := findResetDrawing(t, account, r, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), 100)
	draw := drawAccountSchedulingThreshold(account, r, "5h", "", until, time.UTC)
	require.Equal(t, 100, draw.percent)
	require.Empty(t, draw.recordKey)
}

func TestRandomSchedulingThreshold_MonthFollowsSiteTimezone(t *testing.T) {
	taipei, err := time.LoadLocation("Asia/Taipei")
	require.NoError(t, err)
	account := &Account{ID: 11}
	r := AccountSchedulingThresholdRange{Min: 100, Max: 100, FullMonthlyLimit: 3}
	// 2026-10-31 18:00 UTC is already November in Taipei.
	draw := drawAccountSchedulingThreshold(account, r, "5h", "", time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC), taipei)
	require.Equal(t, "2026-11", draw.recordValue["month"])
}

func TestRandomSchedulingThreshold_EvaluatePausesAtDrawnThreshold(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	r := AccountSchedulingThresholdRange{Min: 90, Max: 97}
	probe := &Account{ID: 501}
	until := findResetDrawing(t, probe, r, now.Add(time.Hour), 94)
	rangeCreds := map[string]any{"min": 90, "max": 97, "full_monthly_limit": 0}

	below := randomThresholdTestAccount(501, rangeCreds, map[string]any{
		"codex_5h_used_percent": 93.9,
		"codex_5h_reset_at":     until.Format(time.RFC3339),
	})
	decision := EvaluateAccountSchedulingThresholdWithPolicy(below, AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.False(t, decision.ShouldPause)

	at := randomThresholdTestAccount(501, rangeCreds, map[string]any{
		"codex_5h_used_percent": 94.0,
		"codex_5h_reset_at":     until.Format(time.RFC3339),
	})
	decision = EvaluateAccountSchedulingThresholdWithPolicy(at, AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, 94, decision.ThresholdPercent)
	require.Equal(t, "5h", decision.Window)
}

func TestRandomSchedulingThreshold_FullDrawDoesNotPauseAndIsRecorded(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	r := AccountSchedulingThresholdRange{Min: 98, Max: 100, FullMonthlyLimit: 2}
	until := findResetDrawing(t, &Account{ID: 502}, r, now.Add(time.Hour), 100)
	account := randomThresholdTestAccount(502, map[string]any{"min": 98, "max": 100, "full_monthly_limit": 2}, map[string]any{
		"codex_5h_used_percent": 100.0,
		"codex_5h_reset_at":     until.Format(time.RFC3339),
	})

	decision := EvaluateAccountSchedulingThresholdWithPolicy(account, AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.False(t, decision.ShouldPause)
	require.Contains(t, decision.FullDrawExtraUpdates, "scheduling_threshold_full_draws_5h")
}

func TestRandomSchedulingThreshold_RulePrecedence(t *testing.T) {
	policy := AccountSchedulingThresholdPolicy{
		Thresholds: map[string]int{PlatformOpenAI: 80},
		Ranges: map[string]AccountSchedulingThresholdRange{
			PlatformOpenAI: {Enabled: true, Min: 85, Max: 95},
			PlatformGrok:   {Enabled: false, Min: 85, Max: 95},
		},
	}

	rule, ok := resolveAccountSchedulingThresholdRule(&Account{}, policy, PlatformOpenAI)
	require.True(t, ok)
	require.Equal(t, &AccountSchedulingThresholdRange{Enabled: true, Min: 85, Max: 95}, rule.random)

	rule, _ = resolveAccountSchedulingThresholdRule(&Account{Credentials: map[string]any{"account_scheduling_threshold": 70}}, policy, PlatformOpenAI)
	require.Nil(t, rule.random)
	require.Equal(t, 70, rule.fixed)

	rule, _ = resolveAccountSchedulingThresholdRule(&Account{Credentials: map[string]any{
		"account_scheduling_threshold":               60,
		AccountSchedulingThresholdRangeCredentialKey: map[string]any{"min": 60, "max": 65},
	}}, policy, PlatformOpenAI)
	require.Equal(t, 60, rule.random.Min)
	require.Equal(t, 65, rule.random.Max)

	// Disabled platform range falls back to the fixed platform threshold.
	rule, ok = resolveAccountSchedulingThresholdRule(&Account{}, policy, PlatformGrok)
	require.False(t, ok)
	require.Nil(t, rule.random)

	// Invalid account range is ignored.
	rule, _ = resolveAccountSchedulingThresholdRule(&Account{Credentials: map[string]any{
		AccountSchedulingThresholdRangeCredentialKey: map[string]any{"min": 96, "max": 90},
	}}, policy, PlatformOpenAI)
	require.Equal(t, 85, rule.random.Min)
}

func TestRandomSchedulingThreshold_FixedThresholdsUnchanged(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{ID: 503, Platform: PlatformOpenAI, Extra: map[string]any{
		"codex_7d_used_percent": 85.0,
		"codex_7d_reset_at":     now.Add(48 * time.Hour).Format(time.RFC3339),
	}}
	decision := EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenAI: 85}, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, 85, decision.ThresholdPercent)
	require.Nil(t, decision.FullDrawExtraUpdates)
	require.Nil(t, AccountSchedulingThresholdRounds(account, AccountSchedulingThresholdPolicy{Thresholds: map[string]int{PlatformOpenAI: 85}}, now))
}

func TestRandomSchedulingThreshold_RoundsListCurrentWindows(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(3 * time.Hour)
	reset7d := now.Add(72 * time.Hour)
	account := randomThresholdTestAccount(504, map[string]any{"min": 90, "max": 97, "full_monthly_limit": 2}, map[string]any{
		"codex_5h_used_percent": 40.0,
		"codex_5h_reset_at":     reset5h.Format(time.RFC3339),
		"codex_7d_used_percent": 10.0,
		"codex_7d_reset_at":     reset7d.Format(time.RFC3339),
	})

	rounds := AccountSchedulingThresholdRounds(account, AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.Len(t, rounds, 2)
	require.Equal(t, "5h", rounds[0].Window)
	require.Equal(t, "7d", rounds[1].Window)
	for _, round := range rounds {
		require.GreaterOrEqual(t, round.ThresholdPercent, 90)
		require.LessOrEqual(t, round.ThresholdPercent, 97)
		require.Equal(t, 90, round.RangeMin)
		require.Equal(t, 97, round.RangeMax)
		require.Equal(t, 2, round.FullMonthlyLimit)
	}
	require.Equal(t, 40.0, rounds[0].UsedPercent)
}

func TestRandomSchedulingThreshold_ApplyRecordsFullDrawOnce(t *testing.T) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})
	accountSchedulingThresholdRangesCache.Store(nil)
	t.Cleanup(func() { accountSchedulingThresholdRangesCache.Store(nil) })

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"openai":100}`
	settingsRepo.data[SettingKeyAccountSchedulingThresholdRanges] = `{"openai":{"enabled":true,"min":99,"max":100,"full_monthly_limit":5}}`
	accountRepo := &rateLimitAccountRepoStub{}
	rl := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	rl.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))

	account := &Account{
		ID:          90001,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"codex_5h_used_percent": 100.0,
			"codex_5h_reset_at": findResetDrawing(t, &Account{ID: 90001}, AccountSchedulingThresholdRange{Min: 99, Max: 100},
				time.Now().UTC().Add(time.Hour), 100).Format(time.RFC3339),
		},
	}

	require.False(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.False(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.Equal(t, 1, accountRepo.updateExtraCalls)
	require.Contains(t, accountRepo.lastExtraUpdates, "scheduling_threshold_full_draws_5h")
	require.Zero(t, accountRepo.tempCalls)
}

func TestRandomSchedulingThreshold_SettingsValidation(t *testing.T) {
	_, err := validateAndNormalizeAccountSchedulingThresholdRanges(map[string]AccountSchedulingThresholdRange{
		PlatformOpenAI: {Enabled: true, Min: 97, Max: 90},
	})
	require.Error(t, err)

	_, err = validateAndNormalizeAccountSchedulingThresholdRanges(map[string]AccountSchedulingThresholdRange{
		"gemini": {Enabled: true, Min: 90, Max: 97},
	})
	require.Error(t, err)

	_, err = validateAndNormalizeAccountSchedulingThresholdRanges(map[string]AccountSchedulingThresholdRange{
		PlatformOpenAI: {Enabled: true, Min: 90, Max: 100, FullMonthlyLimit: -1},
	})
	require.Error(t, err)

	normalized, err := validateAndNormalizeAccountSchedulingThresholdRanges(map[string]AccountSchedulingThresholdRange{
		PlatformOpenAI: {Enabled: true, Min: 90, Max: 100, FullMonthlyLimit: 2},
		PlatformGrok:   {Enabled: false, Min: 0, Max: 0},
	})
	require.NoError(t, err)
	require.Len(t, normalized, 2)

	parsed := parseAccountSchedulingThresholdRangesSetting(`{"openai":{"enabled":true,"min":90,"max":97},"anthropic":{"enabled":true,"min":99,"max":1},"gemini":{"enabled":true,"min":1,"max":2}}`)
	require.Equal(t, map[string]AccountSchedulingThresholdRange{PlatformOpenAI: {Enabled: true, Min: 90, Max: 97}}, parsed)
}

func TestRandomSchedulingThreshold_SettingServiceRoundTrip(t *testing.T) {
	accountSchedulingThresholdRangesCache.Store(nil)
	t.Cleanup(func() { accountSchedulingThresholdRangesCache.Store(nil) })
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	require.Empty(t, svc.GetAccountSchedulingThresholdRanges(context.Background()))
	saved, err := svc.SetAccountSchedulingThresholdRanges(context.Background(), map[string]AccountSchedulingThresholdRange{
		PlatformAnthropic: {Enabled: true, Min: 88, Max: 96, FullMonthlyLimit: 1},
	})
	require.NoError(t, err)
	require.Equal(t, saved, svc.GetAccountSchedulingThresholdRanges(context.Background()))
	require.Equal(t, 88, svc.GetAccountSchedulingThresholdPolicy(context.Background()).Ranges[PlatformAnthropic].Min)
}

func TestRandomSchedulingThreshold_ExpiredWindowDoesNotUseMonthlyLimit(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	r := AccountSchedulingThresholdRange{Min: 99, Max: 100, FullMonthlyLimit: 1}
	expired := findResetDrawing(t, &Account{ID: 505}, r, now.Add(-200*time.Hour), 100)
	require.False(t, expired.After(now))
	account := &Account{
		ID:       505,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			AccountSchedulingThresholdRangeCredentialKey: map[string]any{"min": 99, "max": 100, "full_monthly_limit": 1},
		},
		Extra:            map[string]any{"session_window_utilization": 0.5},
		SessionWindowEnd: &expired,
	}

	decision := EvaluateAccountSchedulingThresholdWithPolicy(account, AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.False(t, decision.ShouldPause)
	require.Nil(t, decision.FullDrawExtraUpdates)
}
