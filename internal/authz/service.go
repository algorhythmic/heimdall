// Package authz implements explicit, expiring, task-scoped read credentials.
package authz

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

var ErrDenied = errors.New("client access denied")

type Request struct {
	Version int         `json:"version"`
	ID      string      `json:"id"`
	Op      string      `json:"op"`
	Grant   *IssueInput `json:"grant,omitempty"`
	GrantID string      `json:"grant_id,omitempty"`
}

type IssueInput struct {
	Name            string    `json:"name"`
	Target          string    `json:"target"`
	Subtree         bool      `json:"subtree"`
	ResourceIDs     []string  `json:"resource_ids"`
	TokenHash       string    `json:"token_hash"`
	ExpiresAt       time.Time `json:"expires_at"`
	CheckpointWrite bool      `json:"checkpoint_write,omitempty"`
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func Authenticate(st model.State, token string, now time.Time) (model.Grant, error) {
	if !model.TokenHashPattern.MatchString(token) {
		return model.Grant{}, ErrDenied
	}
	hash := HashToken(token)
	for _, g := range st.Grants {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(g.TokenHash)) == 1 {
			if g.RevokedAt != nil || !now.Before(g.ExpiresAt) || now.Before(g.At) {
				break
			}
			return g, nil
		}
	}
	return model.Grant{}, ErrDenied
}

type Service struct{ Store *store.Store }

func (s Service) Execute(ctx context.Context, r Request, now time.Time) (json.RawMessage, error) {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) {
		return nil, fmt.Errorf("invalid grant request")
	}
	if (r.Op == "grant.issue" && (r.Grant == nil || r.GrantID != "")) || (r.Op == "grant.revoke" && (r.Grant != nil || !model.OpaqueID.MatchString(r.GrantID))) || (r.Op != "grant.issue" && r.Op != "grant.revoke") {
		return nil, fmt.Errorf("invalid grant operation")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if len(raw) > 8192 {
		return nil, fmt.Errorf("grant request too large")
	}
	return s.Store.Transact(ctx, "grant-"+r.ID, "cli", raw, now, func(st model.State) (store.Change, error) {
		change := store.Change{Revision: st.Revision}
		var g model.Grant
		verb := "issued"
		if r.Op == "grant.issue" {
			input := r.Grant
			g = model.Grant{Version: 1, ID: r.ID, Name: input.Name, Target: input.Target, Subtree: input.Subtree, ResourceIDs: input.ResourceIDs, TokenHash: input.TokenHash, ExpiresAt: input.ExpiresAt, Actor: "cli", At: now.UTC()}
			if input.CheckpointWrite {
				g.Version = 2
				g.CheckpointWrite = true
			}
			if err := g.Validate(); err != nil {
				return change, err
			}
			if g.ExpiresAt.Sub(now) > 30*24*time.Hour {
				return change, fmt.Errorf("read grant lifetime exceeds 30 days")
			}
			if _, _, err := model.ResolveTarget(st, g.Target); err != nil {
				return change, err
			}
			for _, id := range g.ResourceIDs {
				resource, ok := st.Resources[id]
				if !ok || !resource.Active || !g.Contains(st, resource.Target) {
					return change, fmt.Errorf("resource outside grant scope")
				}
			}
		} else {
			var ok bool
			g, ok = st.Grants[r.GrantID]
			if !ok {
				return change, fmt.Errorf("unknown grant")
			}
			if g.RevokedAt != nil {
				return change, fmt.Errorf("grant already revoked: %w", store.ErrConflict)
			}
			at := now.UTC()
			g.RevokedAt = &at
			verb = "revoked"
		}
		change.Events = []store.Pending{{Subject: "grant", Verb: verb, EntityID: g.ID, Payload: g}}
		change.Result = Public(g)
		return change, nil
	})
}

// Public omits even the credential hash from ordinary grant listings.
func Public(g model.Grant) any {
	capabilities := []string{"task.read", "history.read", "context.read"}
	if g.Version == 2 && g.CheckpointWrite {
		capabilities = append(capabilities, "checkpoint.write")
	}
	return struct {
		ID           string     `json:"id"`
		Name         string     `json:"name"`
		Target       string     `json:"target"`
		Subtree      bool       `json:"subtree"`
		ResourceIDs  []string   `json:"resource_ids"`
		ExpiresAt    time.Time  `json:"expires_at"`
		RevokedAt    *time.Time `json:"revoked_at,omitempty"`
		Capabilities []string   `json:"capabilities"`
	}{g.ID, g.Name, g.Target, g.Subtree, g.ResourceIDs, g.ExpiresAt, g.RevokedAt, capabilities}
}

func List(st model.State) []any {
	ids := []string{}
	for id := range st.Grants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []any{}
	for _, id := range ids {
		out = append(out, Public(st.Grants[id]))
	}
	return out
}
