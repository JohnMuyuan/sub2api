package service

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Window priming: OpenAI Codex and Anthropic usage windows (5h, weekly) only
// start counting at the first request after a reset. When a window has reset
// and nobody used the account since, the runner sends one small request so the
// next window starts right away instead of whenever real traffic arrives.

const (
	// SettingKeyAccountWindowPrime stores the system-level priming settings.
	SettingKeyAccountWindowPrime = "account_window_prime_settings"
	// AccountWindowPrimeExtraKey stores an account's own priming override.
	AccountWindowPrimeExtraKey = "window_prime"
	// AccountWindowPrimeStateExtraKey records the last priming attempt.
	AccountWindowPrimeStateExtraKey = "window_prime_state"

	AccountWindowPrimeModeInherit = "inherit"
	AccountWindowPrimeModeOn      = "on"
	AccountWindowPrimeModeOff     = "off"

	accountWindowPrimeDefaultPrompt     = "Hi"
	accountWindowPrimeDefaultOpenAI     = "gpt-5.4-mini"
	accountWindowPrimeDefaultAnthropic  = "claude-haiku-4-5-20251001"
	MaxAccountWindowPrimePrompts        = 50
	MaxAccountWindowPrimePromptLength   = 200
	MaxAccountWindowPrimeModelLength    = 100
	MaxAccountWindowPrimeDelayMinutes   = 120
	accountWindowPrimeMaxAttempts       = 3
	accountWindowPrimeRetryInterval     = 5 * time.Minute
	accountWindowPrimeStatusSucceeded   = "succeeded"
	accountWindowPrimeStatusFailed      = "failed"
	accountWindowPrimeDefaultDelayMin   = 1
	accountWindowPrimeDefaultDelayMax   = 5
	accountWindowPrimeDefaultHoursStart = "08:00"
	accountWindowPrimeDefaultHoursEnd   = "24:00"
)

// AccountWindowPrimePlatforms are the platforms whose windows start on first use.
var AccountWindowPrimePlatforms = []string{PlatformOpenAI, PlatformAnthropic}

// AccountWindowPrimePlatformSettings is one platform's default.
type AccountWindowPrimePlatformSettings struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
}

// AccountWindowPrimeSettings is the system-level configuration.
type AccountWindowPrimeSettings struct {
	Platforms          map[string]AccountWindowPrimePlatformSettings `json:"platforms"`
	DelayMinMinutes    int                                           `json:"delay_min_minutes"`
	DelayMaxMinutes    int                                           `json:"delay_max_minutes"`
	ActiveHoursEnabled bool                                          `json:"active_hours_enabled"`
	ActiveHoursStart   string                                        `json:"active_hours_start"`
	ActiveHoursEnd     string                                        `json:"active_hours_end"`
	Prompts            []string                                      `json:"prompts"`
	DefaultModels      map[string]string                             `json:"default_models"`
}

// DefaultAccountWindowPrimeSettings is used until an administrator saves settings.
func DefaultAccountWindowPrimeSettings() AccountWindowPrimeSettings {
	platforms := make(map[string]AccountWindowPrimePlatformSettings, len(AccountWindowPrimePlatforms))
	for _, platform := range AccountWindowPrimePlatforms {
		platforms[platform] = AccountWindowPrimePlatformSettings{}
	}
	return AccountWindowPrimeSettings{
		Platforms:        platforms,
		DelayMinMinutes:  accountWindowPrimeDefaultDelayMin,
		DelayMaxMinutes:  accountWindowPrimeDefaultDelayMax,
		ActiveHoursStart: accountWindowPrimeDefaultHoursStart,
		ActiveHoursEnd:   accountWindowPrimeDefaultHoursEnd,
		Prompts:          []string{accountWindowPrimeDefaultPrompt},
		DefaultModels:    accountWindowPrimeDefaultModels(),
	}
}

func accountWindowPrimeDefaultModels() map[string]string {
	return map[string]string{
		PlatformOpenAI:    accountWindowPrimeDefaultOpenAI,
		PlatformAnthropic: accountWindowPrimeDefaultAnthropic,
	}
}

// NormalizeAccountWindowPrimeSettings validates input and fills defaults.
func NormalizeAccountWindowPrimeSettings(input AccountWindowPrimeSettings) (AccountWindowPrimeSettings, error) {
	out := DefaultAccountWindowPrimeSettings()
	for platform, value := range input.Platforms {
		if !isAccountWindowPrimePlatform(platform) {
			return out, fmt.Errorf("unknown platform %q", platform)
		}
		model := strings.TrimSpace(value.Model)
		if utf8.RuneCountInString(model) > MaxAccountWindowPrimeModelLength {
			return out, fmt.Errorf("%s: model is too long", platform)
		}
		out.Platforms[platform] = AccountWindowPrimePlatformSettings{Enabled: value.Enabled, Model: model}
	}
	if input.DelayMinMinutes < 0 || input.DelayMaxMinutes > MaxAccountWindowPrimeDelayMinutes || input.DelayMinMinutes > input.DelayMaxMinutes {
		return out, fmt.Errorf("delay must satisfy 0 <= min <= max <= %d minutes", MaxAccountWindowPrimeDelayMinutes)
	}
	out.DelayMinMinutes = input.DelayMinMinutes
	out.DelayMaxMinutes = input.DelayMaxMinutes
	if _, ok := parseAccountWindowPrimeClock(input.ActiveHoursStart, false); !ok {
		return out, fmt.Errorf("active_hours_start must be HH:MM")
	}
	if _, ok := parseAccountWindowPrimeClock(input.ActiveHoursEnd, true); !ok {
		return out, fmt.Errorf("active_hours_end must be HH:MM (24:00 allowed)")
	}
	out.ActiveHoursEnabled = input.ActiveHoursEnabled
	out.ActiveHoursStart = strings.TrimSpace(input.ActiveHoursStart)
	out.ActiveHoursEnd = strings.TrimSpace(input.ActiveHoursEnd)
	prompts, err := normalizeAccountWindowPrimePrompts(input.Prompts)
	if err != nil {
		return out, err
	}
	if len(prompts) > 0 {
		out.Prompts = prompts
	}
	return out, nil
}

func normalizeAccountWindowPrimePrompts(input []string) ([]string, error) {
	prompts := make([]string, 0, len(input))
	for _, prompt := range input {
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			continue
		}
		if utf8.RuneCountInString(prompt) > MaxAccountWindowPrimePromptLength {
			return nil, fmt.Errorf("each prompt must be at most %d characters", MaxAccountWindowPrimePromptLength)
		}
		prompts = append(prompts, prompt)
	}
	if len(prompts) > MaxAccountWindowPrimePrompts {
		return nil, fmt.Errorf("at most %d prompts", MaxAccountWindowPrimePrompts)
	}
	return prompts, nil
}

// parseAccountWindowPrimeSettings reads stored settings, falling back to
// defaults for anything unusable so a bad value never stops the runner.
func parseAccountWindowPrimeSettings(raw string) AccountWindowPrimeSettings {
	var stored AccountWindowPrimeSettings
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &stored) != nil {
		return DefaultAccountWindowPrimeSettings()
	}
	normalized, err := NormalizeAccountWindowPrimeSettings(stored)
	if err != nil {
		return DefaultAccountWindowPrimeSettings()
	}
	return normalized
}

func isAccountWindowPrimePlatform(platform string) bool {
	for _, allowed := range AccountWindowPrimePlatforms {
		if platform == allowed {
			return true
		}
	}
	return false
}

// accountWindowPrimeSupported reports accounts with first-use windows.
func accountWindowPrimeSupported(account *Account) bool {
	if account == nil || account.IsShadow() || account.IsSyntheticUITest() {
		return false
	}
	return account.IsOpenAIOAuth() || account.IsAnthropicOAuthOrSetupToken()
}

// accountWindowPrimeConfig is an account's own override in account.Extra.
type accountWindowPrimeConfig struct {
	Mode    string
	Model   string
	Prompts []string
}

func readAccountWindowPrimeConfig(account *Account) accountWindowPrimeConfig {
	config := accountWindowPrimeConfig{Mode: AccountWindowPrimeModeInherit}
	if account == nil {
		return config
	}
	values, ok := account.Extra[AccountWindowPrimeExtraKey].(map[string]any)
	if !ok {
		return config
	}
	switch mode, _ := values["mode"].(string); mode {
	case AccountWindowPrimeModeOn, AccountWindowPrimeModeOff:
		config.Mode = mode
	}
	if model, ok := values["model"].(string); ok {
		config.Model = strings.TrimSpace(model)
	}
	if raw, ok := values["prompts"].([]any); ok {
		prompts := make([]string, 0, len(raw))
		for _, item := range raw {
			if prompt, ok := item.(string); ok {
				prompts = append(prompts, prompt)
			}
		}
		// Invalid stored lists fall back to the system prompts.
		if normalized, err := normalizeAccountWindowPrimePrompts(prompts); err == nil {
			config.Prompts = normalized
		}
	}
	return config
}

// accountWindowPrimePlan is what the runner sends for one account.
type accountWindowPrimePlan struct {
	Model   string
	Prompts []string
}

// resolveAccountWindowPrimePlan returns the model and prompts when priming is
// enabled for the account, account override first.
func resolveAccountWindowPrimePlan(account *Account, settings AccountWindowPrimeSettings) (accountWindowPrimePlan, bool) {
	if !accountWindowPrimeSupported(account) {
		return accountWindowPrimePlan{}, false
	}
	platform := strings.ToLower(strings.TrimSpace(account.Platform))
	platformSettings := settings.Platforms[platform]
	config := readAccountWindowPrimeConfig(account)
	switch config.Mode {
	case AccountWindowPrimeModeOff:
		return accountWindowPrimePlan{}, false
	case AccountWindowPrimeModeInherit:
		if !platformSettings.Enabled {
			return accountWindowPrimePlan{}, false
		}
	}
	plan := accountWindowPrimePlan{Model: config.Model, Prompts: config.Prompts}
	if plan.Model == "" {
		plan.Model = platformSettings.Model
	}
	if plan.Model == "" {
		plan.Model = accountWindowPrimeDefaultModels()[platform]
	}
	if len(plan.Prompts) == 0 {
		plan.Prompts = settings.Prompts
	}
	if len(plan.Prompts) == 0 {
		plan.Prompts = []string{accountWindowPrimeDefaultPrompt}
	}
	return plan, true
}

func pickAccountWindowPrimePrompt(prompts []string) string {
	if len(prompts) == 0 {
		return accountWindowPrimeDefaultPrompt
	}
	return prompts[rand.IntN(len(prompts))]
}

// accountWindowPrimeIdleWindow is a window that reset without being used since.
type accountWindowPrimeIdleWindow struct {
	Window  string
	ResetAt time.Time
}

// accountWindowPrimeIdleWindows lists windows whose known reset already passed.
// Any request after a reset refreshes these snapshots, so a past reset means
// the account has not been used since.
func accountWindowPrimeIdleWindows(account *Account, now time.Time) []accountWindowPrimeIdleWindow {
	var idle []accountWindowPrimeIdleWindow
	add := func(window string, resetAt *time.Time) {
		if resetAt != nil && !resetAt.IsZero() && !resetAt.After(now) {
			idle = append(idle, accountWindowPrimeIdleWindow{Window: window, ResetAt: resetAt.UTC()})
		}
	}
	switch {
	case account.IsOpenAIOAuth():
		for _, window := range []string{"5h", "7d"} {
			if _, ok := account.Extra["codex_"+window+"_used_percent"]; !ok {
				continue
			}
			resetAt := parseSchedulingResetAt(account.Extra["codex_"+window+"_reset_at"])
			if resetAt == nil {
				if computed, ok := openAICodexWindowResetAt(account.Extra, window); ok {
					resetAt = &computed
				}
			}
			add(window, resetAt)
		}
	case account.IsAnthropicOAuthOrSetupToken():
		add("5h", account.SessionWindowEnd)
		add("7d", parseSchedulingResetAt(account.Extra["passive_usage_7d_reset"]))
	}
	return idle
}

// accountWindowPrimeTrigger is the earliest idle reset; one request starts all
// idle windows at once.
func accountWindowPrimeTrigger(idle []accountWindowPrimeIdleWindow) (time.Time, []string) {
	var trigger time.Time
	windows := make([]string, 0, len(idle))
	for _, item := range idle {
		if trigger.IsZero() || item.ResetAt.Before(trigger) {
			trigger = item.ResetAt
		}
		windows = append(windows, item.Window)
	}
	return trigger, windows
}

// accountWindowPrimeDueAt adds a delay derived from the account and reset, so
// every instance picks the same moment inside [min, max] minutes.
func accountWindowPrimeDueAt(accountID int64, trigger time.Time, settings AccountWindowPrimeSettings) time.Time {
	minSeconds := settings.DelayMinMinutes * 60
	maxSeconds := settings.DelayMaxMinutes * 60
	delay := minSeconds
	if maxSeconds > minSeconds {
		h := fnv.New64a()
		_, _ = fmt.Fprintf(h, "window-prime|%d|%d", accountID, trigger.Unix())
		delay += int(h.Sum64() % uint64(maxSeconds-minSeconds+1))
	}
	return trigger.Add(time.Duration(delay) * time.Second)
}

// parseAccountWindowPrimeClock parses HH:MM into minutes after midnight.
func parseAccountWindowPrimeClock(value string, allowEndOfDay bool) (int, bool) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, false
	}
	hour, errHour := strconv.Atoi(parts[0])
	minute, errMinute := strconv.Atoi(parts[1])
	if errHour != nil || errMinute != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	if hour == 24 && minute == 0 && allowEndOfDay {
		return 24 * 60, true
	}
	if hour < 0 || hour > 23 {
		return 0, false
	}
	return hour*60 + minute, true
}

// accountWindowPrimeWithinActiveHours checks the site-local clock; a start
// after the end wraps past midnight (22:00-06:00).
func accountWindowPrimeWithinActiveHours(settings AccountWindowPrimeSettings, now time.Time, loc *time.Location) bool {
	if !settings.ActiveHoursEnabled {
		return true
	}
	start, okStart := parseAccountWindowPrimeClock(settings.ActiveHoursStart, false)
	end, okEnd := parseAccountWindowPrimeClock(settings.ActiveHoursEnd, true)
	if !okStart || !okEnd || start == end {
		return true
	}
	local := now.In(loc)
	minute := local.Hour()*60 + local.Minute()
	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}

// accountWindowPrimeState is the last attempt for an account, kept in Extra.
type accountWindowPrimeState struct {
	TriggerResetUnix int64
	Attempts         int
	LastAttemptAt    time.Time
	Status           string
}

func readAccountWindowPrimeState(account *Account) accountWindowPrimeState {
	values, ok := account.Extra[AccountWindowPrimeStateExtraKey].(map[string]any)
	if !ok {
		return accountWindowPrimeState{}
	}
	state := accountWindowPrimeState{}
	if value, ok := accountWindowPrimeInt(values["trigger_reset_unix"]); ok {
		state.TriggerResetUnix = int64(value)
	}
	if value, ok := accountWindowPrimeInt(values["attempts"]); ok {
		state.Attempts = value
	}
	if at := parseSchedulingResetAt(values["last_attempt_at"]); at != nil {
		state.LastAttemptAt = *at
	}
	state.Status, _ = values["status"].(string)
	return state
}

// accountWindowPrimeShouldAttempt applies the per-reset retry budget.
func accountWindowPrimeShouldAttempt(state accountWindowPrimeState, trigger, now time.Time) bool {
	if state.TriggerResetUnix != trigger.Unix() {
		return true
	}
	if state.Status == accountWindowPrimeStatusSucceeded || state.Attempts >= accountWindowPrimeMaxAttempts {
		return false
	}
	return !now.Before(state.LastAttemptAt.Add(accountWindowPrimeRetryInterval))
}

func accountWindowPrimeInt(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, v >= 0
	case int64:
		return int(v), v >= 0
	case float64:
		return int(v), v >= 0
	case json.Number:
		parsed, err := v.Int64()
		return int(parsed), err == nil && parsed >= 0
	default:
		return 0, false
	}
}
