//go:build unit

package admin

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type viewerAdminStub struct {
	service.AdminService
	accounts  []service.Account
	listCalls int
}

func (s *viewerAdminStub) ListAccounts(_ context.Context, _ int, _ int, _ string, _ string, _ string, search string, _ int64, _ string, _ string, _ string) ([]service.Account, int64, error) {
	requireEmptySearch := search == ""
	if !requireEmptySearch {
		panic("private email search")
	}
	s.listCalls++
	return s.accounts, int64(len(s.accounts)), nil
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
func (s *viewerAdminStub) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			return &s.accounts[i], nil
		}
	}
	return nil, nil
}

type viewerUsageStub struct {
	service.UsageLogRepository
	calls int
}

func (s *viewerUsageStub) GetAccountUsageStats(_ context.Context, _ int64, _, _ time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	s.calls++
	return &usagestats.AccountUsageStatsResponse{Summary: usagestats.AccountUsageSummary{TotalRequests: 3, TotalTokens: 42, TotalCost: 0.5}, History: []usagestats.AccountUsageHistory{{Date: "2026-10-18", Label: "PRIVATE_LABEL@example.com", Requests: 3, Tokens: 42, ActualCost: 0.5}}, Models: []usagestats.ModelStat{{Model: "PRIVATE_MODEL@example.com"}}}, nil
}

func TestAccountViewerHandlersScopePrivacyAndNoCredentialSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &viewerAdminStub{accounts: []service.Account{{ID: 1, Name: "alice@example.com", GroupIDs: []int64{10, 20}, Groups: []*service.Group{{ID: 10, Name: "Allowed"}, {ID: 20, Name: "PRIVATE_GROUP"}}, Credentials: map[string]any{"access_token": "PRIVATE_TOKEN"}}, {ID: 2, Name: "other@example.com", GroupIDs: []int64{20}}}}
	usage := &viewerUsageStub{}
	h := &AccountHandler{adminService: svc, accountUsageService: service.NewAccountUsageService(nil, usage, nil, nil, nil, nil, nil, nil, nil, nil, nil)}
	grants := []int64{10}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUserRole), service.RoleAccountViewer)
		c.Request = c.Request.WithContext(service.WithObserverScope(c.Request.Context(), grants))
	})
	r.Use(h.AuthorizeObserver)
	r.GET("/api/v1/admin/account-viewer/accounts", h.ListForAccountViewer)
	r.GET("/api/v1/admin/account-viewer/accounts/:id/stats", h.StatsForAccountViewer)
	call := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := call("/api/v1/admin/account-viewer/accounts/1/stats")
	require.Equal(t, 200, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Body.String(), `"tokens":42`)
	require.NotContains(t, w.Body.String(), "PRIVATE_")
	require.NotContains(t, w.Body.String(), "alice@example.com")
	for _, id := range []string{"2", "999"} {
		w = call("/api/v1/admin/account-viewer/accounts/" + id + "/stats")
		require.Equal(t, 403, w.Code)
	}
	require.Equal(t, 1, usage.calls, "forbidden IDs must not reach stats queries")
	w = call("/api/v1/admin/account-viewer/accounts/1/stats?days=999")
	require.Equal(t, 400, w.Code)
	require.Equal(t, 1, usage.calls)
	w = call("/api/v1/admin/account-viewer/accounts?search=alice@example.com")
	require.Equal(t, 400, w.Code)
	require.Zero(t, svc.listCalls)
	w = call("/api/v1/admin/account-viewer/accounts")
	require.Equal(t, 403, w.Code, "defense in depth if a repository ever returns an out-of-scope row")
	svc.accounts = svc.accounts[:1]
	w = call("/api/v1/admin/account-viewer/accounts")
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "alice@example.com")
	require.NotContains(t, w.Body.String(), "PRIVATE_")
	require.Contains(t, w.Body.String(), `"name":"***"`)
	grants = nil
	before := svc.listCalls
	w = call("/api/v1/admin/account-viewer/accounts")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"total":0`)
	require.Contains(t, w.Body.String(), `"items":[]`)
	require.Equal(t, before, svc.listCalls)
}

func TestAccountViewerGrantChangesRequireAdminStepUp(t *testing.T) {
	r, _ := setupRoleStepUpRouter(t)
	require.Equal(t, 401, doJSON(t, r, "POST", "/api/v1/admin/users", map[string]any{"email": "viewer@example.test", "password": "pass123", "role": "account_viewer"}).Code)
	require.Equal(t, 401, doJSON(t, r, "PUT", "/api/v1/admin/users/1", map[string]any{"role": "account_viewer", "observer_group_ids": []int64{10}}).Code)
}
