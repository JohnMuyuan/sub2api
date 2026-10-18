//go:build unit

package admin

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type viewerAdminStub struct {
	service.AdminService
	accounts []service.Account
}

func (s *viewerAdminStub) GetAccountsByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	out := []*service.Account{}
	for _, id := range ids {
		for i := range s.accounts {
			if s.accounts[i].ID == id {
				out = append(out, &s.accounts[i])
			}
		}
	}
	return out, nil
}

func TestAccountViewerAuthorizeScopesAccountsAndBlocksSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &viewerAdminStub{accounts: []service.Account{{ID: 1, GroupIDs: []int64{10}}, {ID: 2, GroupIDs: []int64{20}}}}
	h := &AccountHandler{adminService: svc}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAccountViewer)
		c.Request = c.Request.WithContext(service.WithAccountViewerScope(c.Request.Context(), []int64{10}))
	})
	r.Use(h.AuthorizeObserver)
	ok := func(c *gin.Context) { c.Status(204) }
	r.GET("/api/v1/admin/accounts", ok)
	r.GET("/api/v1/admin/accounts/:id/stats", ok)
	r.POST("/api/v1/admin/accounts/:id/test", ok)
	r.PUT("/api/v1/admin/accounts/:id", ok)
	r.DELETE("/api/v1/admin/accounts/:id", ok)
	call := func(method, path string) int {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w.Code
	}
	require.Equal(t, 204, call("GET", "/api/v1/admin/accounts"))
	require.Equal(t, 400, call("GET", "/api/v1/admin/accounts?search=alice@example.com"))
	require.Equal(t, 204, call("GET", "/api/v1/admin/accounts/1/stats"))
	require.Equal(t, 204, call("POST", "/api/v1/admin/accounts/1/test"))
	require.Equal(t, 403, call("GET", "/api/v1/admin/accounts/2/stats"))
	require.Equal(t, 403, call("POST", "/api/v1/admin/accounts/2/test"))
	require.Equal(t, 403, call("PUT", "/api/v1/admin/accounts/1"))
	require.Equal(t, 403, call("DELETE", "/api/v1/admin/accounts/1"))
}

func TestAccountViewerGrantChangesRequireAdminStepUp(t *testing.T) {
	r, _ := setupRoleStepUpRouter(t)
	require.Equal(t, 401, doJSON(t, r, "POST", "/api/v1/admin/users", map[string]any{"email": "viewer@example.test", "password": "pass123", "role": "account_viewer"}).Code)
	require.Equal(t, 401, doJSON(t, r, "PUT", "/api/v1/admin/users/1", map[string]any{"role": "account_viewer", "observer_group_ids": []int64{10}}).Code)
}
