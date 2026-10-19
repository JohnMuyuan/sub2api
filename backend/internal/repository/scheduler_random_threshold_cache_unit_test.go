//go:build unit

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRandomSchedulingThreshold_SchedulerSnapshotKeepsRangeAndFullDraws(t *testing.T) {
	r := map[string]any{"min": 90, "max": 100, "full_monthly_limit": 2}
	credentials := filterSchedulerCredentials(map[string]any{
		"account_scheduling_threshold":                         90,
		service.AccountSchedulingThresholdRangeCredentialKey:   r,
		service.AccountSchedulingThresholdWindowsCredentialKey: map[string]any{"5h": map[string]any{"mode": "fixed", "threshold": 80}},
		"access_token": "secret",
	})
	require.Equal(t, r, credentials[service.AccountSchedulingThresholdRangeCredentialKey])
	require.Contains(t, credentials, service.AccountSchedulingThresholdWindowsCredentialKey)
	require.NotContains(t, credentials, "access_token")

	extra := map[string]any{"unrelated": true}
	for _, key := range service.AccountSchedulingThresholdFullDrawExtraKeys() {
		extra[key] = map[string]any{"month": "2026-10", "windows": []any{1.0}}
	}
	filtered := filterSchedulerExtra(extra)
	for _, key := range service.AccountSchedulingThresholdFullDrawExtraKeys() {
		require.Contains(t, filtered, key)
		// Recording a full draw only refreshes the account snapshot.
		require.False(t, shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{key: extra[key]}))
	}
	require.NotContains(t, filtered, "unrelated")
}
