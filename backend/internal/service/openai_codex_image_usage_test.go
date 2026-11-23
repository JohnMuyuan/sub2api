//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexImageRateLimitTestRepo struct {
	modelNotFoundAccountRepoStub
	rateLimitCalls    []time.Time
	schedulableCalls  []bool
	extraUpdates      []map[string]any
}

func (r *codexImageRateLimitTestRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls = append(r.rateLimitCalls, resetAt)
	return nil
}

func (r *codexImageRateLimitTestRepo) SetSchedulable(_ context.Context, _ int64, schedulable bool) error {
	r.schedulableCalls = append(r.schedulableCalls, schedulable)
	return nil
}

func (r *codexImageRateLimitTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	copied := make(map[string]any, len(updates))
	for k, v := range updates {
		copied[k] = v
	}
	r.extraUpdates = append(r.extraUpdates, copied)
	return nil
}

type codexImageUsageModelStatsRepo struct {
	usageBatchLogRepoStub
	modelStats []usagestats.ModelStat
}

func (r *codexImageUsageModelStatsRepo) GetModelStatsWithFilters(
	context.Context, time.Time, time.Time, int64, int64, int64, int64, *int16, *bool, *int8,
) ([]usagestats.ModelStat, error) {
	return r.modelStats, nil
}

func TestOpenAIGatewayServiceForwardImages_DirectCodex429IsolatesImageScopeAndSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &codexImageRateLimitTestRepo{}
	body := []byte(`{"model":"gpt-image-2.5-sunburst","prompt":"draw a sunset"}`)
	errorBody := `{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached"}}`

	headers := http.Header{
		"X-Request-Id":                        []string{"req_codex_img_429"},
		"X-Codex-Primary-Used-Percent":        []string{"100"},
		"X-Codex-Primary-Reset-After-Seconds": []string{"70432"},
		"X-Codex-Primary-Window-Minutes":      []string{"1440"},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &OpenAIGatewayService{
		accountRepo:      repo,
		rateLimitService: &RateLimitService{accountRepo: repo},
		httpUpstream: &httpUpstreamRecorder{
			resp: &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader(errorBody)),
			},
		},
	}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	account := &Account{
		ID:       81,
		Name:     "Plus",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
		Extra: map[string]any{
			"codex_5h_used_percent":             0.0,
			"codex_7d_used_percent":             16.0,
			OpenAIDisableSchedulingOn429ExtraKey: true,
		},
	}

	before := time.Now()
	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.False(t, failoverErr.RetryableOnSameAccount)

	// 仅对 image_generation 模型维度限流，不得整号限流或永久停调
	require.Len(t, repo.modelRateLimitCalls, 1)
	call := repo.modelRateLimitCalls[0]
	require.Equal(t, int64(81), call.accountID)
	require.Equal(t, openAIImageGenerationRateLimitKey, call.scope)
	require.Equal(t, openAIImageRateLimitReason, call.reason)
	require.WithinDuration(t, before.Add(70432*time.Second), call.resetAt, 3*time.Second)

	require.Empty(t, repo.rateLimitCalls, "direct codex image 429 must not rate-limit the whole account")
	require.Empty(t, repo.schedulableCalls, "direct codex image 429 must not permanently disable scheduling")
	_, wholeAccountBlocked := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.False(t, wholeAccountBlocked, "direct codex image 429 must not runtime-block the whole account")

	// 持久化到独立的 codex_image_* 字段，不得覆盖全局 codex_5h/7d 快照
	require.Len(t, repo.extraUpdates, 1)
	persisted := repo.extraUpdates[0]
	require.Equal(t, 100.0, persisted[codexImageUsedPercentKey])
	require.Equal(t, 1440, persisted[codexImageWindowMinutesKey])
	require.Equal(t, 70432, persisted[codexImageResetAfterSecondsKey])
	require.NotEmpty(t, persisted[codexImageResetAtKey])
	require.NotContains(t, persisted, "codex_7d_used_percent")
	require.NotContains(t, persisted, "codex_5h_used_percent")
	require.Equal(t, 16.0, account.Extra["codex_7d_used_percent"])
	require.Equal(t, 0.0, account.Extra["codex_5h_used_percent"])
}

func TestBuildCodexImageUsageProgressFromExtra(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	t.Run("active passive image snapshot", func(t *testing.T) {
		resetAt := now.Add(18 * time.Hour)
		progress := buildCodexImageUsageProgressFromExtra(map[string]any{
			codexImageUsedPercentKey:   62.5,
			codexImageWindowMinutesKey: 1440,
			codexImageResetAtKey:       resetAt.Format(time.RFC3339),
		}, now)
		require.NotNil(t, progress)
		require.Equal(t, 62.5, progress.Utilization)
		require.Equal(t, 1440, progress.WindowMinutes)
		require.NotNil(t, progress.ResetsAt)
		require.True(t, progress.ResetsAt.Equal(resetAt))
		require.Equal(t, int((18 * time.Hour).Seconds()), progress.RemainingSeconds)
	})

	t.Run("expired image window resets utilization to zero", func(t *testing.T) {
		resetAt := now.Add(-5 * time.Minute)
		progress := buildCodexImageUsageProgressFromExtra(map[string]any{
			codexImageUsedPercentKey: 100.0,
			codexImageResetAtKey:     resetAt.Format(time.RFC3339),
		}, now)
		require.NotNil(t, progress)
		require.Equal(t, 0.0, progress.Utilization)
		require.Equal(t, 0, progress.RemainingSeconds)
	})

	t.Run("fallback to active image model rate limit when no passive header exists", func(t *testing.T) {
		resetAt := now.Add(6 * time.Hour)
		progress := buildCodexImageUsageProgressFromExtra(map[string]any{
			modelRateLimitsKey: map[string]any{
				openAIImageGenerationRateLimitKey: map[string]any{
					"reason":              openAIImageRateLimitReason,
					"rate_limit_reset_at": resetAt.Format(time.RFC3339),
				},
			},
		}, now)
		require.NotNil(t, progress)
		require.Equal(t, 100.0, progress.Utilization)
		require.NotNil(t, progress.ResetsAt)
		require.True(t, progress.ResetsAt.Equal(resetAt))
	})
}

func TestGetOpenAIUsage_NormalOAuthPrefersWhamUsageAndIncludesCodexImageWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	imageResetAt := time.Now().Add(12 * time.Hour).UTC().Truncate(time.Second)
	account := &Account{
		ID:       81,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"chatgpt_account_id": "org-plus",
		},
		Extra: map[string]any{
			codexImageUsedPercentKey:   80.0,
			codexImageWindowMinutesKey: 1440,
			codexImageResetAtKey:       imageResetAt.Format(time.RFC3339),
		},
	}

	repo := &sparkShadowUsageTestRepo{
		accounts:      map[int64]*Account{81: account},
		updateExtraCh: make(chan map[string]any, 1),
	}
	tokens := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "fake-oauth-token",
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/backend-api/wham/usage" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"account_id": "org-plus",
				"rate_limit": map[string]any{
					"allowed":       true,
					"limit_reached": false,
					"primary_window": map[string]any{
						"used_percent":         0.0,
						"limit_window_seconds": 18000,
						"reset_after_seconds":  15000,
					},
					"secondary_window": map[string]any{
						"used_percent":         16.0,
						"limit_window_seconds": 604800,
						"reset_after_seconds":  160000,
					},
				},
			})
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	usageLogRepo := &codexImageUsageModelStatsRepo{
		modelStats: []usagestats.ModelStat{
			{Model: "gpt-image-2.5-sunburst", Requests: 101, TotalTokens: 612900, Cost: 12.0, ActualCost: 1.2, AccountCost: 1.2},
			{Model: "gpt-image-2.6-sunburst", Requests: 32, TotalTokens: 146551, Cost: 4.0, ActualCost: 0.4, AccountCost: 0.4},
			{Model: "gpt-6-luna", Requests: 19, TotalTokens: 146984, Cost: 2.0, ActualCost: 0.2, AccountCost: 0.2},
		},
	}

	quotaSvc := NewOpenAIQuotaService(repo, nil, NewOpenAITokenProvider(repo, tokens, nil), newQuotaRedirectingFactory(srv), nil)
	svc := &AccountUsageService{
		accountRepo:        repo,
		usageLogRepo:       usageLogRepo,
		openAIQuotaService: quotaSvc,
		cache:              NewUsageCache(),
	}

	usage, err := svc.getOpenAIUsage(ctx, account, true)
	require.NoError(t, err)
	require.Equal(t, "active", usage.Source)
	require.Empty(t, usage.ErrorCode)
	require.NotNil(t, usage.FiveHour)
	require.InDelta(t, 0.0, usage.FiveHour.Utilization, 0.01)
	require.NotNil(t, usage.SevenDay)
	require.InDelta(t, 16.0, usage.SevenDay.Utilization, 0.01)

	require.NotNil(t, usage.CodexImage)
	require.InDelta(t, 80.0, usage.CodexImage.Utilization, 0.01)
	require.Equal(t, 1440, usage.CodexImage.WindowMinutes)
	require.NotNil(t, usage.CodexImage.WindowStats)
	// 只统计生图模型（101 + 32 = 133），排除文本模型 gpt-6-luna
	require.Equal(t, int64(133), usage.CodexImage.WindowStats.Requests)
	require.Equal(t, int64(759451), usage.CodexImage.WindowStats.Tokens)
}
