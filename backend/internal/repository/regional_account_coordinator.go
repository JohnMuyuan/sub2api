package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/regional/accountstate"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"time"
)

type regionalAccountCoordinator struct {
	db      *sql.DB
	region  string
	primary *accountstate.RPMPrimary
	local   *accountstate.RPMLocal
	states  *accountstate.States
	targets []RegionalRPMTarget
}
type RegionalRPMTarget struct {
	AccountID int64
	Units     int
}

func newRegionalRPMPrimary(db *sql.DB) *accountstate.RPMPrimary {
	return &accountstate.RPMPrimary{DB: db, Limit: func(ctx context.Context, tx *sql.Tx, id int64) (int, error) {
		var raw []byte
		if err := tx.QueryRowContext(ctx, "SELECT extra FROM accounts WHERE id=$1 AND deleted_at IS NULL AND status='active' AND schedulable AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at<=clock_timestamp()) AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until<=clock_timestamp()) FOR SHARE", id).Scan(&raw); err != nil {
			return 0, err
		}
		a := &service.Account{}
		if err := json.Unmarshal(raw, &a.Extra); err != nil {
			return 0, err
		}
		limit := a.GetBaseRPM()
		if limit <= 0 {
			return 0, accountstate.ErrUnavailable
		}
		return limit, nil
	}}
}
func ProvideRPMCache(rdb *redis.Client, db *sql.DB, cfg *config.Config) (service.RPMCache, error) {
	if cfg.Runtime.RegionalConcurrencyFile == "" {
		return NewRPMCache(rdb), nil
	}
	c, err := readRegionalConcurrencyConfig(cfg.Runtime.RegionalConcurrencyFile)
	if err != nil {
		return nil, err
	}
	coordinator := &regionalAccountCoordinator{db: db, states: &accountstate.States{DB: db, Region: c.Region}, targets: c.RPMTargets}
	if c.Mode == "primary" {
		coordinator.primary = newRegionalRPMPrimary(db)
	} else {
		coordinator.region = c.Region
		coordinator.local = &accountstate.RPMLocal{DB: db, Region: c.Region}
	}
	return coordinator, nil
}
func (c *regionalAccountCoordinator) TryAcquireRPM(ctx context.Context, id int64, _ int) (bool, int, time.Time, error) {
	if c.local != nil {
		if err := c.states.Allowed(ctx, id); err != nil {
			return false, 0, time.Time{}, err
		}
		return c.local.Acquire(ctx, id)
	}
	return c.primary.Acquire(ctx, id)
}
func (c *regionalAccountCoordinator) IncrementRPM(ctx context.Context, id int64) (int, error) {
	ok, n, _, err := c.TryAcquireRPM(ctx, id, 0)
	if err != nil {
		return n, err
	}
	if !ok {
		return n, accountstate.ErrUnavailable
	}
	return n, nil
}
func (c *regionalAccountCoordinator) GetRPM(ctx context.Context, id int64) (int, error) {
	var n int
	var err error
	if c.local != nil {
		if err = c.states.Allowed(ctx, id); err != nil {
			return 0, err
		}
		err = c.db.QueryRowContext(ctx, "SELECT COALESCE(sum(used),0) FROM regional_local_rpm_grants WHERE region_id=$1 AND account_id=$2 AND minute=floor(extract(epoch FROM clock_timestamp())/60)::bigint", c.region, id).Scan(&n)
	} else {
		err = c.db.QueryRowContext(ctx, "SELECT primary_used+granted FROM regional_rpm_budgets WHERE account_id=$1 AND minute=floor(extract(epoch FROM clock_timestamp())/60)::bigint", id).Scan(&n)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
	}
	return n, err
}
func (c *regionalAccountCoordinator) GetRPMBatch(ctx context.Context, ids []int64) (map[int64]int, error) {
	out := map[int64]int{}
	for _, id := range ids {
		n, err := c.GetRPM(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, nil
}
func (c *regionalAccountCoordinator) RegionalAccountAllowed(ctx context.Context, id int64) error {
	if c.local == nil {
		var allowed bool
		err := c.db.QueryRowContext(ctx, "SELECT status='active' AND schedulable AND deleted_at IS NULL AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at<=clock_timestamp()) AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until<=clock_timestamp()) AND (overload_until IS NULL OR overload_until<=clock_timestamp()) FROM accounts WHERE id=$1", id).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed {
			return accountstate.ErrUnavailable
		}
		return nil
	}
	return c.states.Allowed(ctx, id)
}
func (c *regionalAccountCoordinator) RegionalAccountReport(ctx context.Context, id int64, status int) error {
	if c.local == nil {
		return nil
	}
	if status != 429 && status != 401 && status != 403 {
		return nil
	}
	e := accountstate.Event{ID: fmt.Sprintf("state-%s-%d", c.region, time.Now().UnixNano()), Region: c.region, AccountID: id, CooldownUntil: time.Now().UTC().Add(time.Minute), Invalid: status == 401 || status == 403}
	return c.states.Report(ctx, e)
}
func (c *regionalAccountCoordinator) Check(ctx context.Context) error {
	if c.local == nil {
		return nil
	}
	if len(c.targets) == 0 {
		return accountstate.ErrUnavailable
	}
	for _, target := range c.targets {
		if err := c.states.Allowed(ctx, target.AccountID); err != nil {
			return err
		}
		var ready bool
		err := c.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM regional_local_rpm_grants WHERE region_id=$1 AND account_id=$2 AND minute=floor(extract(epoch FROM clock_timestamp())/60)::bigint AND used<units AND EXISTS(SELECT 1 FROM regional_local_account_states WHERE account_id=$2 AND clock_valid_until>clock_timestamp() AND extract(second FROM clock_timestamp())>=2 AND extract(second FROM clock_timestamp())<58))", c.region, target.AccountID).Scan(&ready)
		if err != nil {
			return err
		}
		if !ready {
			return accountstate.ErrUnavailable
		}
	}
	return nil
}

var _ service.StrictRPMCache = (*regionalAccountCoordinator)(nil)

// A handler reservation may cross a database minute while queued. Validate the
// exact authority reset before transferring that unit into an actual send.
func (c *regionalAccountCoordinator) RegionalRPMReservationValid(ctx context.Context, id int64, reset time.Time) (bool, error) {
	var valid bool
	if c.local == nil {
		err := c.db.QueryRowContext(ctx, "SELECT to_timestamp((floor(extract(epoch FROM clock_timestamp())/60)::bigint+1)*60)=$1", reset).Scan(&valid)
		return valid, err
	}
	err := c.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM regional_local_account_states WHERE account_id=$1 AND NOT invalid AND cooldown_until<=clock_timestamp() AND valid_until>clock_timestamp() AND clock_valid_until>clock_timestamp() AND extract(second FROM clock_timestamp())>=2 AND extract(second FROM clock_timestamp())<58 AND to_timestamp((floor(extract(epoch FROM clock_timestamp())/60)::bigint+1)*60)=$2)", id, reset).Scan(&valid)
	return valid, err
}
