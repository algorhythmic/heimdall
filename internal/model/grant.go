package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var TokenHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Grant v1 authorizes reads only; v2 can explicitly allow checkpoint writes. Credentials never appear in events or results.
type Grant struct {
	Version         int        `json:"version"`
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Target          string     `json:"target"`
	Subtree         bool       `json:"subtree"`
	ResourceIDs     []string   `json:"resource_ids"`
	TokenHash       string     `json:"token_hash"`
	ExpiresAt       time.Time  `json:"expires_at"`
	At              time.Time  `json:"at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	Actor           string     `json:"actor"`
	CheckpointWrite bool       `json:"checkpoint_write,omitempty"`
}

func (g Grant) Validate() error {
	if (g.Version != 1 && g.Version != 2) || (g.Version == 1 && g.CheckpointWrite) || !OpaqueID.MatchString(g.ID) || !ValidID(g.Target) || g.Actor != "cli" || g.At.IsZero() || !g.ExpiresAt.After(g.At) || !TokenHashPattern.MatchString(g.TokenHash) || strings.TrimSpace(g.Name) == "" || len(g.Name) > 128 || len(g.ResourceIDs) > 16 {
		return fmt.Errorf("invalid read grant")
	}
	for i, id := range g.ResourceIDs {
		if !OpaqueID.MatchString(id) || (i > 0 && g.ResourceIDs[i-1] >= id) {
			return fmt.Errorf("invalid grant resource IDs")
		}
	}
	if g.RevokedAt != nil && g.RevokedAt.IsZero() {
		return fmt.Errorf("invalid revocation time")
	}
	return nil
}

func (g Grant) PermitsCheckpoint(st State, target, contractID string) bool {
	if g.Version != 2 || !g.CheckpointWrite || !g.Contains(st, target) {
		return false
	}
	lineage, err := TargetLineage(st, target)
	if err != nil {
		return false
	}
	for _, t := range lineage {
		if !g.Contains(st, t) {
			return false
		}
	}
	ids, err := ResourceScope(st, target)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if !Contains(g.ResourceIDs, id) {
			return false
		}
	}
	c, ok := st.Contracts[contractID]
	if !ok || c.Target != target {
		return false
	}
	for _, id := range c.ResourceIDs {
		if !Contains(g.ResourceIDs, id) {
			return false
		}
	}
	return true
}

func (g Grant) Contains(st State, target string) bool {
	r, _, err := ResolveTarget(st, target)
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	for {
		if r.Task.ID == g.Target {
			return true
		}
		if !g.Subtree || r.Task.Parent == "" || seen[r.Task.ID] {
			return false
		}
		seen[r.Task.ID] = true
		var ok bool
		r, ok = st.Tasks[r.Task.Parent]
		if !ok {
			return false
		}
	}
}
