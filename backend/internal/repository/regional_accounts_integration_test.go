//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/regional/accountstate"
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
