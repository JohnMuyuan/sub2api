package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/regional"
	"github.com/Wei-Shaw/sub2api/internal/regional/ledger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

type RegionalFundsTarget struct {
	UserID, APIKeyID, AccountID int64
	Amount                      regional.Amount
}
type RegionalFundsRuntimeConfig struct {
	Mode, Region  string
	Prices        []ledger.Price
	Targets       []RegionalFundsTarget
	MaximumOutbox int
}

func readRegionalFundsConfig(path string) (RegionalFundsRuntimeConfig, error) {
	var c RegionalFundsRuntimeConfig
	f, err := os.Open(path)
	if err != nil {
		return c, errors.New("funds config unavailable")
	}
	defer func() { _ = f.Close() }()
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, errors.New("invalid funds config")
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("trailing funds config")
	}
	if c.Mode != "primary" && c.Mode != "regional" || len(c.Prices) == 0 || len(c.Targets) == 0 || c.MaximumOutbox < 1 || c.MaximumOutbox > 10000 {
		return c, errors.New("funds mode, prices, targets and outbox limit required")
	}
	seen := map[string]bool{}
	for _, p := range c.Prices {
		if err = p.Validate(); err != nil {
			return c, err
		}
		if seen[p.Model] {
			return c, errors.New("duplicate funds model")
		}
		seen[p.Model] = true
	}
	for _, target := range c.Targets {
		if target.UserID <= 0 || target.APIKeyID <= 0 || target.AccountID <= 0 || target.Amount <= 0 {
			return c, errors.New("invalid funded target")
		}
		if _, err = ledger.USD(target.Amount); err != nil {
			return c, err
		}
	}
	return c, nil
}

// ProvideRegionalMetering attaches finance to the already validated local/primary
// concurrency authority. Financial issuance remains fenced until full request
// coverage and legacy-writer cutover can be proved (see readiness below).
func ProvideRegionalMetering(db *sql.DB, cfg *config.Config, cache service.ConcurrencyCache) (*service.RegionalMetering, error) {
	if cfg == nil || cfg.Runtime.RegionalFundsFile == "" {
		return nil, nil
	}
	c, err := readRegionalFundsConfig(cfg.Runtime.RegionalFundsFile)
	if err != nil {
		return nil, err
	}
	capacity, ok := cache.(*regionalConcurrencyCache)
	if !ok || capacity.runtimeConfig == nil || capacity.runtimeConfig.Mode != c.Mode || capacity.runtimeConfig.Region != c.Region {
		return nil, errors.New("funds require matching guarded concurrency runtime")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	prices := map[string]ledger.Price{}
	for _, price := range c.Prices {
		if err = ledger.InstallPrice(ctx, db, price); err != nil {
			return nil, err
		}
		prices[price.Model] = price
	}
	var requests ledger.RequestLedger
	if c.Mode == "primary" {
		_, holds, _ := NewRegionalFundsAuthorities(db, capacity.primary.GrantGuard)
		requests = holds
	} else {
		requests = &ledger.Local{DB: db, Region: c.Region}
	}
	meter := &service.RegionalMetering{Ledger: requests}
	meter.Admit = func(ctx context.Context, key *service.APIKey, account *service.Account, body []byte) (ledger.Reservation, ledger.Price, error) {
		r := ledger.Reservation{}
		if err = meter.Check(ctx); err != nil {
			return r, ledger.Price{}, err
		}
		if key == nil || account == nil {
			return r, ledger.Price{}, ledger.ErrConflict
		}
		var request struct{ Model string }
		if err := json.Unmarshal(body, &request); err != nil {
			return r, ledger.Price{}, err
		}
		price, ok := prices[request.Model]
		if !ok {
			return r, ledger.Price{}, errors.New("immutable price unavailable")
		}
		allowed := false
		for _, target := range c.Targets {
			if target.UserID == key.UserID && target.APIKeyID == key.ID && target.AccountID == account.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return r, price, ledger.ErrConflict
		}
		r = ledger.Reservation{RequestID: uuid.NewString(), RegionID: c.Region, UserID: key.UserID, APIKeyID: key.ID, AccountID: account.ID, AdmittedAt: time.Now().UTC().Truncate(time.Microsecond)}
		if c.Mode == "regional" {
			var raw []byte
			err := db.QueryRowContext(ctx, `SELECT lease FROM regional_local_leases WHERE region_id=$1 AND user_id=$2 AND state='open' AND (lease->>'api_key_id')::bigint=$3 AND (lease->>'account_id')::bigint=$4 AND (lease->>'expires_at')::timestamptz>clock_timestamp() AND granted-spent-reserved>0 ORDER BY lease_id LIMIT 1`, c.Region, key.UserID, key.ID, account.ID).Scan(&raw)
			if err != nil {
				return r, price, err
			}
			var lease regional.Lease
			if err = json.Unmarshal(raw, &lease); err != nil {
				return r, price, err
			}
			r.LeaseID = lease.LeaseID
		}
		return r, price, nil
	}
	// The observation is not a switch. There is intentionally no config boolean
	// that can substitute for proving every billable path and legacy writer uses
	// this authority. The parent gate sees the remaining compatibility blocker.
	meter.Readiness = func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			return err
		}
		var unresolved int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM regional_usage_outbox WHERE NOT acknowledged").Scan(&unresolved); err != nil {
			return err
		}
		if unresolved >= c.MaximumOutbox {
			return errors.New("regional usage outbox at admission limit")
		}
		return errors.New("funds rollout incomplete: legacy monetary writers and all model protocols require unified admission")
	}
	var once sync.Once
	stopped := make(chan struct{})
	meter.Run = func(ctx context.Context) {
		select {
		case <-ctx.Done():
		case <-stopped:
		}
	}
	meter.Stop = func() { once.Do(func() { close(stopped) }) }
	return meter, nil
}
