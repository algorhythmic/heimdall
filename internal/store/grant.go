package store

import (
	"fmt"
	"heimdall/internal/model"
	"reflect"
)

func applyGrant(st *model.State, e Event) error {
	var g model.Grant
	if err := model.StrictJSON(e.Payload, &g); err != nil {
		return err
	}
	if err := g.Validate(); err != nil {
		return err
	}
	if e.Actor != "cli" || e.EntityID != g.ID {
		return fmt.Errorf("invalid grant authority")
	}
	old, exists := st.Grants[g.ID]
	switch e.Verb {
	case "issued":
		if exists || g.RevokedAt != nil || !g.At.Equal(e.TS) {
			return fmt.Errorf("invalid grant issue")
		}
		if _, _, err := model.ResolveTarget(*st, g.Target); err != nil {
			return err
		}
		for _, existing := range st.Grants {
			if existing.TokenHash == g.TokenHash {
				return fmt.Errorf("duplicate credential hash")
			}
		}
		for _, id := range g.ResourceIDs {
			r, ok := st.Resources[id]
			if !ok || !r.Active || !g.Contains(*st, r.Target) {
				return fmt.Errorf("resource outside grant scope")
			}
		}
	case "revoked":
		if !exists || old.RevokedAt != nil || g.RevokedAt == nil || !g.RevokedAt.Equal(e.TS) {
			return fmt.Errorf("invalid grant revocation")
		}
		old.RevokedAt = g.RevokedAt
		if !reflect.DeepEqual(old, g) {
			return fmt.Errorf("revocation changed immutable grant")
		}
	default:
		return fmt.Errorf("unknown grant event")
	}
	st.Grants[g.ID] = g
	return nil
}
