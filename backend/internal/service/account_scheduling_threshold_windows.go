package service

import (
	"encoding/json"
	"fmt"
)

// Per-window scheduling thresholds: each usage window (5h, 7d, Fable 7d_oi,
// weekly, ...) may carry its own fixed threshold or random range instead of
// sharing one value. Lookup order for a window is: the account's rule for that
// window, the account default, the platform's rule for that window, then the
// platform default. Anything set on the account wins over the platform.

const (
	// AccountSchedulingThresholdWindowsCredentialKey stores an account's
	// per-window rules as {"<window>": {"mode": ..., ...}}.
	AccountSchedulingThresholdWindowsCredentialKey = "account_scheduling_threshold_windows"

	// SettingKeyAccountSchedulingThresholdWindowRules stores platform per-window
	// rules as {"<platform>": {"<window>": {...}}}.
	SettingKeyAccountSchedulingThresholdWindowRules = "account_scheduling_threshold_window_rules"

	AccountSchedulingThresholdModeFixed  = "fixed"
	AccountSchedulingThresholdModeRandom = "random"
)

// AccountSchedulingThresholdPlatformWindows lists the windows the evaluator
// produces for each platform, in display order.
var AccountSchedulingThresholdPlatformWindows = map[string][]string{
	PlatformOpenAI:      {"5h", "7d"},
	PlatformAnthropic:   {"5h", "7d", "7d_oi"},
	PlatformGrok:        {"quota"},
	PlatformKimi:        {"5h", "weekly"},
	PlatformZhipu:       {"5h", "weekly"},
	PlatformMiniMax:     {"5h", "weekly"},
	PlatformOpenCodeGo:  {"5h", "weekly", "monthly"},
	PlatformCommandCode: {"5h", "weekly", "monthly"},
}

// AccountSchedulingThresholdWindowRule is one window's own rule.
type AccountSchedulingThresholdWindowRule struct {
	Mode             string `json:"mode"`
	Threshold        int    `json:"threshold,omitempty"`
	Min              int    `json:"min,omitempty"`
	Max              int    `json:"max,omitempty"`
	FullMonthlyLimit int    `json:"full_monthly_limit,omitempty"`
}

func (r AccountSchedulingThresholdWindowRule) toRule() (accountSchedulingThresholdRule, bool) {
	switch r.Mode {
	case AccountSchedulingThresholdModeFixed:
		if r.Threshold < 1 || r.Threshold > 100 {
			return accountSchedulingThresholdRule{}, false
		}
		return accountSchedulingThresholdRule{fixed: r.Threshold}, true
	case AccountSchedulingThresholdModeRandom:
		rng := AccountSchedulingThresholdRange{Enabled: true, Min: r.Min, Max: r.Max, FullMonthlyLimit: r.FullMonthlyLimit}
		if !rng.valid() {
			return accountSchedulingThresholdRule{}, false
		}
		return accountSchedulingThresholdRule{random: &rng}, true
	default:
		return accountSchedulingThresholdRule{}, false
	}
}

func isAccountSchedulingThresholdPlatformWindow(platform, window string) bool {
	for _, known := range AccountSchedulingThresholdPlatformWindows[platform] {
		if known == window {
			return true
		}
	}
	return false
}

// resolveAccountSchedulingThresholdWindowRule returns the rule for one window and
// whether it came from a window-specific setting.
func resolveAccountSchedulingThresholdWindowRule(account *Account, policy AccountSchedulingThresholdPolicy, platform, window string) (rule accountSchedulingThresholdRule, windowSpecific, ok bool) {
	if rule, ok := accountSchedulingThresholdWindowOverride(account, window); ok {
		return rule, true, true
	}
	if rule, ok := accountSchedulingThresholdDefaultOverride(account); ok {
		return rule, false, true
	}
	if stored, found := policy.WindowRules[platform][window]; found {
		if rule, ok := stored.toRule(); ok {
			return rule, true, true
		}
	}
	rule, ok = platformSchedulingThresholdDefault(policy, platform)
	return rule, false, ok
}

// hasAccountSchedulingThresholdWindowRules reports whether any window of the
// account may have its own rule, which can pause even when the default cannot.
func hasAccountSchedulingThresholdWindowRules(account *Account, policy AccountSchedulingThresholdPolicy, platform string) bool {
	if len(policy.WindowRules[platform]) > 0 {
		return true
	}
	if account == nil {
		return false
	}
	values, ok := account.Credentials[AccountSchedulingThresholdWindowsCredentialKey].(map[string]any)
	return ok && len(values) > 0
}

func accountSchedulingThresholdWindowOverride(account *Account, window string) (accountSchedulingThresholdRule, bool) {
	if account == nil || window == "" {
		return accountSchedulingThresholdRule{}, false
	}
	values, ok := account.Credentials[AccountSchedulingThresholdWindowsCredentialKey].(map[string]any)
	if !ok {
		return accountSchedulingThresholdRule{}, false
	}
	rule, ok := parseAccountSchedulingThresholdWindowRule(values[window])
	if !ok {
		return accountSchedulingThresholdRule{}, false
	}
	return rule.toRule()
}

func parseAccountSchedulingThresholdWindowRule(raw any) (AccountSchedulingThresholdWindowRule, bool) {
	values, ok := raw.(map[string]any)
	if !ok {
		return AccountSchedulingThresholdWindowRule{}, false
	}
	rule := AccountSchedulingThresholdWindowRule{}
	rule.Mode, _ = values["mode"].(string)
	switch rule.Mode {
	case AccountSchedulingThresholdModeFixed:
		rule.Threshold, ok = parseAccountSchedulingThresholdValue(values["threshold"])
		return rule, ok
	case AccountSchedulingThresholdModeRandom:
		r, ok := parseAccountSchedulingThresholdRange(values)
		rule.Min, rule.Max, rule.FullMonthlyLimit = r.Min, r.Max, r.FullMonthlyLimit
		return rule, ok
	default:
		return rule, false
	}
}

func validateAndNormalizeAccountSchedulingThresholdWindowRules(input map[string]map[string]AccountSchedulingThresholdWindowRule) (map[string]map[string]AccountSchedulingThresholdWindowRule, error) {
	normalized := make(map[string]map[string]AccountSchedulingThresholdWindowRule, len(input))
	for platform, windows := range input {
		if !isAllowedSchedulingThresholdPlatform(platform) {
			return nil, fmt.Errorf("unknown platform %q", platform)
		}
		for window, rule := range windows {
			if !isAccountSchedulingThresholdPlatformWindow(platform, window) {
				return nil, fmt.Errorf("%s has no window %q", platform, window)
			}
			if _, ok := rule.toRule(); !ok {
				return nil, fmt.Errorf("%s %s: use a fixed threshold 1-100 or a valid random range", platform, window)
			}
			if normalized[platform] == nil {
				normalized[platform] = map[string]AccountSchedulingThresholdWindowRule{}
			}
			normalized[platform][window] = rule
		}
	}
	return normalized, nil
}

func parseAccountSchedulingThresholdWindowRulesSetting(raw string) map[string]map[string]AccountSchedulingThresholdWindowRule {
	parsed := map[string]map[string]AccountSchedulingThresholdWindowRule{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]map[string]AccountSchedulingThresholdWindowRule{}
	}
	out := make(map[string]map[string]AccountSchedulingThresholdWindowRule, len(parsed))
	for platform, windows := range parsed {
		for window, rule := range windows {
			if _, ok := rule.toRule(); !ok || !isAccountSchedulingThresholdPlatformWindow(platform, window) {
				continue
			}
			if out[platform] == nil {
				out[platform] = map[string]AccountSchedulingThresholdWindowRule{}
			}
			out[platform][window] = rule
		}
	}
	return out
}

func cloneAccountSchedulingThresholdWindowRules(input map[string]map[string]AccountSchedulingThresholdWindowRule) map[string]map[string]AccountSchedulingThresholdWindowRule {
	cloned := make(map[string]map[string]AccountSchedulingThresholdWindowRule, len(input))
	for platform, windows := range input {
		cloned[platform] = make(map[string]AccountSchedulingThresholdWindowRule, len(windows))
		for window, rule := range windows {
			cloned[platform][window] = rule
		}
	}
	return cloned
}
