package service

import (
	"context"
	"errors"
)

// RegionalAdmissionReadiness observes availability. A successful check never
// reserves capacity and must not replace transactional admission per request.
type RegionalAdmissionReadiness interface {
	Check(context.Context) error
}

type RegionalReadinessCheck func(context.Context) error

func (f RegionalReadinessCheck) Check(ctx context.Context) error {
	if f == nil {
		return errors.New("regional dependency is not configured")
	}
	return f(ctx)
}

// RegionalAdmissionDependencies is immutable once supplied to a running server.
// Missing dependencies deliberately prevent a partially wired deployment from
// appearing ready. Errors contain component names, never upstream credentials.
type RegionalAdmissionDependencies struct {
	AuthScheduler RegionalAdmissionReadiness
	Slots         RegionalAdmissionReadiness
	Funds         RegionalAdmissionReadiness
	Account       RegionalAdmissionReadiness
}

func (r *RegionalAdmissionDependencies) Check(ctx context.Context) error {
	if r == nil {
		return errors.New("regional admission is not configured")
	}
	for _, dependency := range []struct {
		name  string
		check RegionalAdmissionReadiness
	}{
		{"auth_scheduler", r.AuthScheduler},
		{"slots", r.Slots},
		{"funds", r.Funds},
		{"account", r.Account},
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if dependency.check == nil || dependency.check.Check(ctx) != nil {
			return errors.New("regional dependency unavailable: " + dependency.name)
		}
	}
	return ctx.Err()
}

// RegionalSnapshotReadiness checks the actual installed subscribers, not
// freshly reloaded configuration or a publisher's unrelated status.
func RegionalSnapshotReadiness(auth *APIKeyService, scheduler *SchedulerSnapshotService) RegionalAdmissionReadiness {
	return RegionalReadinessCheck(func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if auth == nil || scheduler == nil || auth.localAuth == nil || auth.regionalAuthRuntime == nil || scheduler.regionalRuntime == nil {
			return errors.New("regional snapshots are not installed")
		}
		for _, runtime := range []*regionalSchedulerRuntime{auth.regionalAuthRuntime, scheduler.regionalRuntime} {
			if runtime.replica == nil || runtime.cfg.Mode != "subscriber" {
				return errors.New("regional subscriber is not installed")
			}
			select {
			case <-runtime.done:
				return errors.New("regional subscriber stopped")
			default:
			}
			if _, _, err := runtime.snapshot(); err != nil {
				return errors.New("regional snapshot is not current")
			}
		}
		return ctx.Err()
	})
}
