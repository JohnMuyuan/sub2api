package timezone

import (
	"fmt"
	"strings"
	"time"
)

const SettingKey = "system_timezone"

// Empty means follow the startup TZ/TIMEZONE/config setting.
func ValidateSetting(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > 100 || (value != "UTC" && !strings.Contains(value, "/")) || strings.HasPrefix(value, "/") || strings.Contains(value, "..") || strings.Contains(value, "\\") {
		return fmt.Errorf("timezone must be an IANA name such as Asia/Taipei or UTC")
	}
	if _, err := time.LoadLocation(value); err != nil {
		return fmt.Errorf("invalid IANA timezone %q", value)
	}
	return nil
}
