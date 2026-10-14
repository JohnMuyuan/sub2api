package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/regional"
	"github.com/Wei-Shaw/sub2api/internal/regional/ledger"
)

// NewRegionalFundsAuthorities installs mandatory authoritative scope checks.
// Guard is shared with the deployment cutover fence; absent evidence refuses
// grants, never silently runs a partially migrated financial authority.
func NewRegionalFundsAuthorities(db *sql.DB, guard func(context.Context, *sql.Tx) error) (*ledger.Primary, *ledger.PrimaryRequests, ledger.VerifyEvent) {
	scope := RegionalBudgetScope{}
	primary := &ledger.Primary{DB: db}
	primary.Funding = regionalBalanceFunding
	primary.ReserveScope = func(ctx context.Context, tx *sql.Tx, l regional.Lease) error {
		if guard == nil {
			return errors.New("funds cutover evidence required")
		}
		if err := guard(ctx, tx); err != nil {
			return err
		}
		return scope.Reserve(ctx, tx, "grant:"+l.LeaseID, l.UserID, l.APIKeyID, l.AccountID, l.Granted)
	}
	primary.CloseScope = func(ctx context.Context, tx *sql.Tx, l regional.Lease, spent regional.Amount) error {
		return scope.Close(ctx, tx, "grant:"+l.LeaseID, spent)
	}
	verify := ledger.PricingVerifier(func(ctx context.Context, tx *sql.Tx, e ledger.Event) error {
		return scope.Charge(ctx, tx, "grant:"+e.Usage.LeaseID, e)
	})
	holds := &ledger.PrimaryRequests{DB: db}
	holds.Funding = regionalBalanceFunding
	holds.Authorize = func(ctx context.Context, tx *sql.Tx, r ledger.Reservation) error {
		return scope.Reserve(ctx, tx, "request:"+r.RequestID, r.UserID, r.APIKeyID, r.AccountID, r.Maximum)
	}
	holds.SettleScope = ledger.PricingVerifier(func(ctx context.Context, tx *sql.Tx, e ledger.Event) error {
		id := "request:" + e.Usage.EventID
		if err := scope.Charge(ctx, tx, id, e); err != nil {
			return err
		}
		return scope.Close(ctx, tx, id, e.Usage.Charge)
	})
	holds.CancelScope = func(ctx context.Context, tx *sql.Tx, r ledger.Reservation) error {
		return scope.Close(ctx, tx, "request:"+r.RequestID, 0)
	}
	return primary, holds, verify
}

func regionalBalanceFunding(ctx context.Context, tx *sql.Tx, user, key int64) (bool, error) {
	var locked int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL FOR UPDATE", user).Scan(&locked); err != nil {
		return false, err
	}
	var kind string
	if err := tx.QueryRowContext(ctx, "SELECT g.subscription_type FROM api_keys k JOIN groups g ON g.id=k.group_id WHERE k.id=$1 AND k.user_id=$2 AND k.deleted_at IS NULL AND g.deleted_at IS NULL FOR SHARE OF k,g", key, user).Scan(&kind); err != nil {
		return false, err
	}
	return kind != "subscription", nil
}
