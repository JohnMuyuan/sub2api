package service

import (
	"context"
	"slices"
	"time"
)

type ProfitOperationsRepository interface {
	ReadProfitOperations(context.Context, UpstreamOperationsFilter) (*UpstreamOperationsSnapshot, error)
}

func ValidateProfitOperationsFilter(f UpstreamOperationsFilter) error {
	if !slices.Contains([]string{"requests", "standard_charge", "request_income", "estimated_cost", "estimated_profit", "margin", "name"}, f.Sort) {
		return ErrUpstreamOperationsValidation
	}
	f.Sort = "requests"
	return ValidateUpstreamOperationsFilter(f)
}
func (s *ChannelService) ProfitOperations(ctx context.Context, f UpstreamOperationsFilter) (*UpstreamOperationsSnapshot, error) {
	if err := ValidateProfitOperationsFilter(f); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	return s.repo.(ProfitOperationsRepository).ReadProfitOperations(ctx, f)
}

// OperationsCostMultiplier distinguishes the scheduling default from a saved
// estimate, and corrupt legacy metadata from an explicit zero cost.
func OperationsCostMultiplier(a *Account) (*float64, string) {
	if a == nil {
		return nil, "account_missing"
	}
	raw, present := a.Extra[AccountCostMultiplierExtraKey]
	if !present || raw == nil {
		v := DefaultAccountCostMultiplier
		return &v, "current_default_0.1"
	}
	v, ok := accountCostMultiplierNumber(raw)
	if !ok {
		return nil, "invalid_multiplier"
	}
	return &v, "current_saved_multiplier"
}
