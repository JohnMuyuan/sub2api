package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// accountWindowPrimeResponse adds the built-in model choices to the settings.
type accountWindowPrimeResponse struct {
	service.AccountWindowPrimeSettings
	ModelOptions map[string][]string `json:"model_options"`
}

func (h *SettingHandler) GetAccountWindowPrime(c *gin.Context) {
	response.Success(c, accountWindowPrimeResponse{
		AccountWindowPrimeSettings: h.settingService.GetAccountWindowPrimeSettings(c.Request.Context()),
		ModelOptions:               service.AccountWindowPrimeModelOptions(),
	})
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
	response.Success(c, accountWindowPrimeResponse{AccountWindowPrimeSettings: settings, ModelOptions: service.AccountWindowPrimeModelOptions()})
}
