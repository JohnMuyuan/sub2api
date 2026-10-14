package service

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"net/http"
)

func (s *GatewayService) regionalDoWithTLS(req *http.Request, proxy string, account *Account, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if guard, ok := s.rpmCache.(RegionalAccountGuard); ok {
		if err := guard.RegionalAccountAllowed(req.Context(), account.ID); err != nil {
			return nil, err
		}
		if account.SupportsRPMLimit() && account.GetBaseRPM() > 0 {
			strict, ok := s.rpmCache.(StrictRPMCache)
			if !ok {
				return nil, errors.New("regional strict RPM unavailable")
			}
			acquired, _, _, err := strict.TryAcquireRPM(req.Context(), account.RPMAccountID(), account.GetBaseRPM())
			if err != nil {
				return nil, err
			}
			if !acquired {
				return nil, ErrOpenAIRPMExhausted
			}
		}
	}
	response, err := s.httpUpstream.DoWithTLS(req, proxy, account.ID, account.Concurrency, profile)
	return response, err
}
