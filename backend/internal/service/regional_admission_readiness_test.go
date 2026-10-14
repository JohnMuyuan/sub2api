//go:build unit

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/regional"
	"github.com/stretchr/testify/require"
)

func TestRegionalAdmissionReadinessRequiresEveryDependency(t *testing.T) {
	var nilAggregate *RegionalAdmissionDependencies
	require.Error(t, nilAggregate.Check(t.Context()))
	var nilCheck RegionalReadinessCheck
	require.Error(t, nilCheck.Check(t.Context()))
	for missing := 0; missing < 4; missing++ {
		checks := make([]RegionalAdmissionReadiness, 4)
		for i := range checks {
			checks[i] = RegionalReadinessCheck(func(context.Context) error { return nil })
		}
		checks[missing] = nil
		r := &RegionalAdmissionDependencies{checks[0], checks[1], checks[2], checks[3]}
		require.Error(t, r.Check(t.Context()))
	}
}

func TestRegionalAdmissionReadinessRechecksAndRedacts(t *testing.T) {
	available := true
	calls := 0
	check := RegionalReadinessCheck(func(context.Context) error {
		calls++
		if !available {
			return errors.New("backend password=DO_NOT_EXPOSE")
		}
		return nil
	})
	r := &RegionalAdmissionDependencies{check, check, check, check}
	require.NoError(t, r.Check(t.Context()))
	require.Equal(t, 4, calls)
	available = false
	err := r.Check(t.Context())
	require.ErrorContains(t, err, "auth_scheduler")
	require.NotContains(t, err.Error(), "DO_NOT_EXPOSE")
	available = true
	require.NoError(t, r.Check(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, r.Check(ctx), context.Canceled)
	require.Equal(t, 9, calls)
}

func TestRegionalSnapshotReadinessRejectsMissingRuntime(t *testing.T) {
	require.Error(t, RegionalSnapshotReadiness(nil, nil).Check(t.Context()))
	require.Error(t, RegionalSnapshotReadiness(&APIKeyService{}, &SchedulerSnapshotService{}).Check(t.Context()))
}

func TestRegionalSnapshotReadinessChecksExpiryAndStoppedSubscribers(t *testing.T) {
	codec, err := regional.NewCodec(map[string][]byte{"test": bytes.Repeat([]byte{7}, 32)})
	require.NoError(t, err)
	makeRuntime := func(kind regional.SnapshotKind, expired bool) *regionalSchedulerRuntime {
		t.Helper()
		policy := regional.Config{RegionID: "test", MaxSnapshotAgeSeconds: 60}
		issued := time.Now().Add(-time.Second)
		if expired {
			issued = issued.Add(-time.Hour)
		}
		payload := []byte("{}")
		digest := sha256.Sum256(payload)
		wire, err := codec.Seal("test", regional.Snapshot{SchemaVersion: 1, RegionID: "test", Kind: kind, Revision: 1, IssuedAt: issued, ExpiresAt: issued.Add(time.Minute), Payload: payload, PayloadSHA256: hex.EncodeToString(digest[:])})
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "snapshot")
		require.NoError(t, os.WriteFile(path, wire, 0600))
		replica, err := regional.OpenReplica(path, policy, kind, codec, func([]byte) error { return nil }, false)
		require.NoError(t, err)
		return &regionalSchedulerRuntime{cfg: RegionalSchedulerRuntimeConfig{Mode: "subscriber"}, replica: replica, done: make(chan struct{})}
	}
	auth := &APIKeyService{localAuth: &RegionalAuthReader{}, regionalAuthRuntime: makeRuntime(regional.SnapshotAuth, false)}
	scheduler := &SchedulerSnapshotService{regionalRuntime: makeRuntime(regional.SnapshotScheduler, false)}
	check := RegionalSnapshotReadiness(auth, scheduler)
	require.NoError(t, check.Check(t.Context()))
	scheduler.regionalRuntime = makeRuntime(regional.SnapshotScheduler, true)
	require.ErrorContains(t, check.Check(t.Context()), "not current")
	scheduler.regionalRuntime = makeRuntime(regional.SnapshotScheduler, false)
	close(scheduler.regionalRuntime.done)
	require.ErrorContains(t, check.Check(t.Context()), "stopped")
	scheduler.regionalRuntime = makeRuntime(regional.SnapshotScheduler, false)
	auth.regionalAuthRuntime = makeRuntime(regional.SnapshotAuth, true)
	require.ErrorContains(t, check.Check(t.Context()), "not current")
}
