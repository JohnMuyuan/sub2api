//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/regional/accountstate"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestRegionalAccountRPMSharedBudgetAcrossPods(t *testing.T) {
	ctx := context.Background()
	id := time.Now().UnixNano()
	p := &accountstate.RPMPrimary{DB: integrationDB, Limit: func(context.Context, *sql.Tx, int64) (int, error) { return 4, nil }}
	var minute int64
	require.NoError(t, integrationDB.QueryRow("SELECT floor(extract(epoch FROM clock_timestamp())/60)::bigint").Scan(&minute))
	g := accountstate.RPMGrant{ID: fmt.Sprint(id), Region: "us", AccountID: id, Minute: minute, Units: 2}
	require.NoError(t, p.Issue(ctx, g))
	require.NoError(t, p.Issue(ctx, g))
	changed := g
	changed.Units++
	require.ErrorIs(t, p.Issue(ctx, changed), accountstate.ErrConflict)
	_, err := integrationDB.Exec("INSERT INTO regional_local_account_states(account_id,revision,cooldown_until,invalid,valid_until,clock_valid_until) VALUES($1,1,to_timestamp(0),false,clock_timestamp()+interval '1 minute',clock_timestamp()+interval '1 minute')", id)
	require.NoError(t, err)
	local := &accountstate.RPMLocal{DB: integrationDB, Region: "us"}
	require.NoError(t, local.Install(ctx, g))
	require.NoError(t, local.Install(ctx, g))
	var wg sync.WaitGroup
	results := make(chan bool, 32)
	errs := make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); ok, _, _, err := p.Acquire(ctx, id); results <- ok; errs <- err }()
		go func() {
			defer wg.Done()
			restarted := &accountstate.RPMLocal{DB: integrationDB, Region: "us"}
			ok, _, _, err := restarted.Acquire(ctx, id)
			results <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	count := 0
	for ok := range results {
		if ok {
			count++
		}
	}
	require.Equal(t, 4, count)
}
func TestRegionalAccountStateMonotonicAndExpired(t *testing.T) {
	ctx := context.Background()
	s := &accountstate.States{DB: integrationDB, Region: "us"}
	id := time.Now().UnixNano()
	state := accountstate.State{AccountID: id, Revision: 2, CooldownUntil: time.Now().UTC().Add(-time.Minute), ValidUntil: time.Now().UTC().Add(time.Minute)}
	require.NoError(t, s.Install(ctx, state))
	require.NoError(t, s.Allowed(ctx, id))
	old := state
	old.Revision--
	require.ErrorIs(t, s.Install(ctx, old), accountstate.ErrConflict)
	changed := state
	changed.Invalid = true
	require.ErrorIs(t, s.Install(ctx, changed), accountstate.ErrConflict)
	e := accountstate.Event{ID: fmt.Sprint(id), Region: "us", AccountID: id, CooldownUntil: time.Now().UTC().Add(time.Minute), Invalid: true}
	require.NoError(t, s.Report(ctx, e))
	require.NoError(t, s.Report(ctx, e))
	require.Error(t, s.Allowed(ctx, id))
	e.Invalid = false
	require.ErrorIs(t, s.Report(ctx, e), accountstate.ErrConflict)
	state.AccountID++
	state.ValidUntil = time.Now().UTC().Add(-time.Second)
	require.NoError(t, s.Install(ctx, state))
	require.Error(t, s.Allowed(ctx, state.AccountID))
}

type regionalBatchStateRemote struct {
	db    *sql.DB
	calls int
	omit  bool
	delay time.Duration
}

func (r *regionalBatchStateRemote) ReportState(context.Context, accountstate.Event) error { return nil }
func (r *regionalBatchStateRemote) GetStates(ctx context.Context, ids []int64) (accountstate.StateBatch, error) {
	r.calls++
	var now time.Time
	if err := r.db.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return accountstate.StateBatch{}, err
	}
	if r.delay > 0 {
		select {
		case <-ctx.Done():
			return accountstate.StateBatch{}, ctx.Err()
		case <-time.After(r.delay):
		}
	}
	batch := accountstate.StateBatch{ServerTime: now}
	for _, id := range ids {
		batch.States = append(batch.States, accountstate.State{AccountID: id, Revision: 1, CooldownUntil: now.Add(-time.Hour).Truncate(time.Microsecond), ValidUntil: now.Add(15 * time.Second), ServerTime: now})
	}
	if r.omit {
		batch.States = batch.States[:len(batch.States)-1]
	}
	return batch, nil
}
func TestRegionalAccountStateBatchOneRoundTripAndAtomicClockProof(t *testing.T) {
	ctx := context.Background()
	s := &accountstate.States{DB: integrationDB, Region: "batch"}
	ids := make([]int64, 32)
	start := time.Now().UnixNano()
	for i := range ids {
		ids[i] = start + int64(i)
	}
	remote := &regionalBatchStateRemote{db: integrationDB, delay: 30 * time.Millisecond}
	require.NoError(t, s.Sync(ctx, remote, ids))
	require.Equal(t, 1, remote.calls)
	var count, deadlines int
	require.NoError(t, integrationDB.QueryRow("SELECT count(*),count(DISTINCT clock_valid_until) FROM regional_local_account_states WHERE account_id=ANY($1) AND clock_valid_until>clock_timestamp()", pq.Array(ids)).Scan(&count, &deadlines))
	require.Equal(t, 32, count)
	require.Equal(t, 1, deadlines)
	missing := []int64{start + 100, start + 101}
	remote.omit = true
	require.Error(t, s.Sync(ctx, remote, missing))
	require.NoError(t, integrationDB.QueryRow("SELECT count(*) FROM regional_local_account_states WHERE account_id=ANY($1)", pq.Array(missing)).Scan(&count))
	require.Zero(t, count)
	remote.omit = false
	remote.delay = 2 * time.Second
	require.Error(t, s.Sync(ctx, remote, missing))
	require.NoError(t, integrationDB.QueryRow("SELECT count(*) FROM regional_local_account_states WHERE account_id=ANY($1)", pq.Array(missing)).Scan(&count))
	require.Zero(t, count)
}

// Exercise a real database minute boundary after obtaining the budget lock.
// No mocked clock or alternate SQL replaces the production transaction.
func TestRegionalAccountRPMRejectsRolloverAfterLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	id := time.Now().UnixNano()
	p := &accountstate.RPMPrimary{DB: integrationDB, Limit: func(ctx context.Context, tx *sql.Tx, id int64) (int, error) {
		_, err := tx.ExecContext(ctx, "SELECT pg_sleep(GREATEST(0,60-extract(second FROM clock_timestamp())+0.05))")
		return 4, err
	}}
	ok, _, _, err := p.Acquire(ctx, id)
	require.ErrorIs(t, err, accountstate.ErrUnavailable)
	require.False(t, ok)
	var used int
	require.NoError(t, integrationDB.QueryRow("SELECT COALESCE(sum(primary_used),0) FROM regional_rpm_budgets WHERE account_id=$1", id).Scan(&used))
	require.Zero(t, used)
}
