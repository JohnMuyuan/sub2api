//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Uses the existing transactional PostgreSQL harness. Verify real SQL calendar
// calculations, including a stale reset_at left by the previous site clock.
func (s *AccountRepoSuite) TestSystemTimezoneFixedQuotaInheritsConnectionClock() {
	_, err := s.repo.sql.ExecContext(s.ctx, "SET LOCAL TIME ZONE 'Asia/Taipei'")
	s.Require().NoError(err)
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "site-timezone-quota", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_daily_limit": 100.0, "quota_weekly_limit": 100.0, "quota_daily_used": 9.0, "quota_weekly_used": 9.0,
			"quota_daily_reset_mode": "fixed", "quota_weekly_reset_mode": "fixed",
			"quota_daily_start": "2000-01-01T00:00:00Z", "quota_weekly_start": "2000-01-01T00:00:00Z",
			"quota_daily_reset_at": "2099-01-01T00:00:00Z", "quota_weekly_reset_at": "2099-01-01T00:00:00Z",
		},
	})
	s.Require().NoError(s.repo.IncrementQuotaUsed(s.ctx, account.ID, 2))
	updated, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(2.0, updated.GetQuotaDailyUsed())
	s.Require().Equal(2.0, updated.GetQuotaWeeklyUsed())
	zone, err := time.LoadLocation("Asia/Taipei")
	s.Require().NoError(err)
	for _, key := range []string{"quota_daily_reset_at", "quota_weekly_reset_at"} {
		reset, err := time.Parse(time.RFC3339, updated.Extra[key].(string))
		s.Require().NoError(err)
		s.Require().Equal(0, reset.In(zone).Hour())
		if key == "quota_weekly_reset_at" {
			s.Require().Equal(time.Monday, reset.In(zone).Weekday())
		}
	}
}
