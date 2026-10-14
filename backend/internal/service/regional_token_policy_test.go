//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func TestRegionalTokenPolicyNeverUsesPrimaryRefreshOrCache(t *testing.T) {
	ctx := context.Background()
	replica := regionalTestReplica(t, filepath.Join(t.TempDir(), "snapshot"), true)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "test-token", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}
	require.NoError(t, replica.PublishNext(ctx, "test", time.Minute, regionalDoc(t, account)))
	reader, err := NewRegionalSchedulerReader(replica)
	require.NoError(t, err)
	snapshot := NewSchedulerSnapshotService(nil, nil, nil, nil, nil)
	snapshot.SetLocalSnapshotReader(reader)
	providers := []func(context.Context, *Account) (string, error){(&OpenAITokenProvider{regionalSnapshot: snapshot}).GetAccessToken, (&ClaudeTokenProvider{regionalSnapshot: snapshot}).GetAccessToken, (&GeminiTokenProvider{regionalSnapshot: snapshot}).GetAccessToken, (&AntigravityTokenProvider{regionalSnapshot: snapshot}).GetAccessToken, (&GrokTokenProvider{regionalSnapshot: snapshot}).GetAccessToken}
	for _, get := range providers {
		token, err := get(ctx, account)
		require.NoError(t, err)
		require.Equal(t, "test-token", token)
	}
	account.Credentials["expires_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339)
	require.NoError(t, replica.PublishNext(ctx, "test", time.Minute, regionalDoc(t, account)))
	for _, get := range providers {
		_, err := get(ctx, account)
		require.Error(t, err)
	}
	require.NoError(t, replica.PublishNext(ctx, "test", time.Minute, regionalDoc(t)))
	for _, get := range providers {
		_, err := get(ctx, account)
		require.Error(t, err)
	}
}
