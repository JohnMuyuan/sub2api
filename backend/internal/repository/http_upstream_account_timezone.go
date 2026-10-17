package repository

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountTimezoneLookup struct {
	cache    service.SchedulerCache
	accounts service.AccountRepository
	resolve  func(context.Context, string) string
}

// Keep the concrete transport (and its optional routing interfaces) intact.
func NewHTTPUpstreamWithAccountTimezone(cfg *config.Config, cache service.SchedulerCache, accounts service.AccountRepository, prober service.ProxyExitInfoProber) service.HTTPUpstream {
	return newHTTPUpstream(cfg, &accountTimezoneLookup{cache, accounts, service.NewAccountRequestTimezoneResolver(prober)})
}

func (s *httpUpstreamService) bindAccountTimezone(req *http.Request, accountID int64) *http.Request {
	if s.accountTimezones == nil || req == nil || accountID <= 0 || req.Method != http.MethodPost || !strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/json") {
		return req
	}
	lookup := s.accountTimezones
	var account *service.Account
	if lookup.cache != nil {
		account, _ = lookup.cache.GetAccount(req.Context(), accountID)
	}
	if account == nil && lookup.accounts != nil {
		var err error
		account, err = lookup.accounts.GetByID(req.Context(), accountID)
		if err != nil {
			slog.Debug("request timezone account lookup failed", "account_id", accountID)
			return req
		}
	}
	return service.WithAccountRequestTimezone(req, account, lookup.resolve)
}
