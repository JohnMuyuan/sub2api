//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountViewerJWTEnforcesReadOnlyCapabilitiesAndFreshGrants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := service.NewAuthService(nil, nil, nil, nil, &config.Config{JWT: config.JWTConfig{Secret: "viewer-auth-test", ExpireHour: 1}}, nil, nil, nil, nil, nil, nil, nil, nil)
	user := &service.User{ID: 12, Role: service.RoleAccountViewer, Status: service.StatusActive, ObserverGroupIDs: []int64{7}}
	repo := &stubUserRepo{getByID: func(context.Context, int64) (*service.User, error) { clone := *user; return &clone, nil }}
	token, err := auth.GenerateToken(t.Context(), user)
	require.NoError(t, err)
	r := gin.New()
	r.Use(gin.HandlerFunc(NewAdminAuthMiddleware(auth, service.NewUserService(repo, nil, nil, nil), nil, nil)))
	var grants []int64
	handler := func(c *gin.Context) { grants, _ = service.ObserverGroupIDs(c.Request.Context()); c.Status(204) }
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/admin/account-viewer/accounts"}, {"GET", "/api/v1/admin/account-viewer/accounts/:id/stats"}, {"GET", "/api/v1/admin/accounts"}, {"GET", "/api/v1/admin/accounts/data"}, {"GET", "/api/v1/admin/accounts/:id/export-session/:task_id"}, {"GET", "/api/v1/admin/settings/system-timezone"}, {"PUT", "/api/v1/admin/settings/system-timezone"}, {"GET", "/api/v1/admin/proxies/all"}, {"PUT", "/api/v1/admin/accounts/:id"}, {"DELETE", "/api/v1/admin/accounts/:id"}, {"POST", "/api/v1/admin/accounts/:id/test"}, {"POST", "/api/v1/admin/accounts/:id/refresh"}, {"POST", "/api/v1/admin/accounts/bulk-update"}} {
		r.Handle(route.method, route.path, handler)
	}
	call := func(method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	require.Equal(t, 204, call("GET", "/api/v1/admin/account-viewer/accounts"))
	require.Equal(t, []int64{7}, grants)
	require.Equal(t, 204, call("GET", "/api/v1/admin/account-viewer/accounts/1/stats"))
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/admin/accounts"}, {"GET", "/api/v1/admin/accounts/data"}, {"GET", "/api/v1/admin/accounts/1/export-session/task"}, {"GET", "/api/v1/admin/settings/system-timezone"}, {"PUT", "/api/v1/admin/settings/system-timezone"}, {"GET", "/api/v1/admin/proxies/all"}, {"PUT", "/api/v1/admin/accounts/1"}, {"DELETE", "/api/v1/admin/accounts/1"}, {"POST", "/api/v1/admin/accounts/1/test"}, {"POST", "/api/v1/admin/accounts/1/refresh"}, {"POST", "/api/v1/admin/accounts/bulk-update"}} {
		require.Equal(t, http.StatusForbidden, call(route.method, route.path), route.path)
	}
	user.ObserverGroupIDs = []int64{9}
	require.Equal(t, 204, call("GET", "/api/v1/admin/account-viewer/accounts"))
	require.Equal(t, []int64{9}, grants)
	user.ObserverGroupIDs = nil
	require.Equal(t, 204, call("GET", "/api/v1/admin/account-viewer/accounts"))
	require.Empty(t, grants)
	user.Role = service.RoleUser
	require.Equal(t, 403, call("GET", "/api/v1/admin/account-viewer/accounts"))
}

func TestAccountViewerAllowlistDeniesMutationsAndFutureEndpoints(t *testing.T) {
	for route := range observerAccountRoutes {
		method, path, _ := strings.Cut(route, " ")
		require.False(t, AccountViewerRouteAllowed(method, path), route)
	}
	require.False(t, AccountViewerRouteAllowed("POST", "/api/v1/admin/account-viewer/accounts"))
	require.False(t, AccountViewerRouteAllowed("GET", "/api/v1/admin/account-viewer/accounts/:id/credentials"))
}
