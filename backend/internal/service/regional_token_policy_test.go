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

func TestRegionalCredentialShadowAndParentLookupNeverUsePrimary(t *testing.T) {
	ctx := context.Background()
	replica := regionalTestReplica(t, filepath.Join(t.TempDir(), "shadow"), true)
	parent := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "local-parent", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}
	shadow := &Account{ID: 2, ParentAccountID: &parent.ID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	// Scheduler export flattens shadow credentials and rejects parent pointers
	// in the wire payload. Keep the stale selected shadow outside the snapshot.
	projected := *shadow
	projected.ParentAccountID = nil
	projected.Credentials = parent.Credentials
	require.NoError(t, replica.PublishNext(ctx, "test", time.Minute, regionalDoc(t, parent, &projected)))
	reader, err := NewRegionalSchedulerReader(replica)
	require.NoError(t, err)
	snapshot := NewSchedulerSnapshotService(nil, nil, nil, nil, nil)
	snapshot.SetLocalSnapshotReader(reader)
	gateway := &OpenAIGatewayService{schedulerSnapshot: snapshot, openAITokenProvider: &OpenAITokenProvider{regionalSnapshot: snapshot}}
	token, _, err := gateway.GetAccessToken(ctx, shadow)
	require.NoError(t, err)
	require.Equal(t, "local-parent", token)
	require.Equal(t, parent.ID, gateway.parentAccountLookup(ctx)(parent.ID).ID)
	require.NoError(t, replica.PublishNext(ctx, "test", time.Minute, regionalDoc(t, &projected)))
	token, _, err = gateway.GetAccessToken(ctx, shadow)
	require.NoError(t, err)
	require.Equal(t, "local-parent", token)
	require.Nil(t, gateway.parentAccountLookup(ctx)(parent.ID))
	require.NoError(t, replica.PublishNext(ctx, "test", time.Minute, regionalDoc(t)))
	_, _, err = gateway.GetAccessToken(ctx, shadow)
	require.Error(t, err)
}
