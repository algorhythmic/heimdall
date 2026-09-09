package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"maps"
	"sync"
	"time"
)

type SnapshotRequest struct {
	Version              int                  `json:"version"`
	ID                   string               `json:"id"`
	Op                   string               `json:"op"`
	Target               string               `json:"target"`
	Previous             string               `json:"previous"`
	ExpectedTaskRevision int64                `json:"expected_task_revision,omitempty"`
	ManifestID           string               `json:"manifest_id,omitempty"`
	SourceID             string               `json:"source_id,omitempty"`
	InputDigest          string               `json:"input_digest,omitempty"`
	Policy               *SnapshotPolicyInput `json:"policy,omitempty"`
	SnapshotID           string               `json:"snapshot_id,omitempty"`
	SnapshotIDs          []string             `json:"snapshot_ids,omitempty"`
	Reason               string               `json:"reason,omitempty"`
}
type SnapshotPolicyInput struct {
	Enabled         bool `json:"enabled"`
	DebounceSeconds int  `json:"debounce_seconds"`
	MaxDirtySeconds int  `json:"max_dirty_seconds"`
	RetainCount     int  `json:"retain_count"`
}

func DecodeSnapshot(raw []byte) (SnapshotRequest, error) {
	var r SnapshotRequest
	if len(raw) > MaxRequest {
		return r, fmt.Errorf("snapshot request exceeds 64 KiB")
	}
	if err := model.StrictJSON(raw, &r); err != nil {
		return r, err
	}
	if err := r.Validate(); err != nil {
		return r, err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	allowed := map[string]bool{"version": true, "id": true, "op": true, "target": true, "previous": true}
	extras := map[string][]string{
		"capture": {"expected_task_revision", "manifest_id", "source_id", "input_digest"},
		"policy":  {"expected_task_revision", "manifest_id", "source_id", "policy"},
		"pin":     {"snapshot_id", "reason"}, "unpin": {"snapshot_id", "reason"}, "prune": {"snapshot_ids", "reason"},
	}
	for _, key := range extras[r.Op] {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return r, fmt.Errorf("field %s is not permitted for %s", key, r.Op)
		}
	}
	if r.Op == "policy" {
		var policy map[string]json.RawMessage
		_ = json.Unmarshal(fields["policy"], &policy)
		if _, ok := policy["enabled"]; !ok {
			return r, fmt.Errorf("policy enabled must be explicit")
		}
		if string(policy["enabled"]) == "null" {
			return r, fmt.Errorf("policy enabled cannot be null")
		}
	}
	return r, nil
}
func (r SnapshotRequest) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.ValidID(r.Target) || (r.Previous != "none" && !model.OpaqueID.MatchString(r.Previous)) {
		return fmt.Errorf("snapshot version, ID, target and explicit previous required")
	}
	switch r.Op {
	case "capture", "policy":
		if r.ExpectedTaskRevision < 1 || !model.OpaqueID.MatchString(r.ManifestID) || !model.OpaqueID.MatchString(r.SourceID) || r.SnapshotID != "" || r.SnapshotIDs != nil || r.Reason != "" {
			return fmt.Errorf("snapshot requires task, manifest and source preconditions")
		}
		if r.Op == "capture" {
			if r.Policy != nil || !model.TokenHashPattern.MatchString(r.InputDigest) {
				return fmt.Errorf("capture input digest required")
			}
		} else {
			if r.Policy == nil || r.InputDigest != "" {
				return fmt.Errorf("policy input required")
			}
			return r.policy(time.Unix(1, 0)).Validate()
		}
	case "pin", "unpin":
		if !model.OpaqueID.MatchString(r.SnapshotID) || r.SnapshotIDs != nil || r.Policy != nil || r.ExpectedTaskRevision != 0 || r.ManifestID != "" || r.SourceID != "" || r.InputDigest != "" {
			return fmt.Errorf("pin requires only snapshot ID, previous pin and reason")
		}
		return (model.SnapshotPin{Version: 1, ID: r.ID, Target: r.Target, SnapshotID: r.SnapshotID, Previous: previous(r.Previous), Active: r.Op == "pin", Reason: r.Reason, Actor: "cli", At: time.Unix(1, 0)}).Validate()
	case "prune":
		if r.Previous != "none" || r.SnapshotID != "" || r.Policy != nil || r.ExpectedTaskRevision != 0 || r.ManifestID != "" || r.SourceID != "" || r.InputDigest != "" {
			return fmt.Errorf("prune requires snapshot IDs and reason")
		}
		return (model.SnapshotPrune{Version: 1, ID: r.ID, Target: r.Target, SnapshotIDs: r.SnapshotIDs, Reason: r.Reason, Actor: "cli", At: time.Unix(1, 0)}).Validate()
	default:
		return fmt.Errorf("unknown snapshot operation")
	}
	return nil
}
func (r SnapshotRequest) policy(now time.Time) model.SnapshotPolicy {
	return model.SnapshotPolicy{Version: 1, ID: r.ID, Target: r.Target, Previous: previous(r.Previous), Enabled: r.Policy.Enabled, TaskRevision: r.ExpectedTaskRevision, ManifestID: r.ManifestID, SourceID: r.SourceID, DebounceSeconds: r.Policy.DebounceSeconds, MaxDirtySeconds: r.Policy.MaxDirtySeconds, RetainCount: r.Policy.RetainCount, Actor: "cli", At: now.UTC()}
}

type CaptureDiagnostic struct {
	LastAttempt   time.Time `json:"last_attempt"`
	LastObserved  time.Time `json:"last_observed"`
	Issue         string    `json:"issue"`
	DirtySince    time.Time `json:"dirty_since"`
	LastChanged   time.Time `json:"last_changed"`
	ContentDigest string    `json:"content_digest"`
}
type SnapshotService struct {
	Store       *store.Store
	Observer    *hyprland.Observer
	mu          sync.Mutex
	diagnostics map[string]CaptureDiagnostic
}

func pointPayload(st model.State, target string, observed hyprland.Status) (model.WorkspacePointPayload, error) {
	if !observed.Fresh || observed.Snapshot == nil {
		return model.WorkspacePointPayload{}, fmt.Errorf("fresh compositor coverage unavailable")
	}
	live := observed.Snapshot
	m := st.WorkspaceManifests[st.WorkspaceHeads[target]]
	p := model.WorkspacePointPayload{Boundary: model.SnapshotBoundary{Method: observed.Coverage, StartedAt: live.StartedAt, FinishedAt: live.CapturedAt, KnownEventGaps: observed.Gaps}, Version: 1, Target: target, TaskRevision: st.Tasks[target].Revision, ManifestID: m.ID, SourceEpoch: live.SourceEpoch, ObservedAt: live.CapturedAt, Coverage: "complete", Monitors: []model.DesktopMonitor{}, Workspaces: []model.DesktopWorkspace{}, Surfaces: []model.SnapshotSurface{}}
	for _, monitor := range live.Monitors {
		monitor.ActiveWorkspace = 0
		monitor.SpecialWorkspace = 0
		p.Monitors = append(p.Monitors, monitor)
	}
	workspaces := map[int]bool{}
	for _, surface := range m.Surfaces {
		row := model.SnapshotSurface{SurfaceID: surface.ID, ViewportBindingID: st.ViewportHeads[surface.ID], SessionBindingID: st.SessionHeads[surface.ID], Status: "unowned"}
		b := st.ViewportBindings[row.ViewportBindingID]
		if b.Active {
			row.Status = "unavailable"
			if b.Window != nil && b.Window.SourceEpoch == live.SourceEpoch {
				row.Status = "missing"
				for _, w := range live.Windows {
					if w.Identity == *b.Window {
						copy := w
						row.Window = &copy
						row.Status = "observed"
						workspaces[w.WorkspaceID] = true
						break
					}
				}
			}
		}
		if row.Window == nil {
			p.Coverage = "partial"
		}
		p.Surfaces = append(p.Surfaces, row)
	}
	if len(p.Surfaces) == 0 {
		p.Coverage = "partial"
	}
	for _, ws := range live.Workspaces {
		if workspaces[ws.ID] {
			p.Workspaces = append(p.Workspaces, ws)
		}
	}
	return p, nil
}
func (s *SnapshotService) Execute(ctx context.Context, r SnapshotRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("snapshot command requires local CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(r)
	if result, found, err := s.Store.CommandReceipt(ctx, "snapshot-"+r.ID, raw); err != nil || found {
		return result, err
	}
	if r.Op == "capture" {
		st, err := s.Store.State(ctx)
		if err != nil {
			return nil, err
		}
		observed, err := s.Observer.Read(ctx, true)
		if err != nil {
			return nil, err
		}
		return s.capture(ctx, r, st, observed, "cli", "", raw, now)
	}
	return s.Store.Transact(ctx, "snapshot-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		change := store.Change{Revision: st.Revision}
		var payload any
		verb := ""
		switch r.Op {
		case "policy":
			payload = r.policy(now)
			verb = "policy"
		case "pin", "unpin":
			payload = model.SnapshotPin{Version: 1, ID: r.ID, Target: r.Target, SnapshotID: r.SnapshotID, Previous: previous(r.Previous), Active: r.Op == "pin", Reason: r.Reason, Actor: actor, At: now.UTC()}
			verb = "pin"
		case "prune":
			payload = model.SnapshotPrune{Version: 1, ID: r.ID, Target: r.Target, SnapshotIDs: r.SnapshotIDs, Reason: r.Reason, Actor: actor, At: now.UTC()}
			verb = "pruned"
		}
		change.Events = []store.Pending{{Subject: "snapshot", Verb: verb, EntityID: r.ID, Payload: payload}}
		change.Result = payload
		return change, nil
	})
}
func (s *SnapshotService) capture(ctx context.Context, r SnapshotRequest, initial model.State, observed hyprland.Status, actor, policyID string, raw json.RawMessage, now time.Time) (json.RawMessage, error) {
	if model.SnapshotInputDigest(initial, r.Target) != r.InputDigest {
		return nil, fmt.Errorf("snapshot input changed: %w", store.ErrConflict)
	}
	p, err := pointPayload(initial, r.Target, observed)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(p)
	kind := "manual"
	if actor == "snapshotter" {
		kind = "automatic"
		if p.Coverage != "complete" {
			return nil, fmt.Errorf("partial coverage; retaining last complete point")
		}
	}
	point := model.WorkspacePoint{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, ManifestID: r.ManifestID, SourceID: r.SourceID, SourceEpoch: p.SourceEpoch, PreviousHead: previous(r.Previous), InputDigest: r.InputDigest, ContentDigest: model.SnapshotContentDigest(p), PayloadDigest: model.SnapshotHash(body), PayloadBytes: len(body), Kind: kind, PolicyID: policyID, Coverage: p.Coverage, Published: p.Coverage == "complete", ObservedAt: p.ObservedAt, Actor: actor, At: now.UTC()}
	retained, err := s.Store.WorkspaceRetainedPoints(ctx, r.Target)
	if err != nil {
		return nil, err
	}
	check := func(model.State) error {
		if err := s.Observer.Check(observed.Snapshot.ID); err != nil {
			return fmt.Errorf("%s: %w", err, store.ErrConflict)
		}
		return nil
	}
	return s.Store.TransactChecked(ctx, "snapshot-"+r.ID, actor, raw, now, check, func(st model.State) (store.Change, error) {
		if st.SnapshotHeads[r.Target].ID != point.PreviousHead || model.SnapshotInputDigest(st, r.Target) != r.InputDigest {
			return store.Change{}, fmt.Errorf("snapshot publication inputs changed: %w", store.ErrConflict)
		}
		change := store.Change{Revision: st.Revision, SnapshotPayloads: map[string]json.RawMessage{r.ID: body}, Events: []store.Pending{{Subject: "snapshot", Verb: "captured", EntityID: r.ID, Payload: point}}, Result: point}
		// Evaluate protection after the proposed head publication, so the former
		// head becomes eligible only in the transaction publishing its successor.
		future := st
		future.SnapshotHeads = maps.Clone(st.SnapshotHeads)
		if point.Published {
			future.SnapshotHeads[r.Target] = point
		}
		retain := 32
		if policy := st.SnapshotPolicies[r.Target]; policy.ID != "" {
			retain = policy.RetainCount
		}
		prune := []string{}
		recent := 0
		// Explicit pins are additional protected points; they do not consume the
		// rolling history allowance. Every manual capture creates such a pin.
		if point.Kind == "automatic" {
			recent = 1
		}
		for _, old := range retained {
			if _, pinned := st.SnapshotPins[old.Point.ID]; pinned {
				continue
			}
			recent++
			if recent > retain && !model.SnapshotProtected(future, old.Point.ID) && len(prune) < 256 {
				prune = append(prune, old.Point.ID)
			}
		}
		if len(prune) > 0 {
			change.Events = append(change.Events, store.Pending{Subject: "snapshot", Verb: "pruned", EntityID: r.ID, Payload: model.SnapshotPrune{Version: 1, ID: r.ID, Target: r.Target, SnapshotIDs: prune, Reason: "bounded workspace payload retention", Actor: actor, At: now.UTC()}})
		}
		return change, nil
	})
}
