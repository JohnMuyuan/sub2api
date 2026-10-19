//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func windowRuleTestAccount(credentials map[string]any, used5h, used7d float64, now time.Time) *Account {
	return &Account{
		ID:          801,
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: credentials,
		Extra: map[string]any{
			"codex_5h_used_percent": used5h,
			"codex_5h_reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
			"codex_7d_used_percent": used7d,
			"codex_7d_reset_at":     now.Add(72 * time.Hour).Format(time.RFC3339),
		},
	}
}

func TestPerWindowSchedulingThreshold_AccountWindowOverridesDefault(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	credentials := map[string]any{
		"account_scheduling_threshold": 95,
		AccountSchedulingThresholdWindowsCredentialKey: map[string]any{
			"5h": map[string]any{"mode": "fixed", "threshold": 80},
		},
	}
	policy := AccountSchedulingThresholdPolicy{Location: time.UTC}

	decision := EvaluateAccountSchedulingThresholdWithPolicy(windowRuleTestAccount(credentials, 85, 90, now), policy, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, "5h", decision.Window)
	require.Equal(t, 80, decision.ThresholdPercent)

	// 7d keeps the account default of 95.
	decision = EvaluateAccountSchedulingThresholdWithPolicy(windowRuleTestAccount(credentials, 10, 94, now), policy, now)
	require.False(t, decision.ShouldPause)
	decision = EvaluateAccountSchedulingThresholdWithPolicy(windowRuleTestAccount(credentials, 10, 95, now), policy, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, "7d", decision.Window)
	require.Equal(t, 95, decision.ThresholdPercent)
}

func TestPerWindowSchedulingThreshold_WindowRuleWorksWithoutDefault(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	// Platform default 100 never pauses; only the 7d rule can.
	policy := AccountSchedulingThresholdPolicy{
		Thresholds: map[string]int{PlatformOpenAI: 100},
		WindowRules: map[string]map[string]AccountSchedulingThresholdWindowRule{
			PlatformOpenAI: {"7d": {Mode: "fixed", Threshold: 70}},
		},
		Location: time.UTC,
	}
	decision := EvaluateAccountSchedulingThresholdWithPolicy(windowRuleTestAccount(nil, 99, 70, now), policy, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, "7d", decision.Window)
	require.Equal(t, 70, decision.ThresholdPercent)

	// Anything set on the account wins over the platform's window rule.
	decision = EvaluateAccountSchedulingThresholdWithPolicy(windowRuleTestAccount(map[string]any{"account_scheduling_threshold": 90}, 10, 80, now), policy, now)
	require.False(t, decision.ShouldPause)
}

func TestPerWindowSchedulingThreshold_FixedHundredNeverPauses(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	credentials := map[string]any{
		"account_scheduling_threshold": 50,
		AccountSchedulingThresholdWindowsCredentialKey: map[string]any{
			"5h": map[string]any{"mode": "fixed", "threshold": 100},
			"7d": map[string]any{"mode": "fixed", "threshold": 100},
		},
	}
	decision := EvaluateAccountSchedulingThresholdWithPolicy(windowRuleTestAccount(credentials, 100, 100, now), AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.False(t, decision.ShouldPause)
}

func TestPerWindowSchedulingThreshold_RandomWindowRule(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	credentials := map[string]any{
		"account_scheduling_threshold": 95,
		AccountSchedulingThresholdWindowsCredentialKey: map[string]any{
			"7d": map[string]any{"mode": "random", "min": 60, "max": 70, "full_monthly_limit": 0},
		},
	}
	account := windowRuleTestAccount(credentials, 10, 0, now)
	rounds := AccountSchedulingThresholdRounds(account, AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.Len(t, rounds, 2)
	require.Equal(t, "5h", rounds[0].Window)
	require.Equal(t, "fixed", rounds[0].Mode)
	require.Equal(t, 95, rounds[0].ThresholdPercent)
	require.Equal(t, "7d", rounds[1].Window)
	require.Equal(t, "random", rounds[1].Mode)
	require.GreaterOrEqual(t, rounds[1].ThresholdPercent, 60)
	require.LessOrEqual(t, rounds[1].ThresholdPercent, 70)
	require.Equal(t, 60, rounds[1].RangeMin)

	account.Extra["codex_7d_used_percent"] = float64(rounds[1].ThresholdPercent)
	decision := EvaluateAccountSchedulingThresholdWithPolicy(account, AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, rounds[1].ThresholdPercent, decision.ThresholdPercent)
}

func TestPerWindowSchedulingThreshold_RoundsHiddenForPlainFixedDefault(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	account := windowRuleTestAccount(map[string]any{"account_scheduling_threshold": 90}, 10, 10, now)
	require.Nil(t, AccountSchedulingThresholdRounds(account, AccountSchedulingThresholdPolicy{Location: time.UTC}, now))
}

func TestPerWindowSchedulingThreshold_FableWindowRule(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{
		ID:       802,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"passive_usage_7d_oi_utilization": 0.55,
			"passive_usage_7d_oi_reset":       float64(now.Add(48 * time.Hour).Unix()),
		},
	}
	policy := AccountSchedulingThresholdPolicy{
		Thresholds: map[string]int{PlatformAnthropic: 100},
		WindowRules: map[string]map[string]AccountSchedulingThresholdWindowRule{
			PlatformAnthropic: {"7d_oi": {Mode: "fixed", Threshold: 50}},
		},
		Location: time.UTC,
	}
	decision := evaluateAnthropicFableSchedulingThresholdWithPolicy(account, policy, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, "7d_oi", decision.Window)
	require.Equal(t, 50, decision.ThresholdPercent)
	// The account-level Fable decision is separate from the whole-account one.
	require.False(t, EvaluateAccountSchedulingThresholdWithPolicy(account, policy, now).ShouldPause)
}

func TestPerWindowSchedulingThreshold_InvalidAccountRulesFallBack(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	credentials := map[string]any{
		"account_scheduling_threshold": 90,
		AccountSchedulingThresholdWindowsCredentialKey: map[string]any{
			"5h": map[string]any{"mode": "fixed", "threshold": 0},
			"7d": map[string]any{"mode": "random", "min": 80, "max": 70},
		},
	}
	decision := EvaluateAccountSchedulingThresholdWithPolicy(windowRuleTestAccount(credentials, 91, 10, now), AccountSchedulingThresholdPolicy{Location: time.UTC}, now)
	require.True(t, decision.ShouldPause)
	require.Equal(t, 90, decision.ThresholdPercent)
}

func TestPerWindowSchedulingThreshold_SettingsValidationAndRoundTrip(t *testing.T) {
	for name, rules := range map[string]map[string]map[string]AccountSchedulingThresholdWindowRule{
		"unknown platform": {"gemini": {"5h": {Mode: "fixed", Threshold: 90}}},
		"unknown window":   {PlatformOpenAI: {"7d_oi": {Mode: "fixed", Threshold: 90}}},
		"bad fixed":        {PlatformOpenAI: {"5h": {Mode: "fixed", Threshold: 101}}},
		"bad random":       {PlatformOpenAI: {"5h": {Mode: "random", Min: 95, Max: 90}}},
		"bad mode":         {PlatformOpenAI: {"5h": {Mode: "other"}}},
	} {
		_, err := validateAndNormalizeAccountSchedulingThresholdWindowRules(rules)
		require.Error(t, err, name)
	}

	accountSchedulingThresholdRangesCache.Store(nil)
	t.Cleanup(func() { accountSchedulingThresholdRangesCache.Store(nil) })
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})
	rules := map[string]map[string]AccountSchedulingThresholdWindowRule{
		PlatformAnthropic: {"7d_oi": {Mode: "random", Min: 80, Max: 90, FullMonthlyLimit: 1}},
		PlatformKimi:      {"weekly": {Mode: "fixed", Threshold: 85}},
	}
	_, savedRules, err := svc.SetAccountSchedulingThresholdRanges(context.Background(), map[string]AccountSchedulingThresholdRange{}, rules)
	require.NoError(t, err)
	require.Equal(t, rules, savedRules)
	require.Equal(t, rules, svc.GetAccountSchedulingThresholdPolicy(context.Background()).WindowRules)

	// Saving ranges without window_rules keeps the stored rules.
	_, kept, err := svc.SetAccountSchedulingThresholdRanges(context.Background(), map[string]AccountSchedulingThresholdRange{
		PlatformOpenAI: {Enabled: true, Min: 90, Max: 97},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, rules, kept)

	parsed := parseAccountSchedulingThresholdWindowRulesSetting(`{"openai":{"5h":{"mode":"fixed","threshold":80},"7d_oi":{"mode":"fixed","threshold":80},"7d":{"mode":"fixed","threshold":0}}}`)
	require.Equal(t, map[string]map[string]AccountSchedulingThresholdWindowRule{PlatformOpenAI: {"5h": {Mode: "fixed", Threshold: 80}}}, parsed)
}
