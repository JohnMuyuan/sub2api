package repository

import (
	"errors"
	"testing"

	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestSystemTimezoneStartupOverride(t *testing.T) {
	for _, tt := range []struct {
		name, stored   string
		readErr        error
		want           string
		changed, fails bool
	}{
		{name: "saved", stored: "Asia/Taipei", want: "Asia/Taipei", changed: true},
		{name: "trimmed", stored: " Asia/Taipei ", want: "Asia/Taipei", changed: true},
		{name: "follow startup", want: "UTC"},
		{name: "not configured", readErr: sql.ErrNoRows, want: "UTC"},
		{name: "already effective", stored: "UTC", want: "UTC"},
		{name: "invalid fails startup", stored: "Invalid/Zone", want: "UTC", fails: true},
		{name: "database error", readErr: errors.New("offline"), want: "UTC", fails: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			query := mock.ExpectQuery("SELECT value FROM settings WHERE key").WithArgs(timezone.SettingKey)
			if tt.readErr != nil {
				query.WillReturnError(tt.readErr)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(tt.stored))
			}
			cfg := &config.Config{Timezone: "UTC", StartupTimezone: "UTC"}
			changed, err := loadStoredSystemTimezone(t.Context(), db, cfg)
			if tt.fails {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.changed, changed)
			require.Equal(t, tt.want, cfg.Timezone)
			require.Equal(t, "UTC", cfg.StartupTimezone)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
