//go:build unit

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRegionalReadinessCompositionRejectsUnconfiguredProvider(t *testing.T) {
	cfg := &config.Config{Runtime: config.RuntimeConfig{Role: config.RuntimeRoleGateway, RegionalAuthFile: "auth.json"}}
	ready := ProvideRegionalAdmissionReadiness(cfg, nil, nil, nil, nil, nil)
	require.Error(t, ready.Check(t.Context()))
	for _, field := range []string{"auth", "scheduler", "slots", "funds"} {
		c := &config.Config{Runtime: config.RuntimeConfig{Role: config.RuntimeRoleGateway}}
		switch field {
		case "auth":
			c.Runtime.RegionalAuthFile = "auth"
		case "scheduler":
			c.Runtime.RegionalSchedulerFile = "scheduler"
		case "slots":
			c.Runtime.RegionalConcurrencyFile = "slots"
		case "funds":
			c.Runtime.RegionalFundsFile = "funds"
		}
		require.True(t, regionalRuntimeConfigured(c))
		lifecycle := NewLifecycle(func(context.Context) error { return nil })
		attachRegionalReadiness(lifecycle, c, nil)
		response := httptest.NewRecorder()
		lifecycle.Wrap(http.NotFoundHandler()).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
	}
}

func TestRegionalLifecycleRechecksAndKeepsLivenessIndependent(t *testing.T) {
	available := true
	check := service.RegionalReadinessCheck(func(context.Context) error {
		if !available {
			return errors.New("private failure")
		}
		return nil
	})
	ready := &service.RegionalAdmissionDependencies{AuthScheduler: check, Slots: check, Funds: check, Account: check}
	lifecycle := NewLifecycle(func(context.Context) error { return nil })
	attachRegionalReadiness(lifecycle, &config.Config{Runtime: config.RuntimeConfig{Role: config.RuntimeRoleGateway, RegionalAuthFile: "auth"}}, ready)
	handler := lifecycle.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	for _, value := range []bool{true, false, true} {
		available = value
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if value {
			require.Equal(t, http.StatusOK, response.Code)
		} else {
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
		}
		live := httptest.NewRecorder()
		handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health", nil))
		require.Equal(t, http.StatusOK, live.Code)
	}
	lifecycle.BeginDrain()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}
