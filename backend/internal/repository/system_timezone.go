package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/lib/pq"
)

func openConfiguredDatabase(cfg *config.Config) (*entsql.Driver, error) {
	dsn := cfg.Database.DSNWithTimezone(cfg.Timezone)

	// 使用 Ent 的 SQL 驱动打开 PostgreSQL 连接。
	// dialect.Postgres 指定使用 PostgreSQL 方言进行 SQL 生成。
	var drv *entsql.Driver
	if cfg.Server.EnableServerTiming {
		connector, err := pq.NewConnector(dsn)
		if err != nil {
			return nil, err
		}
		drv = entsql.OpenDB(dialect.Postgres, sql.OpenDB(newServerTimingConnector(connector)))
	} else {
		var err error
		drv, err = entsql.Open(dialect.Postgres, dsn)
		if err != nil {
			return nil, err
		}
	}
	applyDBPoolSettings(drv.DB(), cfg)

	return drv, nil
}

func loadStoredSystemTimezone(ctx context.Context, db *sql.DB, cfg *config.Config) (bool, error) {
	var raw string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = $1", timezone.SettingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load system timezone: %w", err)
	}
	value := strings.TrimSpace(raw)
	if err := timezone.ValidateSetting(value); err != nil {
		return false, err
	}
	if value == "" || value == cfg.Timezone {
		return false, nil
	}
	cfg.Timezone = value
	return true, nil
}
