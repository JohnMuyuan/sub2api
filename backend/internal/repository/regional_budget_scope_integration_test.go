//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/regional/ledger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRegionalBudgetScopeReservesEveryDimension(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	for _, dimension := range []string{"key", "key_rate", "account", "platform", "subscription"} {
		t.Run(dimension, func(t *testing.T) {
			user := regionalLedgerUser(t)
			t.Cleanup(func() { _, _ = integrationDB.Exec("DELETE FROM regional_budget_allocations WHERE user_id=$1", user) })
			g := &service.Group{Name: "scope-" + uuid.NewString(), Platform: service.PlatformOpenAI, RateMultiplier: 1}
			if dimension == "subscription" {
				g.SubscriptionType = "subscription"
				limit := 7.0
				g.DailyLimitUSD = &limit
			}
			group := mustCreateGroup(t, client, g)
			key := mustCreateApiKey(t, client, &service.APIKey{UserID: user, GroupID: &group.ID, Key: "sk-mock-" + uuid.NewString(), Name: "scope"})
			account := mustCreateAccount(t, client, &service.Account{Name: "scope-" + uuid.NewString(), Type: service.AccountTypeAPIKey, Platform: service.PlatformOpenAI, Status: "active", Schedulable: true})
			_, err := integrationDB.Exec("INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)", account.ID, group.ID)
			require.NoError(t, err)
			switch dimension {
			case "key":
				_, err = integrationDB.Exec("UPDATE api_keys SET quota=7 WHERE id=$1", key.ID)
			case "key_rate":
				_, err = integrationDB.Exec("UPDATE api_keys SET rate_limit_5h=7 WHERE id=$1", key.ID)
			case "account":
				_, err = integrationDB.Exec("UPDATE accounts SET extra=jsonb_build_object('quota_daily_limit',7) WHERE id=$1", account.ID)
			case "platform":
				_, err = integrationDB.Exec("INSERT INTO user_platform_quotas(user_id,platform,daily_limit_usd) VALUES($1,'openai',7)", user)
			case "subscription":
				_, err = integrationDB.Exec("INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES($1,$2,NOW()-interval '1 minute',NOW()+interval '1 day','active')", user, group.ID)
			}
			require.NoError(t, err)
			primary, _, _ := NewRegionalFundsAuthorities(integrationDB, func(context.Context, *sql.Tx) error { return nil })
			first := regionalLedgerLease(fmt.Sprintf("scope-first-%d", user), user, 4_000_000_000)
			first.APIKeyID = key.ID
			first.AccountID = account.ID
			if dimension == "subscription" {
				first.BillingMode = "subscription"
			}
			require.NoError(t, primary.Issue(ctx, first))
			second := first
			second.LeaseID = fmt.Sprintf("scope-second-%d", user)
			second.RegionID = "eu"
			second.IssuedAt = time.Now().Add(-time.Second).Truncate(time.Microsecond)
			second.ExpiresAt = second.IssuedAt.Add(time.Hour)
			require.ErrorIs(t, primary.Issue(ctx, second), ledger.ErrExhausted)
			var balance string
			require.NoError(t, integrationDB.QueryRow("SELECT balance::text FROM users WHERE id=$1", user).Scan(&balance))
			if dimension == "subscription" {
				require.Equal(t, "10.00000000", balance)
			} else {
				require.Equal(t, "6.00000000", balance)
			}
		})
	}
}
