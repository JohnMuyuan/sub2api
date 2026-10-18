//go:build unit

package dto

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountForObserverRedactsAccountViewerResponses(t *testing.T) {
	proxyID := int64(99)
	account := &service.Account{ID: 1, Name: "Personal alice@example.com", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive,
		Notes:        new("PRIVATE_NOTES"),
		Credentials:  map[string]any{"email": "alice@example.com", "access_token": "PRIVATE_TOKEN", "plan_type": "plus"},
		Extra:        map[string]any{"email_address": "alice@example.com", "codex_5h_used_percent": 25.0},
		ProxyID:      &proxyID,
		ErrorMessage: "token for alice@example.com expired",
		GroupIDs:     []int64{10, 20},
		Groups:       []*service.Group{{ID: 10, Name: "Pool owner@example.com"}, {ID: 20, Name: "PRIVATE_GROUP"}},
	}
	ctx := service.WithAccountViewerScope(context.Background(), []int64{10})
	out := AccountForObserver(ctx, AccountFromService(account))
	data, err := json.Marshal(out)
	require.NoError(t, err)
	for _, secret := range []string{"@example.com", "PRIVATE_", "proxy_id\":99"} {
		require.NotContains(t, string(data), secret)
	}
	require.Equal(t, "Personal ***", out.Name)
	require.Equal(t, "plus", out.Credentials["plan_type"])
	require.Equal(t, 25.0, out.Extra["codex_5h_used_percent"])
	require.Len(t, out.Groups, 1)
	require.Equal(t, "Pool ***", out.Groups[0].Name)

	require.Equal(t, "alice@example.com", account.Credentials["email"], "service account must stay untouched")
	require.Equal(t, "alice@example.com", account.Extra["email_address"])
	require.Equal(t, "Pool owner@example.com", account.Groups[0].Name)

	observer := AccountForObserver(service.WithObserverScope(context.Background(), []int64{10}), AccountFromService(account))
	require.Equal(t, "Personal alice@example.com", observer.Name, "observer responses keep full detail")
}
