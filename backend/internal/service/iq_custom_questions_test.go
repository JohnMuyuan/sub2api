package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type iqCustomQuestionsSettingRepoStub struct {
	values map[string]string
}

func newIQCustomQuestionsSettingRepoStub() *iqCustomQuestionsSettingRepoStub {
	return &iqCustomQuestionsSettingRepoStub{values: map[string]string{}}
}

func (r *iqCustomQuestionsSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	v, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: v}, nil
}

func (r *iqCustomQuestionsSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	v, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}

func (r *iqCustomQuestionsSettingRepoStub) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *iqCustomQuestionsSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *iqCustomQuestionsSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *iqCustomQuestionsSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *iqCustomQuestionsSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestIQCustomQuestionsCRUD(t *testing.T) {
	ctx := context.Background()
	repo := newIQCustomQuestionsSettingRepoStub()
	svc := NewSettingService(repo, &config.Config{})

	initial, err := svc.ListIQCustomQuestions(ctx)
	require.NoError(t, err)
	require.Empty(t, initial)

	created, err := svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{
		Title:            "  河内塔推理  ",
		Prompt:           "写出 4 层河内塔的最少移动步骤。",
		OutputType:       "text",
		DeliveryContract: "只输出步骤列表。",
		ReferenceNote:    "最少 15 步",
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(created.ID, "cq_"))
	require.Equal(t, "河内塔推理", created.Title)
	require.Equal(t, IQCustomQuestionOutputText, created.OutputType)
	require.Equal(t, "只输出步骤列表。", created.DeliveryContract)
	require.Equal(t, "最少 15 步", created.ReferenceNote)

	htmlQ, err := svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{
		Title:      "物理小球弹跳",
		Prompt:     "生成一个带重力的小球弹跳 HTML 动画",
		OutputType: "html",
	})
	require.NoError(t, err)
	require.Equal(t, IQCustomQuestionOutputHTML, htmlQ.OutputType)

	list, err := svc.ListIQCustomQuestions(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, created.ID, list[0].ID)
	require.Equal(t, htmlQ.ID, list[1].ID)

	updated, err := svc.UpdateIQCustomQuestion(ctx, created.ID, IQCustomQuestionInput{
		Title:            "河内塔推理 V2",
		Prompt:           "写出 5 层河内塔的最少移动次数。",
		OutputType:       "text",
		DeliveryContract: "",
		ReferenceNote:    "31",
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, "河内塔推理 V2", updated.Title)
	require.Empty(t, updated.DeliveryContract)
	require.Equal(t, "31", updated.ReferenceNote)

	require.NoError(t, svc.DeleteIQCustomQuestion(ctx, created.ID))
	list, err = svc.ListIQCustomQuestions(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, htmlQ.ID, list[0].ID)

	err = svc.DeleteIQCustomQuestion(ctx, "cq_nonexistent")
	require.Error(t, err)
}

func TestIQCustomQuestionsValidation(t *testing.T) {
	ctx := context.Background()
	svc := NewSettingService(newIQCustomQuestionsSettingRepoStub(), &config.Config{})

	_, err := svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{Title: "   ", Prompt: "hello"})
	require.Error(t, err)

	_, err = svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{Title: "ok", Prompt: "   "})
	require.Error(t, err)

	_, err = svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{Title: "ok", Prompt: "hello", OutputType: "xml"})
	require.Error(t, err)

	_, err = svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{Title: strings.Repeat("a", IQCustomQuestionMaxTitle+1), Prompt: "hello"})
	require.Error(t, err)

	_, err = svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{Title: "ok", Prompt: "hello", DeliveryContract: strings.Repeat("x", IQCustomQuestionMaxContract+1)})
	require.Error(t, err)

	// Default output_type falls back to text.
	item, err := svc.CreateIQCustomQuestion(ctx, IQCustomQuestionInput{Title: "默认类型", Prompt: "test"})
	require.NoError(t, err)
	require.Equal(t, IQCustomQuestionOutputText, item.OutputType)
}

func TestCustomQuestionScheduledPromptAndValidation(t *testing.T) {
	now := time.Now()
	textCfg := &PelicanTestConfig{
		QuestionKind:    CustomQuestionKind,
		Prompt:          "请计算 17 * 23",
		ReasoningEffort: "medium",
		ParallelCount:   1,
	}
	require.Equal(t, "请计算 17 * 23", intelligenceTestPrompt(textCfg))
	require.Empty(t, intelligenceTestOutputError(textCfg, "391"))
	require.Equal(t, pelicanErrEmptyOutput, intelligenceTestOutputError(textCfg, "  "))

	textWithContract := &PelicanTestConfig{
		QuestionKind:     CustomQuestionKind,
		Prompt:           "请计算 17 * 23",
		DeliveryContract: "只输出数字。",
		ReasoningEffort:  "medium",
		ParallelCount:    1,
	}
	require.Equal(t, "请计算 17 * 23\n\n只输出数字。", intelligenceTestPrompt(textWithContract))

	htmlCfg := &PelicanTestConfig{
		QuestionKind:    CustomHTMLQuestionKind,
		Prompt:          "做一个单文件 HTML 时钟",
		ReasoningEffort: "high",
		ParallelCount:   1,
	}
	require.Equal(t, "做一个单文件 HTML 时钟", intelligenceTestPrompt(htmlCfg))
	require.NotEmpty(t, intelligenceTestOutputError(htmlCfg, "plain text only"))
	require.Empty(t, intelligenceTestOutputError(htmlCfg, "<!doctype html><html><body>clock</body></html>"))

	for _, kind := range []string{CustomQuestionKind, CustomHTMLQuestionKind} {
		plan := &ScheduledTestPlan{
			ModelID:        "gpt-6-astra",
			CronExpression: "*/30 * * * *",
			MaxResults:     50,
			PelicanConfig: &PelicanTestConfig{
				QuestionKind:     kind,
				Prompt:           "custom prompt",
				DeliveryContract: "custom contract",
				ReasoningEffort:  "medium",
				ParallelCount:    2,
			},
		}
		_, err := nextPlanRun(plan, now)
		require.NoError(t, err, "kind=%s should be accepted", kind)
	}
}
