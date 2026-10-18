//go:build unit

package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
)

type systemTimezoneHandlerRepo struct {
	service.SettingRepository
	value  string
	writes int
}

func (r *systemTimezoneHandlerRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{timezone.SettingKey: r.value}, nil
}
func (r *systemTimezoneHandlerRepo) Set(_ context.Context, _, value string) error {
	r.value = value
	r.writes++
	return nil
}

func TestSystemTimezoneHandlerValidatesAndPreservesOmittedValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &systemTimezoneHandlerRepo{value: "UTC"}
	h := &SettingHandler{settingService: service.NewSettingService(repo, &config.Config{Timezone: "UTC"})}
	router := gin.New()
	router.PUT("/timezone", h.UpdateSystemTimezone)
	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{}`, 400}, {`{"timezone":null}`, 400}, {`{"timezone":"Invalid/Zone"}`, 400}, {`{"timezone":"Local"}`, 400},
		{`{"timezone":"Asia/Taipei"}`, 200}, {`{"timezone":""}`, 200},
	} {
		request := httptest.NewRequest("PUT", "/timezone", strings.NewReader(tt.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, tt.status, response.Code, tt.body)
	}
	require.Equal(t, 2, repo.writes)
	require.Empty(t, repo.value)
}
