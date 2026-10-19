package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetAccountWindowPrime(c *gin.Context) {
	response.Success(c, h.settingService.GetAccountWindowPrimeSettings(c.Request.Context()))
}

func (h *SettingHandler) UpdateAccountWindowPrime(c *gin.Context) {
	var req service.AccountWindowPrimeSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid account window prime settings")
		return
	}
	settings, err := h.settingService.SetAccountWindowPrimeSettings(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}
