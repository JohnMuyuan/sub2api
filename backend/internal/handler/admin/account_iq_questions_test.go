package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type iqQuestionsSettingRepoStub struct {
	values map[string]string
}

func (r *iqQuestionsSettingRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	v, ok := r.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: v}, nil
}

func (r *iqQuestionsSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	v, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return v, nil
}

func (r *iqQuestionsSettingRepoStub) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *iqQuestionsSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *iqQuestionsSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *iqQuestionsSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *iqQuestionsSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestIQCustomQuestionsHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	repo := &iqQuestionsSettingRepoStub{values: map[string]string{}}
	settings := service.NewSettingService(repo, cfg)
	h := &AccountHandler{cfg: cfg}
	h.SetCodexTicketSettings(settings)

	r := gin.New()
	r.GET("/iq-custom-questions", h.ListIQCustomQuestions)
	r.POST("/iq-custom-questions", h.CreateIQCustomQuestion)
	r.PUT("/iq-custom-questions/:id", h.UpdateIQCustomQuestion)
	r.DELETE("/iq-custom-questions/:id", h.DeleteIQCustomQuestion)

	// Empty list initially.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/iq-custom-questions", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Data []service.IQCustomQuestion `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Empty(t, listResp.Data)

	// Create question.
	createBody := `{"title":"代码调试题","prompt":"找出死锁原因","output_type":"text","delivery_contract":"分点作答","reference_note":"锁顺序相反"}`
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/iq-custom-questions", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var createResp struct {
		Data service.IQCustomQuestion `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	require.NotEmpty(t, createResp.Data.ID)
	require.Equal(t, "代码调试题", createResp.Data.Title)

	// Update question.
	updateBody := `{"title":"代码调试题（更新）","prompt":"找出死锁并修复","output_type":"html","delivery_contract":"","reference_note":"已更新"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/iq-custom-questions/"+createResp.Data.ID, bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var updateResp struct {
		Data service.IQCustomQuestion `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updateResp))
	require.Equal(t, "代码调试题（更新）", updateResp.Data.Title)
	require.Equal(t, "html", updateResp.Data.OutputType)

	// Invalid payload returns 400.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/iq-custom-questions", bytes.NewBufferString(`{"title":"","prompt":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Delete question.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/iq-custom-questions/"+createResp.Data.ID, nil))
	require.Equal(t, http.StatusOK, w.Code)

	// Delete again returns 404.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/iq-custom-questions/"+createResp.Data.ID, nil))
	require.Equal(t, http.StatusNotFound, w.Code)
}
