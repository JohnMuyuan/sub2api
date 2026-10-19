package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

type cachedAccountSchedulingThresholdRanges struct {
	ranges    map[string]AccountSchedulingThresholdRange
	expiresAt int64
}

// Same freshness as the fixed thresholds: other instances see a saved change
// within one TTL.
var accountSchedulingThresholdRangesCache atomic.Pointer[cachedAccountSchedulingThresholdRanges]
var accountSchedulingThresholdRangesSF singleflight.Group

// GetAccountSchedulingThresholdRanges returns the per-platform random ranges,
// including disabled ones. It never fails: errors fall back to no ranges.
func (s *SettingService) GetAccountSchedulingThresholdRanges(ctx context.Context) map[string]AccountSchedulingThresholdRange {
	if s == nil || s.settingRepo == nil {
		return map[string]AccountSchedulingThresholdRange{}
	}
	if cached := accountSchedulingThresholdRangesCache.Load(); cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cloneAccountSchedulingThresholdRanges(cached.ranges)
	}
	// Collapse the reload when the cache expires under load.
	result, _, _ := accountSchedulingThresholdRangesSF.Do(SettingKeyAccountSchedulingThresholdRanges, func() (any, error) {
		return s.loadAccountSchedulingThresholdRanges(ctx), nil
	})
	ranges, _ := result.(map[string]AccountSchedulingThresholdRange)
	return cloneAccountSchedulingThresholdRanges(ranges)
}

func (s *SettingService) loadAccountSchedulingThresholdRanges(ctx context.Context) map[string]AccountSchedulingThresholdRange {
	ttl := accountSchedulingThresholdsCacheTTL
	ranges := map[string]AccountSchedulingThresholdRange{}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accountSchedulingThresholdsDBTimeout)
	defer cancel()
	raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyAccountSchedulingThresholdRanges)
	switch {
	case err == nil:
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			ranges = parseAccountSchedulingThresholdRangesSetting(trimmed)
		}
	case !errors.Is(err, ErrSettingNotFound):
		slog.Warn("failed to get account scheduling threshold ranges, using none", "error", err)
		ttl = accountSchedulingThresholdsErrorTTL
	}
	accountSchedulingThresholdRangesCache.Store(&cachedAccountSchedulingThresholdRanges{
		ranges:    cloneAccountSchedulingThresholdRanges(ranges),
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
	return ranges
}

// SetAccountSchedulingThresholdRanges replaces all platform ranges.
func (s *SettingService) SetAccountSchedulingThresholdRanges(ctx context.Context, input map[string]AccountSchedulingThresholdRange) (map[string]AccountSchedulingThresholdRange, error) {
	normalized, err := validateAndNormalizeAccountSchedulingThresholdRanges(input)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_SCHEDULING_THRESHOLD_RANGES", err.Error())
	}
	blob, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAccountSchedulingThresholdRanges, string(blob)); err != nil {
		return nil, err
	}
	accountSchedulingThresholdRangesSF.Forget(SettingKeyAccountSchedulingThresholdRanges)
	accountSchedulingThresholdRangesCache.Store(nil)
	return normalized, nil
}

// GetAccountSchedulingThresholdPolicy bundles fixed thresholds and ranges for
// EvaluateAccountSchedulingThresholdWithPolicy.
func (s *SettingService) GetAccountSchedulingThresholdPolicy(ctx context.Context) AccountSchedulingThresholdPolicy {
	return AccountSchedulingThresholdPolicy{
		Thresholds: s.GetAccountSchedulingThresholds(ctx),
		Ranges:     s.GetAccountSchedulingThresholdRanges(ctx),
	}
}

func cloneAccountSchedulingThresholdRanges(input map[string]AccountSchedulingThresholdRange) map[string]AccountSchedulingThresholdRange {
	cloned := make(map[string]AccountSchedulingThresholdRange, len(input))
	for platform, r := range input {
		cloned[platform] = r
	}
	return cloned
}
