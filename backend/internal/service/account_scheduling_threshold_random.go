package service

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// Randomized scheduling thresholds: instead of one fixed pause percentage an
// account (or a platform default) may configure a range such as 90-97. Every
// usage window draws its own percentage from that range. The draw is derived
// from the account ID, the window and the window's reset time, so it stays the
// same for the whole window, matches across instances and restarts, and is
// re-drawn automatically once the window resets.
//
// Drawing 100 means "do not pause in this window". An optional monthly limit
// caps how many windows of each kind may draw 100 per calendar month (site
// timezone, month of the window's reset time); once used up, such draws are
// re-drawn between min and 99. Only full draws are persisted, in account.Extra.

const (
	// AccountSchedulingThresholdRangeCredentialKey stores an account's own range.
	// account_scheduling_threshold is kept at the range minimum alongside it so
	// builds without range support still pause at the conservative end.
	AccountSchedulingThresholdRangeCredentialKey = "account_scheduling_threshold_range"

	// SettingKeyAccountSchedulingThresholdRanges stores the per-platform default
	// ranges (JSON map) next to the fixed account_scheduling_thresholds setting.
	SettingKeyAccountSchedulingThresholdRanges = "account_scheduling_threshold_ranges"

	accountSchedulingThresholdFullDrawExtraKeyPrefix = "scheduling_threshold_full_draws_"

	// MaxAccountSchedulingThresholdFullMonthlyLimit bounds the stored window list.
	MaxAccountSchedulingThresholdFullMonthlyLimit = 1000

	// Reset times derived from "seconds until reset" drift by a few seconds
	// between snapshots; rounding to the hour keeps one window on one draw.
	accountSchedulingThresholdWindowBucket = time.Hour
)

// accountSchedulingThresholdWindows lists every window name the evaluator can
// produce; each gets its own full-draw counter key.
var accountSchedulingThresholdWindows = []string{"5h", "7d", "7d_oi", "quota", "weekly", "monthly"}

// AccountSchedulingThresholdFullDrawExtraKeys returns the account.Extra keys
// that hold monthly full-draw records. The scheduler snapshot must keep them,
// otherwise candidate filtering would not see the month's usage.
func AccountSchedulingThresholdFullDrawExtraKeys() []string {
	keys := make([]string, 0, len(accountSchedulingThresholdWindows))
	for _, window := range accountSchedulingThresholdWindows {
		keys = append(keys, accountSchedulingThresholdFullDrawExtraKeyPrefix+window)
	}
	return keys
}

// AccountSchedulingThresholdRange is a random pause range. Enabled is only
// meaningful for platform defaults; an account range is active while stored.
type AccountSchedulingThresholdRange struct {
	Enabled          bool `json:"enabled"`
	Min              int  `json:"min"`
	Max              int  `json:"max"`
	FullMonthlyLimit int  `json:"full_monthly_limit"`
}

func (r AccountSchedulingThresholdRange) valid() bool {
	return r.Min >= 1 && r.Min <= r.Max && r.Max <= 100 &&
		r.FullMonthlyLimit >= 0 && r.FullMonthlyLimit <= MaxAccountSchedulingThresholdFullMonthlyLimit
}

// AccountSchedulingThresholdPolicy is everything the evaluator needs besides
// the account: fixed platform thresholds, platform ranges and the timezone that
// defines calendar months for the full-draw limit.
type AccountSchedulingThresholdPolicy struct {
	Thresholds map[string]int
	Ranges     map[string]AccountSchedulingThresholdRange
	Location   *time.Location
}

func (p AccountSchedulingThresholdPolicy) location() *time.Location {
	if p.Location != nil {
		return p.Location
	}
	if loc := timezone.Location(); loc != nil {
		return loc
	}
	return time.UTC
}

type accountSchedulingThresholdRule struct {
	fixed  int
	random *AccountSchedulingThresholdRange
}

func (r accountSchedulingThresholdRule) basePercent() int {
	if r.random != nil {
		return r.random.Min
	}
	return r.fixed
}

// disabled reports a rule that can never pause: fixed 100, or a range whose
// minimum is already 100.
func (r accountSchedulingThresholdRule) disabled() bool {
	return r.basePercent() >= 100
}

// resolveAccountSchedulingThresholdRule picks, in order: the account range, the
// account fixed override, the platform range, the platform fixed threshold.
func resolveAccountSchedulingThresholdRule(account *Account, policy AccountSchedulingThresholdPolicy, platform string) (accountSchedulingThresholdRule, bool) {
	if r, ok := accountSchedulingThresholdRangeOverride(account); ok {
		return accountSchedulingThresholdRule{random: &r}, true
	}
	if threshold, ok := accountSchedulingThresholdOverride(account); ok {
		return accountSchedulingThresholdRule{fixed: threshold}, true
	}
	if r, ok := policy.Ranges[platform]; ok && r.Enabled && r.valid() {
		return accountSchedulingThresholdRule{random: &r}, true
	}
	threshold, ok := lookupAccountSchedulingThreshold(policy.Thresholds, platform)
	return accountSchedulingThresholdRule{fixed: threshold}, ok
}

func accountSchedulingThresholdRangeOverride(account *Account) (AccountSchedulingThresholdRange, bool) {
	if account == nil || len(account.Credentials) == 0 {
		return AccountSchedulingThresholdRange{}, false
	}
	r, ok := parseAccountSchedulingThresholdRange(account.Credentials[AccountSchedulingThresholdRangeCredentialKey])
	if !ok {
		return AccountSchedulingThresholdRange{}, false
	}
	r.Enabled = true
	return r, true
}

func parseAccountSchedulingThresholdRange(raw any) (AccountSchedulingThresholdRange, bool) {
	values, ok := raw.(map[string]any)
	if !ok {
		return AccountSchedulingThresholdRange{}, false
	}
	minValue, okMin := parseAccountSchedulingThresholdValue(values["min"])
	maxValue, okMax := parseAccountSchedulingThresholdValue(values["max"])
	if !okMin || !okMax {
		return AccountSchedulingThresholdRange{}, false
	}
	r := AccountSchedulingThresholdRange{Min: minValue, Max: maxValue}
	if limit, ok := parseNonNegativeInt(values["full_monthly_limit"]); ok {
		r.FullMonthlyLimit = limit
	}
	return r, r.valid()
}

func parseNonNegativeInt(raw any) (int, bool) {
	var value float64
	switch v := raw.(type) {
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case float64:
		value = v
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		value = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		value = parsed
	default:
		return 0, false
	}
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return int(math.Round(value)), true
}

// accountSchedulingThresholdDraw is the threshold one window uses this round.
type accountSchedulingThresholdDraw struct {
	percent       int
	fullDrawsUsed int
	// recordKey/recordValue persist a newly granted full draw; empty otherwise.
	recordKey   string
	recordValue map[string]any
}

type accountSchedulingThresholdFullDrawRecord struct {
	month   string
	windows []int64
}

func (r accountSchedulingThresholdFullDrawRecord) contains(bucket int64) bool {
	for _, window := range r.windows {
		if window == bucket {
			return true
		}
	}
	return false
}

func drawAccountSchedulingThreshold(account *Account, r AccountSchedulingThresholdRange, window, scope string, until time.Time, loc *time.Location) accountSchedulingThresholdDraw {
	bucket := accountSchedulingThresholdWindowBucketOf(until)
	seed := fmt.Sprintf("%d|%s|%s|%d", account.ID, window, scope, bucket)
	draw := accountSchedulingThresholdDraw{percent: drawInRange(seed, r.Min, r.Max)}
	if r.FullMonthlyLimit <= 0 {
		return draw
	}

	month := until.In(loc).Format("2006-01")
	key := accountSchedulingThresholdFullDrawExtraKeyPrefix + window
	record := readAccountSchedulingThresholdFullDrawRecord(account.Extra[key])
	if record.month != month {
		record = accountSchedulingThresholdFullDrawRecord{month: month}
	}
	draw.fullDrawsUsed = len(record.windows)
	if draw.percent < 100 || record.contains(bucket) {
		return draw
	}
	if len(record.windows) < r.FullMonthlyLimit {
		windows := append(append([]int64(nil), record.windows...), bucket)
		draw.fullDrawsUsed = len(windows)
		draw.recordKey = key
		draw.recordValue = map[string]any{"month": month, "windows": windows}
		return draw
	}
	if r.Min <= 99 {
		draw.percent = drawInRange(seed+"|redraw", r.Min, 99)
	}
	return draw
}

func accountSchedulingThresholdWindowBucketOf(until time.Time) int64 {
	step := int64(accountSchedulingThresholdWindowBucket / time.Second)
	return (until.Unix() + step/2) / step * step
}

func drawInRange(seed string, minValue, maxValue int) int {
	if maxValue <= minValue {
		return minValue
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	return minValue + int(h.Sum64()%uint64(maxValue-minValue+1))
}

func readAccountSchedulingThresholdFullDrawRecord(raw any) accountSchedulingThresholdFullDrawRecord {
	values, ok := raw.(map[string]any)
	if !ok {
		return accountSchedulingThresholdFullDrawRecord{}
	}
	record := accountSchedulingThresholdFullDrawRecord{}
	record.month, _ = values["month"].(string)
	switch windows := values["windows"].(type) {
	case []any:
		for _, item := range windows {
			if bucket, ok := parseNonNegativeInt(item); ok {
				record.windows = append(record.windows, int64(bucket))
			}
		}
	case []int64:
		record.windows = append(record.windows, windows...)
	}
	return record
}

// applyAccountSchedulingThresholdRule assigns every candidate the threshold it
// uses this round and returns the full-draw records that still need saving.
// Windows that already reset are skipped so they never use up the monthly limit.
func applyAccountSchedulingThresholdRule(account *Account, rule accountSchedulingThresholdRule, candidates []*accountSchedulingThresholdCandidate, loc *time.Location, now time.Time) map[string]any {
	var updates map[string]any
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if rule.random == nil || candidate.until == nil || !candidate.until.After(now) {
			candidate.threshold = rule.basePercent()
			continue
		}
		draw := drawAccountSchedulingThreshold(account, *rule.random, candidate.window, candidate.scope, *candidate.until, loc)
		candidate.threshold = draw.percent
		if draw.recordKey != "" {
			if updates == nil {
				updates = map[string]any{}
			}
			updates[draw.recordKey] = draw.recordValue
		}
	}
	return updates
}

// AccountSchedulingThresholdRound is the threshold an account uses in one of its
// current windows, shown in the account list when a random range applies.
type AccountSchedulingThresholdRound struct {
	Window           string    `json:"window"`
	Scope            string    `json:"scope,omitempty"`
	ThresholdPercent int       `json:"threshold_percent"`
	UsedPercent      float64   `json:"used_percent"`
	ResetsAt         time.Time `json:"resets_at"`
	RangeMin         int       `json:"range_min"`
	RangeMax         int       `json:"range_max"`
	FullMonthlyLimit int       `json:"full_monthly_limit"`
	FullDrawsUsed    int       `json:"full_draws_used"`
}

// AccountSchedulingThresholdRounds lists this round's drawn threshold for each
// window that still has a future reset. It returns nil unless a random range
// applies, and never persists anything.
func AccountSchedulingThresholdRounds(account *Account, policy AccountSchedulingThresholdPolicy, now time.Time) []AccountSchedulingThresholdRound {
	if account == nil {
		return nil
	}
	platform := strings.ToLower(strings.TrimSpace(account.Platform))
	if !isAllowedSchedulingThresholdPlatform(platform) {
		return nil
	}
	rule, ok := resolveAccountSchedulingThresholdRule(account, policy, platform)
	if !ok || rule.random == nil {
		return nil
	}

	candidates := accountSchedulingThresholdPlatformCandidates(account, platform, now)
	if platform == PlatformAnthropic {
		candidates = append(candidates, anthropicFableThresholdCandidate(account))
	}
	loc := policy.location()
	var rounds []AccountSchedulingThresholdRound
	for _, candidate := range candidates {
		if candidate == nil || candidate.until == nil || !candidate.until.After(now) {
			continue
		}
		draw := drawAccountSchedulingThreshold(account, *rule.random, candidate.window, candidate.scope, *candidate.until, loc)
		rounds = append(rounds, AccountSchedulingThresholdRound{
			Window:           candidate.window,
			Scope:            candidate.scope,
			ThresholdPercent: draw.percent,
			UsedPercent:      candidate.usedPercent,
			ResetsAt:         candidate.until.UTC(),
			RangeMin:         rule.random.Min,
			RangeMax:         rule.random.Max,
			FullMonthlyLimit: rule.random.FullMonthlyLimit,
			FullDrawsUsed:    draw.fullDrawsUsed,
		})
	}
	sort.SliceStable(rounds, func(i, j int) bool { return rounds[i].ResetsAt.Before(rounds[j].ResetsAt) })
	return rounds
}

func validateAndNormalizeAccountSchedulingThresholdRanges(input map[string]AccountSchedulingThresholdRange) (map[string]AccountSchedulingThresholdRange, error) {
	normalized := make(map[string]AccountSchedulingThresholdRange, len(input))
	for platform, r := range input {
		if !isAllowedSchedulingThresholdPlatform(platform) {
			return nil, fmt.Errorf("unknown platform %q", platform)
		}
		// A disabled range keeps its numbers so the form can restore them.
		if r.Enabled && !r.valid() {
			return nil, fmt.Errorf("%s: need 1 <= min <= max <= 100 and 0 <= full_monthly_limit <= %d", platform, MaxAccountSchedulingThresholdFullMonthlyLimit)
		}
		normalized[platform] = r
	}
	return normalized, nil
}

func parseAccountSchedulingThresholdRangesSetting(raw string) map[string]AccountSchedulingThresholdRange {
	parsed := map[string]AccountSchedulingThresholdRange{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]AccountSchedulingThresholdRange{}
	}
	out := make(map[string]AccountSchedulingThresholdRange, len(parsed))
	for platform, r := range parsed {
		if isAllowedSchedulingThresholdPlatform(platform) && (r.valid() || !r.Enabled) {
			out[platform] = r
		}
	}
	return out
}

// accountSchedulingThresholdFullDrawWrites remembers the last saved full-draw
// record per account and window so a stale scheduler snapshot does not turn
// every request into a write.
var accountSchedulingThresholdFullDrawWrites sync.Map // "<accountID>|<extra key>" -> JSON string

func (s *RateLimitService) recordAccountSchedulingThresholdFullDraws(ctx context.Context, account *Account, updates map[string]any) {
	if s == nil || s.accountRepo == nil || account == nil || len(updates) == 0 {
		return
	}
	pending := make(map[string]any, len(updates))
	for key, value := range updates {
		blob, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if saved, ok := accountSchedulingThresholdFullDrawWrites.Load(fmt.Sprintf("%d|%s", account.ID, key)); ok && saved == string(blob) {
			continue
		}
		pending[key] = value
	}
	if len(pending) == 0 {
		return
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, pending); err != nil {
		slog.Warn("account_scheduling_threshold_full_draw_record_failed", "account_id", account.ID, "error", err)
		return
	}
	for key, value := range pending {
		blob, _ := json.Marshal(value)
		accountSchedulingThresholdFullDrawWrites.Store(fmt.Sprintf("%d|%s", account.ID, key), string(blob))
	}
	slog.Info("account_scheduling_threshold_full_draw_recorded", "account_id", account.ID, "updates", pending)
}

// AccountSchedulingThresholdRounds returns the account's drawn thresholds for
// its current windows, or nil when no random range applies.
func (s *RateLimitService) AccountSchedulingThresholdRounds(ctx context.Context, account *Account) []AccountSchedulingThresholdRound {
	if s == nil || s.settingService == nil || account == nil {
		return nil
	}
	return AccountSchedulingThresholdRounds(account, s.settingService.GetAccountSchedulingThresholdPolicy(ctx), time.Now().UTC())
}
