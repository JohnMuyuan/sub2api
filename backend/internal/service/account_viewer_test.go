//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountViewerDTOHidesCredentialsEmailsAndOtherGroups(t *testing.T) {
	now := time.Now()
	reset := now.Add(time.Hour)
	account := &Account{ID: 1, Name: "Personal alice+one@example.com / 中文邮箱@例子.公司", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials:      map[string]any{"email": "alice+one@example.com", "access_token": "PRIVATE_TOKEN", "refresh_token": "PRIVATE_REFRESH", "cookie": "PRIVATE_COOKIE"},
		Extra:            map[string]any{"private_email": "secret@example.net", "passive_usage_sampled_at": now.Format(time.RFC3339), "session_window_utilization": 0.25, "passive_usage_7d_utilization": 0.5, "passive_usage_7d_reset": reset.Unix()},
		SessionWindowEnd: &reset, ErrorMessage: "error for hidden@example.com PRIVATE_ERROR", GroupIDs: []int64{10, 20},
		Groups: []*Group{{ID: 10, Name: "Pool owner@example.com"}, {ID: 20, Name: "PRIVATE_GROUP"}},
		Notes:  new("PRIVATE_NOTES"), ProxyID: new(int64(99)),
	}
	ctx := WithObserverScope(context.Background(), []int64{10})
	dto := AccountForViewer(ctx, account)
	data, err := json.Marshal(dto)
	require.NoError(t, err)
	for _, secret := range []string{"alice+one@example.com", "中文邮箱@例子.公司", "owner@example.com", "PRIVATE_", "credentials", "proxy_id", "extra", "error_message"} {
		require.NotContains(t, string(data), secret)
	}
	require.Len(t, dto.Groups, 1)
	require.Equal(t, int64(10), dto.Groups[0].ID)
	require.NotNil(t, dto.UsageWindows[0].UsedPercent)
	require.Equal(t, 25.0, *dto.UsageWindows[0].UsedPercent)
	require.Equal(t, "Personal alice+one@example.com / 中文邮箱@例子.公司", account.Name, "never mutate cached source identities")
	require.Len(t, account.Groups, 2)
}

func TestAccountViewerRoleIsDistinctFromObserverAndAdmin(t *testing.T) {
	role, err := normalizeUserRole(RoleAccountViewer, RoleUser)
	require.NoError(t, err)
	require.Equal(t, RoleAccountViewer, role)
	user := &User{Role: role}
	require.True(t, user.IsAccountViewer())
	require.False(t, user.IsAdmin())
	require.False(t, user.IsObserver())
	require.Empty(t, ObserverVisibleGroups(WithObserverScope(t.Context(), nil), []int64{1, 2}))
	require.False(t, ObserverCanManageAccount(WithObserverScope(t.Context(), nil), &Account{GroupIDs: []int64{1}}))
}
