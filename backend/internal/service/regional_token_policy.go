package service

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"time"
)

// localRegionalAccessToken never refreshes or reads a remote repository/cache.
// The scheduler must supply a fresh authenticated account projection.
func localRegionalAccessToken(ctx context.Context, snapshot *SchedulerSnapshotService, account *Account) (string, error) {
	if snapshot == nil || snapshot.localSnapshotReader == nil || account == nil {
		return "", ErrLocalSchedulerSnapshotUnavailable
	}
	fresh, err := snapshot.GetAccount(ctx, account.ID)
	if err != nil || fresh == nil {
		return "", ErrLocalSchedulerSnapshotUnavailable
	}
	expiry := fresh.GetCredentialAsTime("expires_at")
	if fresh.Platform != account.Platform || fresh.Type != account.Type || !fresh.IsActive() || !fresh.Schedulable || expiry == nil || time.Until(*expiry) <= 30*time.Second {
		return "", errors.New("regional OAuth credential unavailable or near expiry")
	}
	token := strings.TrimSpace(fresh.GetCredential("access_token"))
	if token == "" {
		return "", errors.New("regional OAuth access token missing")
	}
	return token, nil
}

// SQL session ownership survives Redis outages and has no lock TTL. A refresh
// holds one dedicated connection until persistence finishes; another Pod must
// wait for that connection to release the PostgreSQL advisory lock.
func (api *OAuthRefreshAPI) lockRegionalRefresh(ctx context.Context, id int64) (func(), error) {
	if api.regionalDB == nil {
		return nil, errors.New("regional OAuth refresh authority unavailable")
	}
	conn, err := api.regionalDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var locked bool
	err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1,$2)", int32(283), int32(id^(id>>32))).Scan(&locked)
	if err != nil || !locked {
		_ = conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	return func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var unlocked bool
		err := conn.QueryRowContext(cleanup, "SELECT pg_advisory_unlock($1,$2)", int32(283), int32(id^(id>>32))).Scan(&unlocked)
		if err != nil || !unlocked {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
	}, nil
}
