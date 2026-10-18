package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSystemTimezoneAccountQuotaMappingDoesNotLeakStaleResetOrMutateSource(t *testing.T) {
	stale := "2099-01-01T00:00:00Z"
	account := &service.Account{Type: service.AccountTypeAPIKey, Extra: map[string]any{
		"quota_daily_reset_mode": "fixed", "quota_daily_reset_hour": float64(9), "quota_daily_reset_at": stale,
	}}
	mapped := AccountFromServiceShallow(account)
	require.NotNil(t, mapped.QuotaDailyResetAt)
	require.NotEqual(t, stale, *mapped.QuotaDailyResetAt)
	reset, err := time.Parse(time.RFC3339, *mapped.QuotaDailyResetAt)
	require.NoError(t, err)
	require.Equal(t, 9, reset.In(timezone.Location()).Hour())
	require.Equal(t, stale, account.Extra["quota_daily_reset_at"])
	account.Extra["quota_reset_timezone"] = "UTC"
	mapped = AccountFromServiceShallow(account)
	require.Equal(t, stale, *mapped.QuotaDailyResetAt)
	require.Equal(t, "UTC", *mapped.QuotaResetTimezone)
}
