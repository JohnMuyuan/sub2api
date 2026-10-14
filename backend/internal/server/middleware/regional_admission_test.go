//go:build unit

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegionalAdmissionRechecksBeforeEachRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ready := false
	checks := 0
	forwarded := 0
	check := service.RegionalReadinessCheck(func(ctx context.Context) error {
		_, bounded := ctx.Deadline()
		require.True(t, bounded)
		checks++
		if !ready {
			return errors.New("private backend detail")
		}
		return nil
	})
	router := gin.New()
	router.Use(RegionalAdmission(check))
	router.POST("/v1/responses", func(c *gin.Context) {
		_, bounded := c.Request.Context().Deadline()
		require.False(t, bounded)
		forwarded++
		c.Status(http.StatusNoContent)
	})
	for _, available := range []bool{false, true, false} {
		ready = available
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
		if available {
			require.Equal(t, http.StatusNoContent, response.Code)
		} else {
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			require.NotContains(t, response.Body.String(), "private backend")
		}
	}
	require.Equal(t, 3, checks)
	require.Equal(t, 1, forwarded)
}

func TestRegionalAdmissionRejectsAbsentProvider(t *testing.T) {
	router := gin.New()
	router.Use(RegionalAdmission(nil))
	router.POST("/v1/responses", func(c *gin.Context) { t.Fatal("unconfigured provider admitted request") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}
