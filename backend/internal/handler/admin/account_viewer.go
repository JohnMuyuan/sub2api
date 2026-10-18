package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func accountViewerRequestAllowed(c *gin.Context) bool {
	role, _ := middleware.GetUserRoleFromContext(c)
	_, scoped := service.ObserverGroupIDs(c.Request.Context())
	if role == service.RoleAdmin || (role == service.RoleAccountViewer && scoped) {
		return true
	}
	response.Forbidden(c, "Account statistics access required")
	return false
}

func (h *AccountHandler) ListForAccountViewer(c *gin.Context) {
	if !accountViewerRequestAllowed(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	page, size := response.ParsePagination(c)
	items := []service.AccountViewerAccount{}
	if ids, scoped := service.ObserverGroupIDs(c.Request.Context()); scoped && len(ids) == 0 {
		response.Paginated(c, items, 0, page, size)
		return
	}
	// Searching raw account names would let a viewer probe hidden email text.
	if c.Query("search") != "" {
		response.BadRequest(c, "Search is unavailable on the read-only account surface")
		return
	}
	accounts, total, err := h.adminService.ListAccounts(c.Request.Context(), page, size, "", "", "", "", 0, "", "name", "asc")
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Unable to load account statistics")
		return
	}
	for i := range accounts {
		if !service.ObserverCanManageAccount(c.Request.Context(), &accounts[i]) {
			response.Forbidden(c, "Account is outside the assigned groups")
			return
		}
		items = append(items, service.AccountForViewer(c.Request.Context(), &accounts[i]))
	}
	response.Paginated(c, items, total, page, size)
}

// Numeric aggregates only. No per-user records, endpoint URLs, prompts, model
// metadata or errors from upstream requests are exposed on the viewer surface.
func (h *AccountHandler) StatsForAccountViewer(c *gin.Context) {
	if !accountViewerRequestAllowed(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), id)
	if err != nil || account == nil || !service.ObserverCanManageAccount(c.Request.Context(), account) {
		response.Forbidden(c, "Account is outside the assigned groups")
		return
	}
	days := 30
	if raw := c.Query("days"); raw != "" {
		days, err = strconv.Atoi(raw)
		if err != nil || days < 1 || days > 90 {
			response.BadRequest(c, "Days must be between 1 and 90")
			return
		}
	}
	now := timezone.Now()
	end := timezone.StartOfDay(now.AddDate(0, 0, 1))
	start := timezone.StartOfDay(now.AddDate(0, 0, -days+1))
	stats, err := h.accountUsageService.GetAccountUsageStats(c.Request.Context(), id, start, end)
	if err != nil || stats == nil {
		response.Error(c, http.StatusInternalServerError, "Unable to load account statistics")
		return
	}
	type dayStats struct {
		Date     string  `json:"date"`
		Requests int64   `json:"requests"`
		Tokens   int64   `json:"tokens"`
		Cost     float64 `json:"cost"`
	}
	history := make([]dayStats, 0, len(stats.History))
	for _, day := range stats.History {
		history = append(history, dayStats{day.Date, day.Requests, day.Tokens, day.ActualCost})
	}
	response.Success(c, gin.H{
		"summary": gin.H{"days": stats.Summary.Days, "requests": stats.Summary.TotalRequests, "tokens": stats.Summary.TotalTokens, "cost": stats.Summary.TotalCost},
		"history": history,
	})
}
