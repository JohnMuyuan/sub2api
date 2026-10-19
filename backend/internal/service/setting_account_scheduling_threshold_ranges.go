package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// cachedAccountSchedulingThresholdCustom holds the platform random ranges and
// per-window rules, which are read together on the scheduling hot path.
type cachedAccountSchedulingThresholdCustom struct {
	ranges      map[string]AccountSchedulingThresholdRange
	windowRules map[string]map[string]AccountSchedulingThresholdWindowRule
	expiresAt   int64
}

// Same freshness as the fixed thresholds: other instances see a saved change
// within one TTL.
var accountSchedulingThresholdRangesCache atomic.Pointer[cachedAccountSchedulingThresholdCustom]
var accountSchedulingThresholdRangesSF singleflight.Group

// GetAccountSchedulingThresholdRanges returns the per-platform random ranges,
// including disabled ones. It never fails: errors fall back to no ranges.
func (s *SettingService) GetAccountSchedulingThresholdRanges(ctx context.Context) map[string]AccountSchedulingThresholdRange {
	return cloneAccountSchedulingThresholdRanges(s.accountSchedulingThresholdCustom(ctx).ranges)
}

// GetAccountSchedulingThresholdWindowRules returns the per-platform, per-window
// rules. It never fails: errors fall back to no rules.
func (s *SettingService) GetAccountSchedulingThresholdWindowRules(ctx context.Context) map[string]map[string]AccountSchedulingThresholdWindowRule {
	return cloneAccountSchedulingThresholdWindowRules(s.accountSchedulingThresholdCustom(ctx).windowRules)
}

func (s *SettingService) accountSchedulingThresholdCustom(ctx context.Context) *cachedAccountSchedulingThresholdCustom {
	if s == nil || s.settingRepo == nil {
		return &cachedAccountSchedulingThresholdCustom{}
	}
	if cached := accountSchedulingThresholdRangesCache.Load(); cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached
	}
	// Collapse the reload when the cache expires under load.
	result, _, _ := accountSchedulingThresholdRangesSF.Do(SettingKeyAccountSchedulingThresholdRanges, func() (any, error) {
		return s.loadAccountSchedulingThresholdCustom(ctx), nil
	})
	if loaded, ok := result.(*cachedAccountSchedulingThresholdCustom); ok {
		return loaded
	}
	return &cachedAccountSchedulingThresholdCustom{}
}

func (s *SettingService) loadAccountSchedulingThresholdCustom(ctx context.Context) *cachedAccountSchedulingThresholdCustom {
	ttl := accountSchedulingThresholdsCacheTTL
	loaded := &cachedAccountSchedulingThresholdCustom{
		ranges:      map[string]AccountSchedulingThresholdRange{},
		windowRules: map[string]map[string]AccountSchedulingThresholdWindowRule{},
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accountSchedulingThresholdsDBTimeout)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(dbCtx, []string{SettingKeyAccountSchedulingThresholdRanges, SettingKeyAccountSchedulingThresholdWindowRules})
	if err != nil {
		slog.Warn("failed to get account scheduling threshold ranges, using none", "error", err)
		ttl = accountSchedulingThresholdsErrorTTL
	} else {
		if raw := strings.TrimSpace(values[SettingKeyAccountSchedulingThresholdRanges]); raw != "" {
			loaded.ranges = parseAccountSchedulingThresholdRangesSetting(raw)
		}
		if raw := strings.TrimSpace(values[SettingKeyAccountSchedulingThresholdWindowRules]); raw != "" {
			loaded.windowRules = parseAccountSchedulingThresholdWindowRulesSetting(raw)
		}
	}
	loaded.expiresAt = time.Now().Add(ttl).UnixNano()
	accountSchedulingThresholdRangesCache.Store(loaded)
	return loaded
}

// SetAccountSchedulingThresholdRanges replaces all platform ranges and, when
// windowRules is non-nil, all platform per-window rules.
func (s *SettingService) SetAccountSchedulingThresholdRanges(ctx context.Context, input map[string]AccountSchedulingThresholdRange, windowRules map[string]map[string]AccountSchedulingThresholdWindowRule) (map[string]AccountSchedulingThresholdRange, map[string]map[string]AccountSchedulingThresholdWindowRule, error) {
	normalized, err := validateAndNormalizeAccountSchedulingThresholdRanges(input)
	if err != nil {
		return nil, nil, infraerrors.BadRequest("INVALID_ACCOUNT_SCHEDULING_THRESHOLD_RANGES", err.Error())
	}
	updates := map[string]string{}
	blob, err := json.Marshal(normalized)
	if err != nil {
		return nil, nil, err
	}
	updates[SettingKeyAccountSchedulingThresholdRanges] = string(blob)
	if windowRules != nil {
		normalizedRules, err := validateAndNormalizeAccountSchedulingThresholdWindowRules(windowRules)
		if err != nil {
			return nil, nil, infraerrors.BadRequest("INVALID_ACCOUNT_SCHEDULING_THRESHOLD_WINDOW_RULES", err.Error())
		}
		rulesBlob, err := json.Marshal(normalizedRules)
		if err != nil {
			return nil, nil, err
		}
		updates[SettingKeyAccountSchedulingThresholdWindowRules] = string(rulesBlob)
	}
	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return nil, nil, err
	}
	accountSchedulingThresholdRangesSF.Forget(SettingKeyAccountSchedulingThresholdRanges)
	accountSchedulingThresholdRangesCache.Store(nil)
	return normalized, s.GetAccountSchedulingThresholdWindowRules(ctx), nil
}

// GetAccountSchedulingThresholdPolicy bundles fixed thresholds, ranges and
// per-window rules for EvaluateAccountSchedulingThresholdWithPolicy.
func (s *SettingService) GetAccountSchedulingThresholdPolicy(ctx context.Context) AccountSchedulingThresholdPolicy {
	custom := s.accountSchedulingThresholdCustom(ctx)
	return AccountSchedulingThresholdPolicy{
		Thresholds:  s.GetAccountSchedulingThresholds(ctx),
		Ranges:      custom.ranges,
		WindowRules: custom.windowRules,
	}
}

func cloneAccountSchedulingThresholdRanges(input map[string]AccountSchedulingThresholdRange) map[string]AccountSchedulingThresholdRange {
	cloned := make(map[string]AccountSchedulingThresholdRange, len(input))
	for platform, r := range input {
		cloned[platform] = r
	}
	return cloned
}
