package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"
)

const (
	codexImageUsedPercentKey       = "codex_image_used_percent"
	codexImageResetAfterSecondsKey = "codex_image_reset_after_seconds"
	codexImageWindowMinutesKey     = "codex_image_window_minutes"
	codexImageResetAtKey           = "codex_image_reset_at"
	codexImageUsageUpdatedAtKey    = "codex_image_usage_updated_at"
)

type codexImageRateLimitWindow struct {
	UsedPercent       *float64
	ResetAfterSeconds *int
	WindowMinutes     *int
}

// parseCodexImageRateLimitWindow extracts the representative rate-limit window
// returned by ChatGPT's dedicated /backend-api/codex/images/* endpoints.
// Unlike /codex/responses (5h + 7d chat quota), the direct image endpoints
// report an independent rolling window (commonly 24h = 1440m) via x-codex-*.
func parseCodexImageRateLimitWindow(headers http.Header) *codexImageRateLimitWindow {
	snapshot := ParseCodexRateLimitHeaders(headers)
	if snapshot == nil {
		return nil
	}

	primary := &codexImageRateLimitWindow{
		UsedPercent:       snapshot.PrimaryUsedPercent,
		ResetAfterSeconds: snapshot.PrimaryResetAfterSeconds,
		WindowMinutes:     snapshot.PrimaryWindowMinutes,
	}
	secondary := &codexImageRateLimitWindow{
		UsedPercent:       snapshot.SecondaryUsedPercent,
		ResetAfterSeconds: snapshot.SecondaryResetAfterSeconds,
		WindowMinutes:     snapshot.SecondaryWindowMinutes,
	}

	hasPrimary := primary.UsedPercent != nil || primary.ResetAfterSeconds != nil || primary.WindowMinutes != nil
	hasSecondary := secondary.UsedPercent != nil || secondary.ResetAfterSeconds != nil || secondary.WindowMinutes != nil

	switch {
	case hasPrimary && !hasSecondary:
		return primary
	case !hasPrimary && hasSecondary:
		return secondary
	case !hasPrimary && !hasSecondary:
		return nil
	}

	// Both primary and secondary present: prefer an exhausted window first,
	// otherwise prefer the larger window (or primary when tied/unspecified).
	primaryExhausted := primary.UsedPercent != nil && *primary.UsedPercent >= 100
	secondaryExhausted := secondary.UsedPercent != nil && *secondary.UsedPercent >= 100
	if primaryExhausted != secondaryExhausted {
		if primaryExhausted {
			return primary
		}
		return secondary
	}
	if primary.WindowMinutes != nil && secondary.WindowMinutes != nil && *secondary.WindowMinutes > *primary.WindowMinutes {
		return secondary
	}
	if primary.UsedPercent == nil && secondary.UsedPercent != nil {
		return secondary
	}
	return primary
}

// calculateOpenAIImage429ResetTime parses an exhausted reset time from the
// x-codex-* headers of a Codex Images 429 response without misclassifying it
// as a global 5h/7d Codex text quota exhaustion.
func calculateOpenAIImage429ResetTime(headers http.Header) *time.Time {
	w := parseCodexImageRateLimitWindow(headers)
	if w == nil || w.UsedPercent == nil || *w.UsedPercent < 100 || w.ResetAfterSeconds == nil || *w.ResetAfterSeconds <= 0 {
		return nil
	}
	resetAt := time.Now().Add(time.Duration(*w.ResetAfterSeconds) * time.Second)
	return &resetAt
}

// buildCodexImageUsageExtraUpdates converts direct Codex Images rate-limit
// headers (or a 429 reset body) into dedicated codex_image_* account Extra keys
// so image quota observations never overwrite global codex_5h_* / codex_7d_* fields.
func buildCodexImageUsageExtraUpdates(headers http.Header, fallbackBody []byte, is429 bool, now time.Time) map[string]any {
	updates := make(map[string]any)
	w := parseCodexImageRateLimitWindow(headers)

	if w != nil && w.UsedPercent != nil {
		updates[codexImageUsedPercentKey] = *w.UsedPercent
		if w.WindowMinutes != nil && *w.WindowMinutes > 0 {
			updates[codexImageWindowMinutesKey] = *w.WindowMinutes
		}
		if w.ResetAfterSeconds != nil && *w.ResetAfterSeconds >= 0 {
			updates[codexImageResetAfterSecondsKey] = *w.ResetAfterSeconds
			updates[codexImageResetAtKey] = now.Add(time.Duration(*w.ResetAfterSeconds) * time.Second).UTC().Format(time.RFC3339)
		}
	} else if is429 {
		// Fall back to explicit reset timestamps from Retry-After / 429 JSON body
		// when x-codex-* headers are absent.
		var resetAt *time.Time
		if parsed := parseRetryAfterResetTime(headers, now); parsed != nil && parsed.After(now) {
			resetAt = parsed
		} else if resetUnix := parseOpenAIRateLimitResetTime(fallbackBody); resetUnix != nil {
			if parsed := time.Unix(*resetUnix, 0); parsed.After(now) {
				resetAt = &parsed
			}
		}
		if resetAt != nil {
			updates[codexImageUsedPercentKey] = 100.0
			updates[codexImageResetAtKey] = resetAt.UTC().Format(time.RFC3339)
			updates[codexImageResetAfterSecondsKey] = int(math.Ceil(resetAt.Sub(now).Seconds()))
		}
	}

	if len(updates) == 0 {
		return nil
	}
	updates[codexImageUsageUpdatedAtKey] = now.UTC().Format(time.RFC3339Nano)
	return updates
}

// UpdateCodexImageUsageSnapshotFromHeaders persists passive image quota headers
// returned by /backend-api/codex/images/* into codex_image_* Extra keys without
// touching the account's global 5h/7d Codex snapshot.
func (s *OpenAIGatewayService) UpdateCodexImageUsageSnapshotFromHeaders(ctx context.Context, accountID int64, headers http.Header) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || headers == nil {
		return
	}
	updates := buildCodexImageUsageExtraUpdates(headers, nil, false, time.Now())
	if len(updates) == 0 {
		return
	}
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	go func() {
		updateCtx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
		defer cancel()
		if err := s.accountRepo.UpdateExtra(updateCtx, accountID, updates); err != nil {
			slog.Warn("openai_codex_image_snapshot_persist_failed", "account_id", accountID, "error", err)
		}
	}()
}

func (s *RateLimitService) persistOpenAICodexImageSnapshot(ctx context.Context, account *Account, headers http.Header, responseBody []byte) {
	if s == nil || s.accountRepo == nil || account == nil || account.ID <= 0 {
		return
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.IsShadow() {
		return
	}
	updates := buildCodexImageUsageExtraUpdates(headers, responseBody, true, time.Now())
	if len(updates) == 0 {
		return
	}
	mergeAccountExtra(account, updates)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("openai_codex_image_snapshot_persist_failed", "account_id", account.ID, "error", err)
	}
}

func buildCodexImageUsageProgressFromExtra(extra map[string]any, now time.Time) *UsageProgress {
	if len(extra) == 0 {
		return nil
	}

	if usedRaw, ok := extra[codexImageUsedPercentKey]; ok {
		progress := &UsageProgress{
			Utilization:   parseExtraFloat64(usedRaw),
			WindowMinutes: parseExtraInt(extra[codexImageWindowMinutesKey]),
		}
		if resetAtRaw, ok := extra[codexImageResetAtKey]; ok {
			if resetAt, err := parseTime(fmt.Sprint(resetAtRaw)); err == nil {
				progress.ResetsAt = &resetAt
				progress.RemainingSeconds = int(resetAt.Sub(now).Seconds())
				if progress.RemainingSeconds < 0 {
					progress.RemainingSeconds = 0
				}
			}
		}
		if progress.ResetsAt == nil {
			if resetAfterSeconds := parseExtraInt(extra[codexImageResetAfterSecondsKey]); resetAfterSeconds > 0 {
				base := now
				if updatedAtRaw, ok := extra[codexImageUsageUpdatedAtKey]; ok {
					if updatedAt, err := parseTime(fmt.Sprint(updatedAtRaw)); err == nil {
						base = updatedAt
					}
				}
				resetAt := base.Add(time.Duration(resetAfterSeconds) * time.Second)
				progress.ResetsAt = &resetAt
				progress.RemainingSeconds = int(resetAt.Sub(now).Seconds())
				if progress.RemainingSeconds < 0 {
					progress.RemainingSeconds = 0
				}
			}
		}

		// 窗口已过期（resetAt 在 now 之前）→ 额度已重置，归零
		if progress.ResetsAt != nil && !now.Before(*progress.ResetsAt) {
			progress.Utilization = 0
			progress.RemainingSeconds = 0
		}
		return progress
	}

	// 兜底：若尚无被动采样头但存在生效中的生图模型级限流，展示 100% 与其重置倒计时。
	modelLimits, _ := extra[modelRateLimitsKey].(map[string]any)
	entry, _ := modelLimits[openAIImageGenerationRateLimitKey].(map[string]any)
	if len(entry) == 0 {
		return nil
	}
	reason, _ := entry["reason"].(string)
	if reason != openAIImageRateLimitReason {
		return nil
	}
	resetAtRaw, ok := entry["rate_limit_reset_at"]
	if !ok {
		return nil
	}
	resetAt, err := parseTime(fmt.Sprint(resetAtRaw))
	if err != nil || !now.Before(resetAt) {
		return nil
	}
	remaining := int(resetAt.Sub(now).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	return &UsageProgress{
		Utilization:      100,
		ResetsAt:         &resetAt,
		RemainingSeconds: remaining,
	}
}
