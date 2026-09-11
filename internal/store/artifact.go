package store

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"sort"
	"strings"
	"time"
)

// SameContentPeers derives the same_content edges the spec calls for: equal
// digest proves equality across artifact records, computed deterministically
// over current versions rather than journaled.
func SameContentPeers(st model.State, versionID string) []string {
	v, ok := st.ArtifactVersions[versionID]
	if !ok {
		return nil
	}
	peers := []string{}
	for id, other := range st.ArtifactVersions {
		if id != versionID && other.Observation.Digest == v.Observation.Digest {
			peers = append(peers, id)
		}
	}
	sort.Strings(peers)
	return peers
}

// RecordArtifactOrigin journals the first exact-digest transfer of a version's
// content observed in a conversation record.
func (s *Store) RecordArtifactOrigin(ctx context.Context, o model.ArtifactOrigin, now time.Time) (json.RawMessage, error) {
	o.At = now.UTC()
	if err := o.Validate(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(o)
	id := "origin-" + conversation.Digest(raw)
	return s.Transact(ctx, id, sourceActor, raw, now, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"artifact", "origin_observed", o.VersionID, o}}}, nil
	})
}

func applyArtifact(st *model.State, e Event) error {
	if e.Verb != "origin_observed" && e.Actor != "cli" {
		return fmt.Errorf("artifact events require CLI authority")
	}
	switch e.Verb {
	case "registered":
		var a model.Artifact
		if err := model.StrictJSON(e.Payload, &a); err != nil {
			return err
		}
		if err := model.ValidArtifact(a); err != nil {
			return err
		}
		if a.Actor != e.Actor || !a.At.Equal(e.TS) || a.ID != e.EntityID {
			return fmt.Errorf("artifact envelope mismatch")
		}
		if _, ok := st.Artifacts[a.ID]; ok {
			return fmt.Errorf("duplicate artifact identity")
		}
		if _, _, err := model.ResolveTarget(*st, a.Target); err != nil {
			return err
		}
		count := 0
		for _, old := range st.Artifacts {
			if old.Target == a.Target {
				count++
			}
		}
		if count >= 128 {
			return fmt.Errorf("target artifact limit (128) exceeded")
		}
		st.Artifacts[a.ID] = a
	case "versioned":
		var v model.ArtifactVersion
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := model.ValidArtifactVersion(v); err != nil {
			return err
		}
		if v.Actor != e.Actor || !v.At.Equal(e.TS) || v.ID != e.EntityID {
			return fmt.Errorf("artifact version envelope mismatch")
		}
		a, ok := st.Artifacts[v.ArtifactID]
		if !ok || a.Target != v.Target || st.ArtifactHeads[a.ID] != v.Previous {
			return fmt.Errorf("invalid artifact version chain")
		}
		if _, ok := st.ArtifactVersions[v.ID]; ok {
			return fmt.Errorf("duplicate artifact version")
		}
		r, _, err := model.ResolveTarget(*st, v.Target)
		if err != nil || r.Revision != v.TaskRevision {
			return fmt.Errorf("artifact task revision mismatch")
		}
		scope, err := model.ResourceScope(*st, v.Target)
		if err != nil || !model.Contains(scope, v.ResourceID) {
			return fmt.Errorf("artifact resource outside target lineage")
		}
		if _, err = model.ArtifactResource(st.Resources[v.ResourceID], v.Path); err != nil {
			return err
		}
		st.ArtifactVersions[v.ID] = v
		st.ArtifactHeads[a.ID] = v.ID
	case "origin_observed":
		var p model.ArtifactOrigin
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		if e.Actor != sourceActor || e.EntityID != p.VersionID || !strings.HasPrefix(e.CommandID, "origin-") || !e.TS.Equal(p.At) {
			return fmt.Errorf("invalid artifact origin provenance")
		}
		if _, ok := st.ArtifactVersions[p.VersionID]; !ok {
			return fmt.Errorf("origin references an unknown version")
		}
		if _, ok := st.Conversations[p.ConversationID]; !ok {
			return fmt.Errorf("origin references an unknown conversation")
		}
		if _, ok := st.ArtifactOrigins[p.VersionID]; ok {
			return fmt.Errorf("artifact origin already recorded")
		}
		st.ArtifactOrigins[p.VersionID] = p
	default:
		return fmt.Errorf("unknown artifact event")
	}
	return nil
}

func validateCheckpointArtifacts(st model.State, v model.Checkpoint) error {
	for _, ref := range v.Artifacts {
		a, ok := st.Artifacts[ref.ArtifactID]
		if !ok || a.Target != v.Target {
			return fmt.Errorf("checkpoint artifact outside target")
		}
		x, ok := st.ArtifactVersions[ref.VersionID]
		if !ok || x.ArtifactID != a.ID || x.Target != v.Target || st.ArtifactHeads[a.ID] != x.ID {
			return fmt.Errorf("checkpoint artifact version is not current")
		}
		found := false
		for _, resource := range v.Resources {
			if resource.ID == x.ResourceID {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("checkpoint artifact resource is not reviewed")
		}
	}
	return nil
}
