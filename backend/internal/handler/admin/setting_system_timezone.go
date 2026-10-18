package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetSystemTimezone(c *gin.Context) {
	settings, err := h.settingService.GetSystemTimezone(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *SettingHandler) UpdateSystemTimezone(c *gin.Context) {
	var req struct {
		Timezone *string `json:"timezone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Timezone == nil {
		response.BadRequest(c, "timezone is required (empty follows startup configuration)")
		return
	}
	if err := h.settingService.SetSystemTimezone(c.Request.Context(), *req.Timezone); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.GetSystemTimezone(c)
}
