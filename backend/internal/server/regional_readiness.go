package server

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ProvideRegionalAdmissionReadiness binds the exact instances used by requests.
// Unconfigured dependencies remain nil and the aggregate rejects admission.
func ProvideRegionalAdmissionReadiness(cfg *config.Config, auth *service.APIKeyService, scheduler *service.SchedulerSnapshotService, slots service.ConcurrencyCache, rpm service.RPMCache, funds *service.RegionalMetering) *service.RegionalAdmissionDependencies {
	r := &service.RegionalAdmissionDependencies{}
	if cfg == nil || cfg.Runtime.Role != config.RuntimeRoleGateway {
		return r
	}
	r.AuthScheduler = service.RegionalSnapshotReadiness(auth, scheduler)
	if check, ok := slots.(interface{ CheckRegionalSlots(context.Context) error }); ok {
		r.Slots = service.RegionalReadinessCheck(check.CheckRegionalSlots)
	}
	if check, ok := rpm.(service.RegionalAdmissionReadiness); ok {
		r.Account = check
	}
	if funds != nil {
		r.Funds = funds
	}
	return r
}

func regionalRuntimeConfigured(cfg *config.Config) bool {
	return cfg != nil && cfg.Runtime.Role == config.RuntimeRoleGateway && (cfg.Runtime.RegionalAuthFile != "" || cfg.Runtime.RegionalSchedulerFile != "" || cfg.Runtime.RegionalConcurrencyFile != "" || cfg.Runtime.RegionalFundsFile != "")
}

func attachRegionalReadiness(lifecycle *Lifecycle, cfg *config.Config, ready service.RegionalAdmissionReadiness) {
	if !regionalRuntimeConfigured(cfg) {
		return
	}
	lifecycle.checks = append(lifecycle.checks, service.RegionalReadinessCheck(func(ctx context.Context) error {
		if ready == nil {
			return (&service.RegionalAdmissionDependencies{}).Check(ctx)
		}
		return ready.Check(ctx)
	}).Check)
}
