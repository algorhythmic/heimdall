package continuity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"heimdall/internal/authz"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
)

const MaxReadBytes = 512 << 10

// ScopedContext operates on the same store snapshot used to authenticate. It
// refuses missing inherited visibility rather than dropping mandatory context.
func ScopedContext(ctx context.Context, st model.State, g model.Grant, target string, budget int) (Bundle, error) {
	if !g.Contains(st, target) {
		return Bundle{}, authz.ErrDenied
	}
	targets, err := lineage(st, target)
	if err != nil {
		return Bundle{}, err
	}
	for _, t := range targets {
		if !g.Contains(st, t) {
			return Bundle{}, fmt.Errorf("mandatory ancestor context outside grant: %w", authz.ErrDenied)
		}
	}
	for _, r := range resources(st, targets) {
		if !model.Contains(g.ResourceIDs, r.ID) {
			return Bundle{}, fmt.Errorf("resource observation outside grant: %w", authz.ErrDenied)
		}
	}
	if budget < 1 || budget > MaxReadBytes/4 {
		return Bundle{}, fmt.Errorf("budget must be 1..%d", MaxReadBytes/4)
	}
	return buildContext(ctx, st, target, budget)
}

type Page struct {
	Version    int               `json:"version"`
	Target     string            `json:"target"`
	Kind       string            `json:"kind"`
	Items      []json.RawMessage `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}
type historyCursor struct {
	Grant  string `json:"grant"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Event  int64  `json:"event"`
	After  string `json:"after"`
}

// Cursors are untrusted positions, not authority. Every page is authorized anew.
// Any intervening event invalidates the cursor rather than mixing snapshots.
func History(st model.State, g model.Grant, target, kind, cursor string, limit int) (Page, error) {
	out := Page{Version: 1, Target: target, Kind: kind, Items: []json.RawMessage{}}
	if !g.Contains(st, target) {
		return out, authz.ErrDenied
	}
	if limit < 1 || limit > 50 {
		return out, fmt.Errorf("limit must be 1..50")
	}
	after := ""
	if cursor != "" {
		if len(cursor) > 2048 {
			return out, fmt.Errorf("invalid cursor")
		}
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return out, fmt.Errorf("invalid cursor")
		}
		var c historyCursor
		if err = model.StrictJSON(raw, &c); err != nil || c.Grant != g.ID || c.Target != target || c.Kind != kind || !model.OpaqueID.MatchString(c.After) {
			return out, fmt.Errorf("cursor scope mismatch")
		}
		if c.Event != st.LastEventID {
			return out, fmt.Errorf("history changed; restart pagination: %w", store.ErrConflict)
		}
		after = c.After
	}
	records := map[string]any{}
	switch kind {
	case "contract":
		for id, v := range st.Contracts {
			if v.Target == target {
				records[id] = v
			}
		}
	case "decision":
		for id, v := range st.Decisions {
			if v.Target == target {
				records[id] = v
			}
		}
	case "resource":
		for id, v := range st.Resources {
			if v.Target == target {
				records[id] = v
			}
		}
	case "checkpoint":
		for id, v := range st.Checkpoints {
			if v.Target == target {
				records[id] = v
			}
		}
	default:
		return out, fmt.Errorf("unknown history kind")
	}
	if after != "" {
		if _, ok := records[after]; !ok {
			return out, fmt.Errorf("invalid cursor position")
		}
	}
	ids := []string{}
	for id := range records {
		if id > after {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	bytes := 4096 // reserve response envelope/cursor room
	last := ""
	for i, id := range ids {
		raw, err := json.Marshal(records[id])
		if err != nil {
			return out, err
		}
		if bytes+len(raw) > MaxReadBytes || len(out.Items) == limit {
			if len(out.Items) == 0 {
				return out, fmt.Errorf("history record exceeds response limit")
			}
			c, _ := json.Marshal(historyCursor{g.ID, target, kind, st.LastEventID, last})
			out.NextCursor = base64.RawURLEncoding.EncodeToString(c)
			break
		}
		out.Items = append(out.Items, raw)
		bytes += len(raw) + 1
		last = ids[i]
	}
	return out, nil
}
