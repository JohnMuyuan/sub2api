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
	var viewer bool
	handler := func(c *gin.Context) {
		grants, _ = service.ObserverGroupIDs(c.Request.Context())
		viewer = service.IsAccountViewerScope(c.Request.Context())
		c.Status(204)
	}
	allowed := []struct{ method, route, path string }{
		{"GET", "/api/v1/admin/accounts", "/api/v1/admin/accounts"},
		{"GET", "/api/v1/admin/accounts/:id/usage", "/api/v1/admin/accounts/1/usage"},
		{"GET", "/api/v1/admin/accounts/:id/stats", "/api/v1/admin/accounts/1/stats"},
		{"POST", "/api/v1/admin/accounts/:id/test", "/api/v1/admin/accounts/1/test"},
		{"GET", "/api/v1/admin/groups/all", "/api/v1/admin/groups/all"},
	}
	denied := []struct{ method, route, path string }{
		{"GET", "/api/v1/admin/accounts/:id", "/api/v1/admin/accounts/1"},
		{"GET", "/api/v1/admin/accounts/data", "/api/v1/admin/accounts/data"},
		{"GET", "/api/v1/admin/accounts/:id/export-session/:task_id", "/api/v1/admin/accounts/1/export-session/task"},
		{"GET", "/api/v1/admin/proxies/all", "/api/v1/admin/proxies/all"},
		{"GET", "/api/v1/admin/settings/system-timezone", "/api/v1/admin/settings/system-timezone"},
		{"POST", "/api/v1/admin/accounts", "/api/v1/admin/accounts"},
		{"PUT", "/api/v1/admin/accounts/:id", "/api/v1/admin/accounts/1"},
		{"DELETE", "/api/v1/admin/accounts/:id", "/api/v1/admin/accounts/1"},
		{"POST", "/api/v1/admin/accounts/:id/refresh", "/api/v1/admin/accounts/1/refresh"},
		{"POST", "/api/v1/admin/accounts/:id/schedulable", "/api/v1/admin/accounts/1/schedulable"},
		{"POST", "/api/v1/admin/accounts/:id/reset-quota", "/api/v1/admin/accounts/1/reset-quota"},
		{"DELETE", "/api/v1/admin/accounts/:id/temp-unschedulable", "/api/v1/admin/accounts/1/temp-unschedulable"},
		{"POST", "/api/v1/admin/accounts/bulk-update", "/api/v1/admin/accounts/bulk-update"},
		{"POST", "/api/v1/admin/accounts/batch-delete", "/api/v1/admin/accounts/batch-delete"},
	}
	for _, route := range append(append([]struct{ method, route, path string }{}, allowed...), denied...) {
		r.Handle(route.method, route.route, handler)
	}
	call := func(method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for _, route := range allowed {
		viewer = false
		require.Equal(t, 204, call(route.method, route.path), route.path)
		require.Equal(t, []int64{7}, grants)
		require.True(t, viewer, "viewer responses must be marked for redaction")
	}
	for _, route := range denied {
		require.Equal(t, http.StatusForbidden, call(route.method, route.path), route.method+" "+route.path)
	}
	user.ObserverGroupIDs = []int64{9}
	require.Equal(t, 204, call("GET", "/api/v1/admin/accounts"))
	require.Equal(t, []int64{9}, grants)
	user.ObserverGroupIDs = nil
	require.Equal(t, 204, call("GET", "/api/v1/admin/accounts"))
	require.Empty(t, grants)
	user.Role = service.RoleUser
	require.Equal(t, 403, call("GET", "/api/v1/admin/accounts"))
}

// Every viewer capability must also be an observer capability: the viewer is a
// strict subset, never a separate grant.
func TestAccountViewerAllowlistIsSubsetOfObserver(t *testing.T) {
	for route := range accountViewerRoutes {
		method, path, _ := strings.Cut(route, " ")
		require.True(t, ObserverAccountRouteAllowed(method, path), route)
	}
	require.False(t, AccountViewerRouteAllowed("GET", "/api/v1/admin/accounts/:id"))
	require.False(t, AccountViewerRouteAllowed("GET", "/api/v1/admin/account-viewer/accounts"))
}
