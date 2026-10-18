package service

import (
	"context"
	"regexp"
)

type accountViewerKey struct{}

// WithAccountViewerScope grants the observer's group scope and marks the
// request so response DTOs are redacted for the read-only account viewer.
func WithAccountViewerScope(ctx context.Context, groupIDs []int64) context.Context {
	return context.WithValue(WithObserverScope(ctx, groupIDs), accountViewerKey{}, true)
}

func IsAccountViewerScope(ctx context.Context) bool {
	v, _ := ctx.Value(accountViewerKey{}).(bool)
	return v
}

// Account/group labels sometimes contain email addresses. Redact at the server,
// including bare addresses and non-ASCII local parts, before serialization.
var accountViewerEmail = regexp.MustCompile(`[^\s<>"',;:()\[\]{}]+@[^\s<>"',;:()\[\]{}]+`)

func RedactAccountViewerText(value string) string {
	return accountViewerEmail.ReplaceAllString(value, "***")
}

// RedactAccountViewerValue returns a copy of a JSON-like value with every email
// in string leaves replaced. Inputs are never mutated: they may be cache-backed.
func RedactAccountViewerValue(value any) any {
	switch v := value.(type) {
	case string:
		return RedactAccountViewerText(v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = RedactAccountViewerValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = RedactAccountViewerValue(item)
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = RedactAccountViewerText(item)
		}
		return out
	default:
		return value
	}
}
