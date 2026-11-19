package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ListIQCustomQuestions returns all saved custom intelligence test questions.
// GET /api/v1/admin/iq-custom-questions
func (h *AccountHandler) ListIQCustomQuestions(c *gin.Context) {
	if h.codexTicketSettings == nil {
		response.Success(c, []service.IQCustomQuestion{})
		return
	}
	items, err := h.codexTicketSettings.ListIQCustomQuestions(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

// CreateIQCustomQuestion creates and persists a custom intelligence test question.
// POST /api/v1/admin/iq-custom-questions
func (h *AccountHandler) CreateIQCustomQuestion(c *gin.Context) {
	var req service.IQCustomQuestionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	created, err := h.codexTicketSettings.CreateIQCustomQuestion(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, created)
}

// UpdateIQCustomQuestion updates an existing custom intelligence test question.
// PUT /api/v1/admin/iq-custom-questions/:id
func (h *AccountHandler) UpdateIQCustomQuestion(c *gin.Context) {
	var req service.IQCustomQuestionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updated, err := h.codexTicketSettings.UpdateIQCustomQuestion(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

// DeleteIQCustomQuestion deletes a custom intelligence test question by ID.
// DELETE /api/v1/admin/iq-custom-questions/:id
func (h *AccountHandler) DeleteIQCustomQuestion(c *gin.Context) {
	if err := h.codexTicketSettings.DeleteIQCustomQuestion(c.Request.Context(), c.Param("id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
