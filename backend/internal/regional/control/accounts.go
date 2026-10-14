package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/regional/accountstate"
	"io"
	"net/http"
)

// AccountHandler shares the pinned certificate scope with slots but exposes
// only background state and request-unit operations.
type AccountHandler struct {
	Scope  Scope
	RPM    *accountstate.RPMPrimary
	States *accountstate.States
}

func (h *AccountHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.Scope.authenticate(r) {
		w.WriteHeader(403)
		return
	}
	if r.Method != "POST" || r.URL.RawQuery != "" {
		w.WriteHeader(405)
		return
	}
	var err error
	switch r.URL.Path {
	case Prefix + "/accounts/rpm":
		var g accountstate.RPMGrant
		if decode(w, r, &g) != nil {
			w.WriteHeader(400)
			return
		}
		if g.Region != h.Scope.Region || !contains(h.Scope.AccountIDs, g.AccountID) || g.Units > h.Scope.MaximumRPM {
			w.WriteHeader(403)
			return
		}
		err = h.RPM.Issue(r.Context(), g)
	case Prefix + "/accounts/report":
		var e accountstate.Event
		if decode(w, r, &e) != nil {
			w.WriteHeader(400)
			return
		}
		if e.Region != h.Scope.Region || !contains(h.Scope.AccountIDs, e.AccountID) {
			w.WriteHeader(403)
			return
		}
		err = h.States.Apply(r.Context(), e)
	case Prefix + "/accounts/state":
		var req struct{ AccountID int64 }
		if decode(w, r, &req) != nil {
			w.WriteHeader(400)
			return
		}
		if !contains(h.Scope.AccountIDs, req.AccountID) {
			w.WriteHeader(403)
			return
		}
		var state accountstate.State
		state, err = h.States.Snapshot(r.Context(), req.AccountID)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(state)
			return
		}
	default:
		w.WriteHeader(404)
		return
	}
	if err != nil {
		if errors.Is(err, accountstate.ErrConflict) {
			w.WriteHeader(409)
		} else if errors.Is(err, accountstate.ErrUnavailable) {
			w.WriteHeader(429)
		} else {
			w.WriteHeader(503)
		}
		return
	}
	w.WriteHeader(204)
}
func (c *Client) IssueRPM(ctx context.Context, g accountstate.RPMGrant) error {
	return c.post(ctx, "/accounts/rpm", g)
}
func (c *Client) ReportState(ctx context.Context, e accountstate.Event) error {
	return c.post(ctx, "/accounts/report", e)
}
func (c *Client) GetState(ctx context.Context, id int64) (accountstate.State, error) {
	var out accountstate.State
	raw, err := json.Marshal(struct{ AccountID int64 }{id})
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+Prefix+"/accounts/state", bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return out, errors.New("regional account transport unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return out, errors.New("regional account snapshot unavailable")
	}
	raw, err = io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return out, errors.New("invalid regional account snapshot")
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}
