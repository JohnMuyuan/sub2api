package service

import (
	"context"
	"regexp"
	"time"
)

// This DTO is an allowlist, independent of admin Account/Credentials/Extra DTOs.
// Do not add raw upstream responses, error messages, notes, URLs or identities.
type AccountViewerAccount struct {
	ID                     int64                   `json:"id"`
	Name                   string                  `json:"name"`
	Platform               string                  `json:"platform"`
	Type                   string                  `json:"type"`
	Status                 string                  `json:"status"`
	Schedulable            bool                    `json:"schedulable"`
	RateLimitResetAt       *time.Time              `json:"rate_limit_reset_at,omitempty"`
	OverloadUntil          *time.Time              `json:"overload_until,omitempty"`
	TempUnschedulableUntil *time.Time              `json:"temp_unschedulable_until,omitempty"`
	Groups                 []AccountViewerGroup    `json:"groups"`
	UsageWindows           []AccountOpsUsageWindow `json:"usage_windows"`
}

type AccountViewerGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Account/group labels sometimes contain email addresses. Redact at the server,
// including bare addresses and non-ASCII local parts, before serialization.
var accountViewerEmail = regexp.MustCompile(`[^\s<>]+@[^\s<>]+`)

func accountViewerLabel(value string) string {
	return accountViewerEmail.ReplaceAllString(value, "***")
}

func AccountForViewer(ctx context.Context, a *Account) AccountViewerAccount {
	out := AccountViewerAccount{
		ID: a.ID, Name: accountViewerLabel(a.Name), Platform: accountViewerLabel(a.Platform), Type: accountViewerLabel(a.Type),
		Status: accountViewerLabel(a.Status), Schedulable: a.Schedulable,
		RateLimitResetAt: a.RateLimitResetAt, OverloadUntil: a.OverloadUntil, TempUnschedulableUntil: a.TempUnschedulableUntil,
		Groups: []AccountViewerGroup{}, UsageWindows: opsExtraUsageWindows(a, time.Now()),
	}
	for _, group := range a.Groups {
		if group != nil && ObserverCanManageGroup(ctx, group.ID) {
			out.Groups = append(out.Groups, AccountViewerGroup{ID: group.ID, Name: accountViewerLabel(group.Name)})
		}
	}
	return out
}
