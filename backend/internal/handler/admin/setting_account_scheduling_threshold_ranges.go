package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountSchedulingThresholdRangesResponse struct {
	Ranges map[string]service.AccountSchedulingThresholdRange `json:"ranges"`
}

func (h *SettingHandler) GetAccountSchedulingThresholdRanges(c *gin.Context) {
	response.Success(c, accountSchedulingThresholdRangesResponse{
		Ranges: h.settingService.GetAccountSchedulingThresholdRanges(c.Request.Context()),
	})
}

func (h *SettingHandler) UpdateAccountSchedulingThresholdRanges(c *gin.Context) {
	var req struct {
		Ranges map[string]service.AccountSchedulingThresholdRange `json:"ranges"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Ranges == nil {
		response.BadRequest(c, "ranges is required")
		return
	}
	ranges, err := h.settingService.SetAccountSchedulingThresholdRanges(c.Request.Context(), req.Ranges)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, accountSchedulingThresholdRangesResponse{Ranges: ranges})
}
