package timezone

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSystemTimezoneValidation(t *testing.T) {
	for _, value := range []string{"", "UTC", "Asia/Taipei", "America/New_York", "Etc/GMT+8"} {
		require.NoError(t, ValidateSetting(value), value)
	}
	for _, value := range []string{"Local", "CST", "../Asia/Taipei", "/etc/passwd", "Invalid/Zone", "Asia/Taipei'", " Asia/Taipei ", "Asia\\Taipei"} {
		require.Error(t, ValidateSetting(value), value)
	}
}

func TestSystemTimezoneCalendarBoundaries(t *testing.T) {
	old := Name()
	if old == "Local" {
		old = "UTC"
	}
	t.Cleanup(func() { require.NoError(t, Init(old)) })
	require.NoError(t, Init("Asia/Taipei"))
	instant := time.Date(2026, 10, 18, 16, 30, 0, 0, time.UTC)
	require.Equal(t, "2026-10-19 00:00 +0800", StartOfDay(instant).Format("2006-01-02 15:04 -0700"))
	require.Equal(t, "2026-10-19 00:00 +0800", StartOfWeek(instant).Format("2006-01-02 15:04 -0700"))
	require.NoError(t, Init("America/New_York"))
	before := StartOfDay(time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC))
	require.Equal(t, 23*time.Hour, before.AddDate(0, 0, 1).Sub(before))
}
