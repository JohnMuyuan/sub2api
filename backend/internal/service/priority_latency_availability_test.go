//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exercise real selection, sticky/ownership, slot waits and forwarding guards.
// These regressions also run unmodified on 9620d315 in the remote RED checkout.
func availabilityFixture(t *testing.T, latencies ...float64) (*OpenAIGatewayService, *defaultOpenAIAccountScheduler, OpenAIAccountScheduleRequest) {
	t.Helper()
	svc, scheduler, req, _, _ := latencyModeFixture(t, PriorityLatencyStop)
	accounts := make([]Account, len(latencies))
	pool := make([]openAIAccountCandidateScore, len(latencies))
	signals := make(map[int64]PrioritySchedulingSignal, len(latencies))
	bad := time.Now().Add(-time.Minute)
	for i, latency := range latencies {
		id := int64(i + 1)
		pool[i] = priorityCandidate(id, .1, 0)
		account := pool[i].account
		account.Status, account.Schedulable = StatusActive, true
		account.GroupIDs = []int64{11}
		accounts[i] = *account
		signals[id] = PrioritySchedulingSignal{Samples: 20, P90TTFTMs: latency, LatestBadLatencyAt: &bad, QualitySamples: 10, QualityPassed: 10}
	}
	svc.accountRepo = schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}
	svc.usageLogRepo = &priorityReaderStub{signal: signals}
	svc.priorityScheduling.mu.Lock()
	svc.priorityScheduling.entries = nil
	svc.priorityScheduling.launches = [2]time.Time{}
	svc.priorityScheduling.mu.Unlock()
	cfg := svc.prioritySchedulingRuntimeConfig()
	require.Eventually(t, func() bool { return svc.priorityHistory(req, cfg, pool).ready() }, 3*time.Second, time.Millisecond)
	return svc, scheduler, req
}

func TestPriorityLatencyAvailabilityPoolBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		latencies []float64
		allowed   int
	}{
		{"only one", []float64{16000}, 1},
		{"minimum two", []float64{16000, 17000, 18000, 19000, 20000, 21000}, 2},
		{"lowest band three", []float64{16000, 16000, 16000, 19000, 20000, 21000}, 3},
		{"maximum five", []float64{16000, 16000, 16000, 16000, 16000, 16000, 16000}, 5},
		{"healthy wins", []float64{16000, 17000, 500, 19000, 20000, 21000}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, scheduler, req := availabilityFixture(t, tc.latencies...)
			var acquired []int64
			results := map[int64]bool{}
			for i := range tc.latencies {
				results[int64(i+1)] = false
			}
			svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: results, acquiredIDs: &acquired})
			selected, err := modeSelect(t, scheduler, req)
			require.NoError(t, err, "all stopped must open a bounded availability pool")
			require.NotNil(t, selected)
			require.NotNil(t, selected.WaitPlan)
			got := map[int64]bool{}
			for _, id := range acquired {
				got[id] = true
			}
			require.Len(t, got, tc.allowed, "slot probes must not expand the admitted pool")
			if tc.name == "healthy wins" {
				require.Equal(t, map[int64]bool{3: true}, got)
			} else {
				for i := 1; i <= tc.allowed; i++ {
					require.True(t, got[int64(i)], "only lowest latency ranks can enter the fallback")
				}
			}
			require.NoError(t, svc.CheckPriorityLatencySelection(context.Background(), selected), "post-slot guard must honor exactly the selected fallback")
			ctx := ContextWithSelectionPriorityRequest(context.Background(), selected)
			_, err = svc.beginPriorityLatencyTurn(ctx, nil, selected.Account, req.RequestedModel)
			require.NoError(t, err, "pre-write guard must honor the same fallback")
			if tc.name != "healthy wins" {
				snapshot := svc.PrioritySchedulingSnapshot()
				require.NotNil(t, snapshot)
				for _, score := range snapshot.Candidates {
					require.True(t, score.LatencyStopped)
					require.NotNil(t, score.LatencyPenaltyUntil)
				}
			}
		})
	}
}

func TestPriorityLatencyAvailabilityHistoryDegrades(t *testing.T) {
	for _, state := range []string{"pending", "error", "unavailable", "unknown after measured"} {
		t.Run(state, func(t *testing.T) {
			svc, scheduler, req := availabilityFixture(t, 16000, 17000, 18000, 19000, 20000, 21000)
			svc.priorityScheduling.mu.Lock()
			entry := svc.priorityScheduling.entries[priorityHistoryKey(req, svc.prioritySchedulingRuntimeConfig())]
			if state == "unknown after measured" {
				delete(entry.signals, 1)
			} else {
				entry.signals = nil
				entry.observed = time.Time{}
				entry.expires = time.Now().Add(time.Hour)
				if state == "error" {
					entry.failure = "query_failed"
				}
			}
			svc.priorityScheduling.mu.Unlock()
			if state == "unavailable" {
				svc.usageLogRepo = nil
			}
			selected, err := modeSelect(t, scheduler, req)
			require.NoError(t, err, "history state alone must not cause a pool-wide 503")
			require.NotNil(t, selected)
			require.NoError(t, svc.CheckPriorityLatencySelection(context.Background(), selected))
			if state == "unknown after measured" {
				require.Contains(t, []int64{2, 3}, selected.Account.ID, "unknown must not rank as measured fastest")
			}
			snapshot := svc.PrioritySchedulingSnapshot()
			require.NotNil(t, snapshot)
			if state != "unknown after measured" {
				require.False(t, snapshot.HistoryReady)
				for _, score := range snapshot.Candidates {
					require.Zero(t, score.Samples)
					require.Nil(t, score.SelectedTTFTMs)
					require.False(t, score.LatencyStopped)
				}
			}
		})
	}
}

func TestPriorityLatencyAvailabilityBoundContinuation(t *testing.T) {
	svc, scheduler, req := availabilityFixture(t, 16000, 17000, 18000, 19000, 20000, 21000)
	for _, owner := range []int64{1, 6} {
		id := "resp_availability_" + string(rune('a'+owner))
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), 11, id, owner, time.Hour))
		for _, movable := range []bool{false, true} {
			bound := req
			bound.PreviousResponseID, bound.PreviousResponseCanMove, bound.StickyWeighted = id, movable, movable
			selected, err := modeSelect(t, scheduler, bound)
			if owner == 1 {
				require.NoError(t, err)
				require.NotNil(t, selected)
				require.Equal(t, owner, selected.Account.ID)
			} else {
				require.Nil(t, selected)
				require.ErrorIs(t, err, ErrPriorityLatencyUnavailable)
			}
			got, err := svc.getOpenAIWSStateStore().GetResponseAccount(context.Background(), 11, id)
			require.NoError(t, err)
			require.Equal(t, owner, got, "never delete ownership to evade latency admission")
		}
	}
}

func TestPriorityLatencyAvailabilityEligibilityAndScope(t *testing.T) {
	for _, gate := range []string{"disabled", "wrong group", "model", "privacy", "excluded"} {
		t.Run(gate, func(t *testing.T) {
			svc, scheduler, req := availabilityFixture(t, 16000, 17000, 500)
			repo := svc.accountRepo.(schedulerGroupAwareOpenAIAccountRepo)
			switch gate {
			case "disabled":
				repo.accounts[2].Schedulable = false
			case "wrong group":
				repo.accounts[2].GroupIDs = []int64{12}
			case "model":
				repo.accounts[2].Credentials = map[string]any{"model_mapping": map[string]any{"only-other-model": "other"}}
			case "privacy":
				req.RequirePrivacySet = true
				repo.accounts[0].Extra["privacy_mode"] = PrivacyModeTrainingOff
				repo.accounts[1].Extra["privacy_mode"] = PrivacyModeTrainingOff
			case "excluded":
				req.ExcludedIDs = map[int64]struct{}{3: {}}
			}
			svc.accountRepo = repo
			selected, err := modeSelect(t, scheduler, req)
			require.NoError(t, err, "ineligible healthy account must not hide authorized fallback")
			require.NotNil(t, selected)
			require.Contains(t, []int64{1, 2}, selected.Account.ID)
		})
	}
	svc, scheduler, req := availabilityFixture(t, 16000, 17000, 18000)
	selected, err := modeSelect(t, scheduler, req)
	require.NoError(t, err)
	require.NotNil(t, selected)
	other := req
	group := int64(12)
	other.GroupID = &group
	selected, err = modeSelect(t, scheduler, other)
	require.Nil(t, selected)
	require.Error(t, err, "fallback must not import accounts across groups")
	require.False(t, errors.Is(err, context.Canceled))
}
