package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GetAccountWindowPrimeSettings returns the saved settings or defaults. The
// runner reads it once a minute, so it goes straight to the repository.
func (s *SettingService) GetAccountWindowPrimeSettings(ctx context.Context) AccountWindowPrimeSettings {
	if s == nil || s.settingRepo == nil {
		return DefaultAccountWindowPrimeSettings()
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountWindowPrime)
	if err != nil {
		if !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to get account window prime settings, using defaults", "error", err)
		}
		return DefaultAccountWindowPrimeSettings()
	}
	return parseAccountWindowPrimeSettings(raw)
}

// SetAccountWindowPrimeSettings validates and replaces the settings.
func (s *SettingService) SetAccountWindowPrimeSettings(ctx context.Context, input AccountWindowPrimeSettings) (AccountWindowPrimeSettings, error) {
	normalized, err := NormalizeAccountWindowPrimeSettings(input)
	if err != nil {
		return AccountWindowPrimeSettings{}, infraerrors.BadRequest("INVALID_ACCOUNT_WINDOW_PRIME_SETTINGS", err.Error())
	}
	blob, err := json.Marshal(normalized)
	if err != nil {
		return AccountWindowPrimeSettings{}, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAccountWindowPrime, string(blob)); err != nil {
		return AccountWindowPrimeSettings{}, err
	}
	return normalized, nil
}
