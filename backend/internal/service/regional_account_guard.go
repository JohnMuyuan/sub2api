package service

import (
	"context"
	"net/http"
	"time"
)

type RegionalAccountGuard interface {
	RegionalAccountAllowed(context.Context, int64) error
	RegionalAccountReport(context.Context, int64, int) error
}

// Only the region owns a local report outbox; primary retains its existing
// provider-specific error policy and publishes the resulting canonical state.
type RegionalAccountReporter interface {
	RegionalAccountReportingLocal() bool
	RegionalAccountRestriction(context.Context, int64, time.Time, bool) error
}

func reportRegionalAccountError(ctx context.Context, guard RegionalAccountReporter, account *Account, status int, headers http.Header, body []byte) (bool, error) {
	if account == nil {
		return false, nil
	}
	if (account.Platform == PlatformGrok && isGrokContentPolicyRejection(status, body)) || isAnthropicSafeguardPolicy403(account, status, body) || (account.Platform == PlatformOpenAI && isOpenAIRequestScopedCapacityShed("", body)) {
		return false, nil
	}
	now := time.Now().UTC()
	until := now.Add(time.Minute)
	invalid := status == http.StatusUnauthorized
	if status == http.StatusForbidden {
		invalid = account.Platform == PlatformOpenAI && isOpenAIHTTPUpstreamAccessStateError(status, "", body)
		if !invalid {
			return false, nil
		}
	}
	if status != http.StatusTooManyRequests && !invalid {
		return false, nil
	}
	if status == http.StatusTooManyRequests {
		if reset := parseRetryAfterResetTime(headers, now); reset != nil && reset.After(until) {
			until = *reset
		}
		if account.Platform == PlatformOpenAI {
			if reset := parseOpenAIRateLimitResetTime(body); reset != nil && time.Unix(*reset, 0).After(until) {
				until = time.Unix(*reset, 0)
			}
		}
		if account.Platform == PlatformAnthropic {
			if limit := selectAnthropicExhaustedWindow(headers, now); limit != nil && limit.resetAt.After(until) {
				until = limit.resetAt
			}
		}
	}
	return true, guard.RegionalAccountRestriction(ctx, account.ID, until, invalid)
}
func (s *GatewayService) regionalHandleUpstreamError(ctx context.Context, account *Account, status int, headers http.Header, body []byte, model ...string) bool {
	if guard, ok := s.rpmCache.(RegionalAccountReporter); ok && guard.RegionalAccountReportingLocal() {
		reportCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		restricted, err := reportRegionalAccountError(reportCtx, guard, account, status, headers, body)
		return restricted || err != nil
	}
	return s.rateLimitService.HandleUpstreamError(ctx, account, status, headers, body, model...)
}

// accountForRegionalRequest preserves the primary path and makes an installed
// regional snapshot the sole authority for all final account/parent reads.
func (s *OpenAIGatewayService) accountForRegionalRequest(ctx context.Context, id int64) (*Account, error) {
	if s.schedulerSnapshot != nil && s.schedulerSnapshot.localSnapshotReader != nil {
		return s.schedulerSnapshot.GetAccount(ctx, id)
	}
	if s.accountRepo == nil {
		return nil, ErrLocalSchedulerSnapshotUnavailable
	}
	return s.accountRepo.GetByID(ctx, id)
}
func (s *OpenAIGatewayService) regionalCredentialAccount(ctx context.Context, account *Account) (*Account, error) {
	if s.schedulerSnapshot == nil || s.schedulerSnapshot.localSnapshotReader == nil {
		return resolveCredentialAccount(ctx, s.accountRepo, account)
	}
	if account == nil {
		return nil, ErrLocalSchedulerSnapshotUnavailable
	}
	fresh, err := s.schedulerSnapshot.GetAccount(ctx, account.ID)
	if err != nil || fresh == nil {
		return nil, ErrLocalSchedulerSnapshotUnavailable
	}
	if !fresh.IsShadow() {
		return fresh, nil
	}
	parent, err := s.schedulerSnapshot.GetAccount(ctx, *fresh.ParentAccountID)
	if err != nil || parent == nil || parent.IsShadow() || !parent.IsOpenAIOAuth() {
		return nil, ErrLocalSchedulerSnapshotUnavailable
	}
	return parent, nil
}
