package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountSchedulingThresholdRangesResponse struct {
	Ranges      map[string]service.AccountSchedulingThresholdRange                 `json:"ranges"`
	WindowRules map[string]map[string]service.AccountSchedulingThresholdWindowRule `json:"window_rules"`
	// Windows lists each platform's usage windows for the per-window form.
	Windows map[string][]string `json:"windows"`
}

func (h *SettingHandler) GetAccountSchedulingThresholdRanges(c *gin.Context) {
	ctx := c.Request.Context()
	response.Success(c, accountSchedulingThresholdRangesResponse{
		Ranges:      h.settingService.GetAccountSchedulingThresholdRanges(ctx),
		WindowRules: h.settingService.GetAccountSchedulingThresholdWindowRules(ctx),
		Windows:     service.AccountSchedulingThresholdPlatformWindows,
	})
}

func (h *SettingHandler) UpdateAccountSchedulingThresholdRanges(c *gin.Context) {
	var req struct {
		Ranges map[string]service.AccountSchedulingThresholdRange `json:"ranges"`
		// Omitted window_rules keep the stored rules.
		WindowRules map[string]map[string]service.AccountSchedulingThresholdWindowRule `json:"window_rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Ranges == nil {
		response.BadRequest(c, "ranges is required")
		return
	}
	ranges, windowRules, err := h.settingService.SetAccountSchedulingThresholdRanges(c.Request.Context(), req.Ranges, req.WindowRules)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, accountSchedulingThresholdRangesResponse{
		Ranges:      ranges,
		WindowRules: windowRules,
		Windows:     service.AccountSchedulingThresholdPlatformWindows,
	})
}
