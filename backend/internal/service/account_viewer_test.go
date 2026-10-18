//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountViewerRedactionMasksEmailsWithoutMutatingInput(t *testing.T) {
	require.Equal(t, "Personal *** / ***", RedactAccountViewerText("Personal alice+one@example.com / 中文邮箱@例子.公司"))
	require.Equal(t, `"***"`, RedactAccountViewerText(`"bob@example.com"`))

	extra := map[string]any{"email_address": "bob@example.com", "nested": map[string]any{"list": []any{"carol@example.com", 3}}, "ratio": 0.5}
	out := RedactAccountViewerValue(extra).(map[string]any)
	require.Equal(t, "***", out["email_address"])
	require.Equal(t, []any{"***", 3}, out["nested"].(map[string]any)["list"])
	require.Equal(t, 0.5, out["ratio"])
	require.Equal(t, "bob@example.com", extra["email_address"], "never mutate cache-backed maps")
	require.Equal(t, "carol@example.com", extra["nested"].(map[string]any)["list"].([]any)[0])
}

func TestAccountViewerScopeIsObserverScopedButDistinct(t *testing.T) {
	ctx := WithAccountViewerScope(context.Background(), []int64{7})
	require.True(t, IsAccountViewerScope(ctx))
	ids, scoped := ObserverGroupIDs(ctx)
	require.True(t, scoped)
	require.Equal(t, []int64{7}, ids)
	require.False(t, IsAccountViewerScope(WithObserverScope(context.Background(), []int64{7})))
	require.False(t, ObserverCanManageAccount(WithAccountViewerScope(context.Background(), nil), &Account{GroupIDs: []int64{1}}))
}

func TestAccountViewerRoleIsDistinctFromObserverAndAdmin(t *testing.T) {
	role, err := normalizeUserRole(RoleAccountViewer, RoleUser)
	require.NoError(t, err)
	require.Equal(t, RoleAccountViewer, role)
	user := &User{Role: role}
	require.True(t, user.IsAccountViewer())
	require.False(t, user.IsAdmin())
	require.False(t, user.IsObserver())
}
