package accountstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"time"
)

// Reports only restrict an account. Recovery requires a new authority revision;
// a region cannot clear an invalidation or shorten another region's cooldown.
type Event struct {
	ID            string
	Region        string
	AccountID     int64
	CooldownUntil time.Time
	Invalid       bool
}
type State struct {
	ServerTime    time.Time
	AccountID     int64
	Revision      int64
	CooldownUntil time.Time
	Invalid       bool
	ValidUntil    time.Time
}

func (e Event) Validate() error {
	if e.ID == "" || len(e.ID) > 128 || e.Region == "" || len(e.Region) > 63 || e.AccountID <= 0 || e.CooldownUntil.IsZero() {
		return ErrConflict
	}
	return nil
}

type States struct {
	DB     *sql.DB
	Region string
}

func (s *States) Report(ctx context.Context, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Region != s.Region {
		return ErrConflict
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	result, err := s.DB.ExecContext(ctx, "INSERT INTO regional_account_state_events(event_id,region_id,account_id,payload) VALUES($1,$2,$3,$4) ON CONFLICT(event_id) DO UPDATE SET event_id=EXCLUDED.event_id WHERE regional_account_state_events.payload=EXCLUDED.payload", e.ID, e.Region, e.AccountID, string(raw))
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

// Apply commits the idempotency record and authoritative account restriction in
// one transaction. Account snapshots read these canonical columns on next export.
func (s *States) Apply(ctx context.Context, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var account int64
	if err = tx.QueryRowContext(ctx, "SELECT id FROM accounts WHERE id=$1 AND deleted_at IS NULL FOR UPDATE", e.AccountID).Scan(&account); err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	var previous []byte
	err = tx.QueryRowContext(ctx, "SELECT payload FROM regional_account_state_events WHERE event_id=$1", e.ID).Scan(&previous)
	if err == nil {
		var old Event
		if json.Unmarshal(previous, &old) != nil || old.ID != e.ID || old.Region != e.Region || old.AccountID != e.AccountID || old.Invalid != e.Invalid || !old.CooldownUntil.Equal(e.CooldownUntil) {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO regional_account_states(account_id,revision,cooldown_until,invalid) VALUES($1,1,$2,$3) ON CONFLICT(account_id) DO UPDATE SET revision=regional_account_states.revision+1,cooldown_until=GREATEST(regional_account_states.cooldown_until,EXCLUDED.cooldown_until),invalid=regional_account_states.invalid OR EXCLUDED.invalid", e.AccountID, e.CooldownUntil, e.Invalid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE accounts SET rate_limit_reset_at=GREATEST(COALESCE(rate_limit_reset_at,$2),$2),schedulable=CASE WHEN $3 THEN false ELSE schedulable END,updated_at=clock_timestamp() WHERE id=$1", e.AccountID, e.CooldownUntil, e.Invalid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO regional_account_state_events(event_id,region_id,account_id,payload,acknowledged) VALUES($1,$2,$3,$4,true)", e.ID, e.Region, e.AccountID, string(raw)); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *States) Snapshot(ctx context.Context, id int64) (State, error) {
	var out State
	// Version also includes ordinary primary updates, so recovery/revocation from
	// admin or OAuth becomes visible even without a regional report.
	err := s.DB.QueryRowContext(ctx, "SELECT id,floor(extract(epoch FROM updated_at)*1000000)::bigint,GREATEST(COALESCE(rate_limit_reset_at,to_timestamp(0)),COALESCE(temp_unschedulable_until,to_timestamp(0)),COALESCE(overload_until,to_timestamp(0))),status<>'active' OR NOT schedulable OR deleted_at IS NOT NULL,clock_timestamp()+interval '15 seconds',clock_timestamp() FROM accounts WHERE id=$1", id).Scan(&out.AccountID, &out.Revision, &out.CooldownUntil, &out.Invalid, &out.ValidUntil, &out.ServerTime)
	return out, err
}
func (s *States) Install(ctx context.Context, v State) error {
	if v.AccountID <= 0 || v.Revision <= 0 || v.ValidUntil.IsZero() || v.CooldownUntil.IsZero() {
		return ErrConflict
	}
	result, err := s.DB.ExecContext(ctx, "INSERT INTO regional_local_account_states(account_id,revision,cooldown_until,invalid,valid_until) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id) DO UPDATE SET revision=EXCLUDED.revision,cooldown_until=EXCLUDED.cooldown_until,invalid=EXCLUDED.invalid,valid_until=EXCLUDED.valid_until WHERE regional_local_account_states.revision<EXCLUDED.revision OR (regional_local_account_states.revision=EXCLUDED.revision AND regional_local_account_states.cooldown_until=EXCLUDED.cooldown_until AND regional_local_account_states.invalid=EXCLUDED.invalid AND regional_local_account_states.valid_until<=EXCLUDED.valid_until)", v.AccountID, v.Revision, v.CooldownUntil, v.Invalid, v.ValidUntil)
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
func (s *States) Allowed(ctx context.Context, id int64) error {
	var allowed bool
	err := s.DB.QueryRowContext(ctx, "SELECT NOT invalid AND cooldown_until<=clock_timestamp() AND valid_until>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM regional_account_state_events e WHERE e.account_id=$1 AND NOT e.acknowledged AND ((e.payload->>'Invalid')::boolean OR (e.payload->>'CooldownUntil')::timestamptz>clock_timestamp())) FROM regional_local_account_states WHERE account_id=$1", id).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrUnavailable
	}
	return nil
}

// StateBatch bounds one authority observation and one regional installation.
// ServerTime is measured by the authority DB after the account read completes.
type StateBatch struct {
	States     []State
	ServerTime time.Time
}

func (s *States) SnapshotBatch(ctx context.Context, ids []int64) (StateBatch, error) {
	var out StateBatch
	if len(ids) == 0 || len(ids) > 100 {
		return out, ErrConflict
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return out, ErrConflict
		}
		seen[id] = true
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, "SELECT id,floor(extract(epoch FROM updated_at)*1000000)::bigint,GREATEST(COALESCE(rate_limit_reset_at,to_timestamp(0)),COALESCE(temp_unschedulable_until,to_timestamp(0)),COALESCE(overload_until,to_timestamp(0))),status<>'active' OR NOT schedulable OR deleted_at IS NOT NULL FROM accounts WHERE id=ANY($1) ORDER BY id", pq.Array(ids))
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v State
		if err = rows.Scan(&v.AccountID, &v.Revision, &v.CooldownUntil, &v.Invalid); err != nil {
			_ = rows.Close()
			return out, err
		}
		out.States = append(out.States, v)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.States) != len(ids) {
		return out, ErrUnavailable
	}
	if err = tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&out.ServerTime); err != nil {
		return out, err
	}
	for i := range out.States {
		out.States[i].ServerTime = out.ServerTime
		out.States[i].ValidUntil = out.ServerTime.Add(15 * time.Second)
	}
	return out, tx.Commit()
}

type StateRemote interface {
	ReportState(context.Context, Event) error
	GetStates(context.Context, []int64) (StateBatch, error)
}

func (s *States) FlushReports(ctx context.Context, remote StateRemote) error {
	rows, err := s.DB.QueryContext(ctx, "SELECT payload FROM regional_account_state_events WHERE region_id=$1 AND NOT acknowledged ORDER BY created_at,event_id LIMIT 100", s.Region)
	if err != nil {
		return err
	}
	var events []Event
	for rows.Next() {
		var raw []byte
		var e Event
		if err = rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &e); err != nil {
			_ = rows.Close()
			return err
		}
		events = append(events, e)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, e := range events {
		if err = remote.ReportState(ctx, e); err != nil {
			return err
		}
	}
	// Acknowledgement waits until a fresh batch is installed. Retrying the same
	// report is idempotent and cannot unblock a stale pre-report snapshot.
	return nil
}

// Sync performs one bounded round trip for the complete account assignment.
// Reports use an independent worker; a slow report cannot starve clock proofs.
func (s *States) Sync(ctx context.Context, remote StateRemote, ids []int64) error {
	if len(ids) == 0 || len(ids) > 100 {
		return ErrConflict
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var before time.Time
	if err := s.DB.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&before); err != nil {
		return err
	}
	batch, err := remote.GetStates(ctx, ids)
	if err != nil {
		return err
	}
	var after time.Time
	if err = s.DB.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&after); err != nil {
		return err
	}
	if batch.ServerTime.IsZero() || after.Before(before) || after.Sub(before) > time.Second || batch.ServerTime.Before(before.Add(-time.Second)) || batch.ServerTime.After(after.Add(time.Second)) {
		return ErrUnavailable
	}
	expected := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || expected[id] {
			return ErrConflict
		}
		expected[id] = true
	}
	if len(batch.States) != len(expected) {
		return ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, v := range batch.States {
		if !expected[v.AccountID] || v.Revision <= 0 || v.CooldownUntil.IsZero() || !v.ServerTime.Equal(batch.ServerTime) || !v.ValidUntil.Equal(batch.ServerTime.Add(15*time.Second)) {
			return ErrConflict
		}
		delete(expected, v.AccountID)
		result, err := tx.ExecContext(ctx, "INSERT INTO regional_local_account_states(account_id,revision,cooldown_until,invalid,valid_until,clock_valid_until) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(account_id) DO UPDATE SET revision=EXCLUDED.revision,cooldown_until=EXCLUDED.cooldown_until,invalid=EXCLUDED.invalid,valid_until=EXCLUDED.valid_until,clock_valid_until=EXCLUDED.clock_valid_until WHERE regional_local_account_states.revision<EXCLUDED.revision OR (regional_local_account_states.revision=EXCLUDED.revision AND regional_local_account_states.cooldown_until=EXCLUDED.cooldown_until AND regional_local_account_states.invalid=EXCLUDED.invalid AND regional_local_account_states.valid_until<=EXCLUDED.valid_until)", v.AccountID, v.Revision, v.CooldownUntil, v.Invalid, v.ValidUntil, before.Add(5*time.Second))
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
		// Only a snapshot visibly containing the restriction acknowledges its
		// report. Reports newer than this snapshot remain locally fail-closed.
		if _, err = tx.ExecContext(ctx, "UPDATE regional_account_state_events SET acknowledged=true WHERE region_id=$1 AND account_id=$2 AND NOT acknowledged AND (payload->>'CooldownUntil')::timestamptz<=$3 AND (NOT (payload->>'Invalid')::boolean OR $4)", s.Region, v.AccountID, v.CooldownUntil, v.Invalid); err != nil {
			return err
		}
	}
	// Do not commit stale proofs after database installation consumed the budget.
	var finished time.Time
	if err = tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&finished); err != nil {
		return err
	}
	if finished.Before(after) || finished.Sub(before) > time.Second {
		return ErrUnavailable
	}
	return tx.Commit()
}
