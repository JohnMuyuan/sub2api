package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type SystemTimezoneSettings struct {
	Timezone          string `json:"timezone"`
	EffectiveTimezone string `json:"effective_timezone"`
	FallbackTimezone  string `json:"fallback_timezone"`
	RestartRequired   bool   `json:"restart_required"`
}

func (s *SettingService) GetSystemTimezone(ctx context.Context) (*SystemTimezoneSettings, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{timezone.SettingKey})
	if err != nil {
		return nil, err
	}
	fallback := "Asia/Shanghai"
	if s.cfg != nil {
		if s.cfg.StartupTimezone != "" {
			fallback = s.cfg.StartupTimezone
		} else if s.cfg.Timezone != "" {
			fallback = s.cfg.Timezone
		}
	}
	configured := strings.TrimSpace(values[timezone.SettingKey])
	desired := configured
	if desired == "" {
		desired = fallback
	}
	return &SystemTimezoneSettings{Timezone: configured, EffectiveTimezone: timezone.Name(), FallbackTimezone: fallback, RestartRequired: desired != timezone.Name()}, nil
}

func (s *SettingService) SetSystemTimezone(ctx context.Context, value string) error {
	value = strings.TrimSpace(value)
	if err := timezone.ValidateSetting(value); err != nil {
		return infraerrors.BadRequest("INVALID_SYSTEM_TIMEZONE", err.Error())
	}
	// Apply at startup: changing time.Local, SQL sessions, cron clocks and quota
	// caches in a live request would produce inconsistent time boundaries.
	return s.settingRepo.Set(ctx, timezone.SettingKey, value)
}
