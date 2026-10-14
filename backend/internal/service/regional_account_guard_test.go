//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

type regionalReportTestGuard struct {
	*openAIRPMTestCache
	calls   int
	until   time.Time
	invalid bool
	err     error
}

func (g *regionalReportTestGuard) RegionalAccountReportingLocal() bool { return true }
func (g *regionalReportTestGuard) RegionalAccountRestriction(_ context.Context, _ int64, until time.Time, invalid bool) error {
	g.calls++
	g.until = until
	g.invalid = invalid
	return g.err
}
func TestRegionalAccountReportingSkipsRequestPolicyAndNeverUsesPrimary(t *testing.T) {
	ctx := context.Background()
	guard := &regionalReportTestGuard{openAIRPMTestCache: &openAIRPMTestCache{counts: map[int64]int{}}}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Status: StatusActive}
	// A nil RateLimitService would panic if this region entered primary mutation.
	gateway := &GatewayService{rpmCache: guard}
	require.False(t, gateway.regionalHandleUpstreamError(ctx, account, http.StatusForbidden, nil, []byte("This request was blocked by safeguard policy")))
	require.Zero(t, guard.calls)
	before := time.Now()
	require.True(t, gateway.regionalHandleUpstreamError(ctx, account, http.StatusTooManyRequests, http.Header{"Retry-After": []string{"3600"}}, nil))
	require.Equal(t, 1, guard.calls)
	require.False(t, guard.invalid)
	require.GreaterOrEqual(t, guard.until.Sub(before), time.Hour)
	openai := &OpenAIGatewayService{rpmCache: guard}
	account.Platform = PlatformOpenAI
	require.True(t, openai.handleOpenAIAccountUpstreamError(ctx, account, http.StatusUnauthorized, nil, []byte("expired")))
	require.Equal(t, 2, guard.calls)
	require.True(t, guard.invalid)
	guard.err = errors.New("local ledger unavailable")
	require.True(t, openai.handleOpenAIAccountUpstreamError(ctx, account, http.StatusTooManyRequests, nil, nil))
	require.Equal(t, 3, guard.calls)
}
