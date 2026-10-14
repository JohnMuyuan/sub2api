// Package accountstate coordinates shared accounts without regional ownership.
package accountstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrUnavailable = errors.New("regional account state unavailable")
var ErrConflict = errors.New("regional account operation identity conflict")

type RPMGrant struct {
	ID        string
	Region    string
	AccountID int64
	Minute    int64
	Units     int
}

func (g RPMGrant) Validate() error {
	if g.ID == "" || len(g.ID) > 128 || g.Region == "" || len(g.Region) > 63 || g.AccountID <= 0 || g.Minute <= 0 || g.Units <= 0 {
		return ErrConflict
	}
	return nil
}

type RPMLimit func(context.Context, *sql.Tx, int64) (int, error)
type RPMPrimary struct {
	DB    *sql.DB
	Limit RPMLimit
	Guard func(context.Context, *sql.Tx) error
}

func rpmLock(ctx context.Context, tx *sql.Tx, id int64, minute int64) error {
	if id <= 0 {
		return ErrConflict
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO regional_rpm_budgets(account_id,minute) VALUES($1,$2) ON CONFLICT DO NOTHING", id, minute); err != nil {
		return err
	}
	var locked int64
	return tx.QueryRowContext(ctx, "SELECT account_id FROM regional_rpm_budgets WHERE account_id=$1 AND minute=$2 FOR UPDATE", id, minute).Scan(&locked)
}
func rpmMinute(ctx context.Context, tx *sql.Tx) (int64, error) {
	var m int64
	err := tx.QueryRowContext(ctx, "SELECT floor(extract(epoch FROM clock_timestamp())/60)::bigint").Scan(&m)
	return m, err
}
func (p *RPMPrimary) Issue(ctx context.Context, g RPMGrant) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if p.Limit == nil {
		return ErrUnavailable
	}
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if p.Guard != nil {
		if err := p.Guard(ctx, tx); err != nil {
			return err
		}
	}
	if err = rpmLock(ctx, tx, g.AccountID, g.Minute); err != nil {
		return err
	}
	var old RPMGrant
	err = tx.QueryRowContext(ctx, "SELECT grant_id,region_id,account_id,minute,units FROM regional_rpm_grants WHERE grant_id=$1", g.ID).Scan(&old.ID, &old.Region, &old.AccountID, &old.Minute, &old.Units)
	if err == nil {
		if old != g {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	minute, err := rpmMinute(ctx, tx)
	if err != nil {
		return err
	}
	if g.Minute != minute {
		return ErrUnavailable
	}
	limit, err := p.Limit(ctx, tx, g.AccountID)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE regional_rpm_budgets SET granted=granted+$3 WHERE account_id=$1 AND minute=$2 AND primary_used+granted<=$4::integer-$3::integer AND minute=floor(extract(epoch FROM clock_timestamp())/60)::bigint", g.AccountID, g.Minute, g.Units, limit)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrUnavailable
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO regional_rpm_grants(grant_id,region_id,account_id,minute,units) VALUES($1,$2,$3,$4,$5)", g.ID, g.Region, g.AccountID, g.Minute, g.Units); err != nil {
		return err
	}
	return tx.Commit()
}
func (p *RPMPrimary) Acquire(ctx context.Context, id int64) (bool, int, time.Time, error) {
	if p.Limit == nil {
		return false, 0, time.Time{}, ErrUnavailable
	}
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()
	minute, err := rpmMinute(ctx, tx)
	if err != nil {
		return false, 0, time.Time{}, err
	}
	reset := time.Unix((minute+1)*60, 0)
	if err = rpmLock(ctx, tx, id, minute); err != nil {
		return false, 0, reset, err
	}
	currentMinute, err := rpmMinute(ctx, tx)
	if err != nil {
		return false, 0, reset, err
	}
	if currentMinute != minute {
		return false, 0, reset, ErrUnavailable
	}

	limit, err := p.Limit(ctx, tx, id)
	if err != nil {
		return false, 0, reset, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT primary_used+granted FROM regional_rpm_budgets WHERE account_id=$1 AND minute=$2", id, minute).Scan(&count); err != nil {
		return false, 0, reset, err
	}
	if limit <= 0 || count >= limit {
		return false, count, reset, nil
	}
	result, err := tx.ExecContext(ctx, "UPDATE regional_rpm_budgets SET primary_used=primary_used+1 WHERE account_id=$1 AND minute=$2 AND minute=floor(extract(epoch FROM clock_timestamp())/60)::bigint", id, minute)
	if err != nil {
		return false, count, reset, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, count, reset, err
	}
	if n != 1 {
		return false, count, reset, ErrUnavailable
	}
	return true, count + 1, reset, tx.Commit()
}

type RPMLocal struct {
	DB     *sql.DB
	Region string
}

func (l *RPMLocal) Install(ctx context.Context, g RPMGrant) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if g.Region != l.Region {
		return ErrConflict
	}
	result, err := l.DB.ExecContext(ctx, "INSERT INTO regional_local_rpm_grants(grant_id,region_id,account_id,minute,units) VALUES($1,$2,$3,$4,$5) ON CONFLICT(grant_id) DO UPDATE SET grant_id=EXCLUDED.grant_id WHERE regional_local_rpm_grants.region_id=EXCLUDED.region_id AND regional_local_rpm_grants.account_id=EXCLUDED.account_id AND regional_local_rpm_grants.minute=EXCLUDED.minute AND regional_local_rpm_grants.units=EXCLUDED.units", g.ID, g.Region, g.AccountID, g.Minute, g.Units)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (l *RPMLocal) Acquire(ctx context.Context, id int64) (bool, int, time.Time, error) {
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()
	minute, err := rpmMinute(ctx, tx)
	if err != nil {
		return false, 0, time.Time{}, err
	}
	reset := time.Unix((minute+1)*60, 0)
	if err = rpmLock(ctx, tx, id, minute); err != nil {
		return false, 0, reset, err
	}
	currentMinute, err := rpmMinute(ctx, tx)
	if err != nil {
		return false, 0, reset, err
	}
	if currentMinute != minute {
		return false, 0, reset, ErrUnavailable
	}

	var clockOK bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM regional_local_account_states WHERE account_id=$1 AND clock_valid_until>clock_timestamp() AND NOT invalid AND valid_until>clock_timestamp() AND cooldown_until<=clock_timestamp() AND extract(second FROM clock_timestamp())>=2 AND extract(second FROM clock_timestamp())<58)", id).Scan(&clockOK); err != nil {
		return false, 0, reset, err
	}
	if !clockOK {
		return false, 0, reset, ErrUnavailable
	}
	var used int
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(sum(used),0) FROM regional_local_rpm_grants WHERE region_id=$1 AND account_id=$2 AND minute=$3", l.Region, id, minute).Scan(&used); err != nil {
		return false, 0, reset, err
	}
	var grant string
	err = tx.QueryRowContext(ctx, "SELECT grant_id FROM regional_local_rpm_grants WHERE region_id=$1 AND account_id=$2 AND minute=$3 AND used<units ORDER BY grant_id LIMIT 1 FOR UPDATE", l.Region, id, minute).Scan(&grant)
	if errors.Is(err, sql.ErrNoRows) {
		return false, used, reset, nil
	}
	if err != nil {
		return false, used, reset, err
	}
	result, err := tx.ExecContext(ctx, "UPDATE regional_local_rpm_grants SET used=used+1 WHERE grant_id=$1 AND used<units AND minute=floor(extract(epoch FROM clock_timestamp())/60)::bigint AND EXISTS(SELECT 1 FROM regional_local_account_states WHERE account_id=$2 AND clock_valid_until>clock_timestamp() AND NOT invalid AND valid_until>clock_timestamp() AND cooldown_until<=clock_timestamp() AND extract(second FROM clock_timestamp())>=2 AND extract(second FROM clock_timestamp())<58)", grant, id)
	if err != nil {
		return false, used, reset, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, used, reset, err
	}
	if n != 1 {
		return false, used, reset, ErrUnavailable
	}
	return true, used + 1, reset, tx.Commit()
}

type RPMRemote interface {
	IssueRPM(context.Context, RPMGrant) error
}

// Exact payload is persisted before transport. Minute rollover never refunds
// unconfirmed units and never reuses that minute's ID with different units.
func (l *RPMLocal) Fetch(ctx context.Context, remote RPMRemote, g RPMGrant) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if g.Region != l.Region {
		return ErrConflict
	}
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	result, err := l.DB.ExecContext(ctx, "INSERT INTO regional_rpm_operations(operation_id,payload) VALUES($1,$2) ON CONFLICT(operation_id) DO UPDATE SET operation_id=EXCLUDED.operation_id WHERE regional_rpm_operations.payload=EXCLUDED.payload", g.ID, string(raw))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if err = remote.IssueRPM(ctx, g); err != nil {
		return err
	}
	if err = l.Install(ctx, g); err != nil {
		return err
	}
	_, err = l.DB.ExecContext(ctx, "UPDATE regional_rpm_operations SET installed=true WHERE operation_id=$1", g.ID)
	return err
}
