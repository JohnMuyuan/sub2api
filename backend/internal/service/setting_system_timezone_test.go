//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type systemTimezoneRepo struct {
	SettingRepository
	value  string
	err    error
	writes int
}

func (r *systemTimezoneRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{timezone.SettingKey: r.value}, r.err
}
func (r *systemTimezoneRepo) Set(_ context.Context, key, value string) error {
	if r.err != nil {
		return r.err
	}
	r.value = value
	r.writes++
	return nil
}

func TestSystemTimezoneSettingPersistsWithoutChangingLiveClocks(t *testing.T) {
	repo := &systemTimezoneRepo{}
	svc := NewSettingService(repo, &config.Config{Timezone: "UTC", StartupTimezone: "UTC"})
	effective, local := timezone.Name(), time.Local
	require.NoError(t, svc.SetSystemTimezone(t.Context(), " Asia/Taipei "))
	state, err := svc.GetSystemTimezone(t.Context())
	require.NoError(t, err)
	require.Equal(t, "Asia/Taipei", state.Timezone)
	require.Equal(t, "UTC", state.FallbackTimezone)
	require.Equal(t, effective, state.EffectiveTimezone)
	require.Equal(t, effective != "Asia/Taipei", state.RestartRequired)
	require.Equal(t, effective, timezone.Name())
	require.Same(t, local, time.Local)
	require.Error(t, svc.SetSystemTimezone(t.Context(), "Local"))
	require.Equal(t, 1, repo.writes)
	require.NoError(t, svc.SetSystemTimezone(t.Context(), ""))
	state, err = svc.GetSystemTimezone(t.Context())
	require.NoError(t, err)
	require.Empty(t, state.Timezone)
	require.Equal(t, effective != "UTC", state.RestartRequired)
	repo.err = errors.New("write failed")
	require.Error(t, svc.SetSystemTimezone(t.Context(), "UTC"))
	require.Equal(t, 2, repo.writes)
	_, err = svc.GetSystemTimezone(t.Context())
	require.Error(t, err)
}
