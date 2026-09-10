package store

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"time"
)

func applyAction(st *model.State, e Event) error {
	if e.Verb == "queued" {
		var probe struct {
			Adapter string `json:"adapter"`
		}
		if err := json.Unmarshal(e.Payload, &probe); err == nil && probe.Adapter == "wcu" {
			return applyExternalAction(st, e)
		}
	} else if st.Actions[e.EntityID].Intent.External != nil {
		return applyExternalAction(st, e)
	}

	if e.Verb == "queued" {
		var v model.ActionIntent
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != e.EntityID || e.Actor != "cli" || e.CommandID != v.AuthorityRef || !v.At.Equal(e.TS) {
			return fmt.Errorf("invalid action queue provenance")
		}
		if _, exists := st.Actions[v.ID]; exists {
			return fmt.Errorf("duplicate action")
		}
		if _, exists := st.BrowserOperations[v.ID]; exists {
			return fmt.Errorf("action ID already used by browser")
		}
		if !model.ActionInputsCurrent(*st, v) {
			return fmt.Errorf("action context changed: %w", ErrConflict)
		}
		if v.Native != nil {
			return queueNativeAction(st, e, v)
		}
		m := st.WorkspaceManifests[v.ManifestID]
		owned := false
		for _, surface := range m.Surfaces {
			if surface.ID == v.SurfaceID && surface.Kind == "browser" {
				owned = true
			}
		}
		if !owned || m.TaskRevision != v.TaskRevision {
			return fmt.Errorf("current task-owned browser surface required")
		}
		if v.Workspace != nil {
			w := v.Workspace
			op := st.WorkspaceOperations[w.OperationID]
			recipe := st.ApplicationRecipes[w.RecipeID]
			if w.PreviousViewport != st.ViewportHeads[v.SurfaceID] || st.Browsers[v.Browser.Profile].RecoveryProtocol != 1 {
				return fmt.Errorf("browser recovery capability or viewport changed")
			}
			if !v.At.Equal(op.Intent.At) || recipe.Spec == nil || recipe.Spec.Browser == nil || recipe.Spec.Browser.Profile != v.Browser.Profile || (v.Browser.Action == "open" && recipe.Spec.Browser.URL != v.Browser.URL) || (v.Browser.Action == "close" && (recipe.Spec.ClosePolicy != "owned_tab" || op.CloseSnapshotID == "")) || (v.Browser.Action == "close" && v.Target == op.Intent.Target && op.Intent.Kind != "close") || (op.Intent.Kind == "close" && v.Browser.Action != "close") {
				return fmt.Errorf("browser action differs from operation recipe or close scope")
			}
		}
		if v.SnapshotID != "" && !model.SnapshotProtected(*st, v.SnapshotID) {
			return fmt.Errorf("referenced snapshot must already be pinned or current")
		}
		if v.SnapshotID != "" {
			belongs := st.SnapshotHeads[v.Target].ID == v.SnapshotID || st.SnapshotPins[v.SnapshotID].Target == v.Target
			if v.Workspace != nil {
				belongs = belongs || model.WorkspaceActionScope(st.WorkspaceOperations[v.Workspace.OperationID], v)
			}
			if !belongs {
				return fmt.Errorf("foreign snapshot reference")
			}
		}
		if conflict := model.ActionConflict(*st, v); conflict != "" {
			return fmt.Errorf("surface has an unresolved action: %w", ErrConflict)
		}
		active := 0
		for _, a := range st.Actions {
			if model.ActionHolds(a) {
				active++
			}
		}
		if active >= 128 {
			return fmt.Errorf("unresolved action limit reached; reconcile existing actions")
		}
		p := st.Browsers[v.Browser.Profile]
		if !p.Paired || p.Epoch != v.Browser.Epoch || p.ActionProtocol != 1 {
			return fmt.Errorf("paired action-capable browser epoch required")
		}
		if v.Browser.Pairing != nil && (p.PairingProtocol != 1 || !model.BrowserExtensionIDPattern.MatchString(p.ExtensionID)) {
			return fmt.Errorf("pairing-capable browser required")
		}
		if v.Browser.Action != "open" {
			owner, ok := st.Actions[v.Browser.OwnerID]
			if !ok || owner.Intent.Target != v.Target || owner.Intent.SurfaceID != v.SurfaceID || owner.Intent.Browser == nil || owner.Intent.Browser.Action != "open" || owner.Intent.Browser.Profile != p.ID || owner.Intent.Browser.Epoch != p.Epoch {
				return fmt.Errorf("tab lacks exact task/surface ownership")
			}
			found := false
			for _, tab := range p.Tabs {
				if tab.ID == v.Browser.TabID && tab.OwnerID == v.Browser.OwnerID && tab.URL == v.Browser.ExpectedURL {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("owned browser instance or URL changed")
			}
		}
		st.Actions[v.ID] = model.ActionRecord{Intent: v, IntentDigest: model.ContentDigest(v), Revision: 1, Execution: "queued", Verification: "pending", UpdatedAt: v.At, LastEventID: e.ID}
		return nil
	}
	var v model.ActionTransition
	if err := model.StrictJSON(e.Payload, &v); err != nil {
		return err
	}
	if (v.Report != nil && (v.Report.Step != 0 || v.Report.Final || v.Report.WCURequestID != "" || v.Report.MetricsDigest != "")) || (v.Observation != nil && (v.Observation.External != nil || v.Observation.Browser != nil)) {
		return fmt.Errorf("external fields on legacy transition")
	}
	a, exists := st.Actions[v.ActionID]
	if !exists || (v.Version != 1 && v.Version != 2 && v.Version != 3 && v.Version != 4) || !model.OpaqueID.MatchString(v.ID) || v.ActionID != e.EntityID || v.AttemptID != a.Intent.AttemptID || v.PreviousRevision != a.Revision || v.Actor != e.Actor || !v.At.Equal(e.TS) || v.At.IsZero() || len(v.Reason) > 512 {
		return fmt.Errorf("invalid action transition identity, revision or provenance")
	}
	if a.Intent.Native == nil && (v.Version == 4 || (v.Report != nil && v.Report.Native != nil) || (v.Observation != nil && v.Observation.Native != nil)) {
		return fmt.Errorf("native fields on browser transition")
	}
	if a.Intent.Native != nil && (v.PairProbe != nil || v.PairReady != nil || v.AssociationID != "" || v.ContinuationID != "") {
		return fmt.Errorf("browser fields on native transition")
	}
	if v.PairProbe != nil && (v.Version != 3 || v.Kind != "pair_probe") {
		return fmt.Errorf("pairing probe on another transition")
	}
	if v.PairReady != nil && (v.Version != 3 || v.Kind != "pair_ready") {
		return fmt.Errorf("pairing report on another transition")
	}
	if v.AssociationID != "" && (v.Version != 3 || v.Kind != "pair_bound") {
		return fmt.Errorf("association on another transition")
	}
	if v.ContinuationID != "" && (v.Version != 3 || !model.Contains([]string{"pair_bound", "pair_continue", "report"}, v.Kind)) {
		return fmt.Errorf("continuation on another transition")
	}
	if v.Kind != "report" && v.Report != nil {
		return fmt.Errorf("report on another transition")
	}
	if v.Kind != "dispatch" && v.Kind != "pair_continue" && v.DeliveryID != "" {
		return fmt.Errorf("delivery identity on another transition")
	}
	if v.Kind != "observe" && v.Kind != "dispatch" && v.Kind != "verify" && v.Kind != "pair_continue" && v.Observation != nil {
		return fmt.Errorf("observation on another transition")
	}
	if a.Intent.Native != nil && model.Contains([]string{"dispatch", "report", "verify", "observe", "source_lost"}, v.Kind) {
		if err := transitionNativeAction(*st, &a, v, e); err != nil {
			return err
		}
	} else {
		switch v.Kind {
		case "pair_ready", "pair_probe", "pair_bound", "pair_continue", "pair_abandoned":
			if err := applyActionPairing(*st, &a, v); err != nil {
				return err
			}
		case "dispatch":
			if e.Actor != "coordinator" || !model.OpaqueID.MatchString(v.DeliveryID) || a.Execution != "queued" || a.CancelRequested || v.At.Before(a.Intent.At) || !v.At.Before(a.Intent.ExpiresAt) || !model.ActionInputsCurrent(*st, a.Intent) || !model.ActionBrowserCurrent(*st, a.Intent) {
				return fmt.Errorf("action cannot dispatch")
			}
			p := st.Browsers[a.Intent.Browser.Profile]
			o := v.Observation
			if o == nil || !model.OpaqueID.MatchString(o.ID) || o.Status != "unknown" || o.SourceEpoch != p.Epoch || o.Digest != model.ContentDigest(p) || !o.ObservedAt.Equal(p.ReceivedAt) || v.At.Before(p.ReceivedAt) || v.At.Sub(p.ReceivedAt) > 5*time.Second || !p.Complete {
				return fmt.Errorf("dispatch lacks a fresh observation reference")
			}
			if (v.Version >= 2 || a.Intent.Browser.Pairing != nil) && (p.Freshness == nil || !p.Freshness.Stable || p.Freshness.Challenge.AfterEventID < a.LastEventID) {
				return fmt.Errorf("dispatch requires challenged stable readback after intent")
			}
			a.Execution = "dispatching"
			a.DeliveryID = v.DeliveryID
			a.DispatchObservation = o
		case "refuse":
			if e.Actor != "coordinator" || a.Execution != "queued" || v.Reason == "" {
				return fmt.Errorf("action cannot be refused after possible dispatch")
			}
			a.Execution = "refused"
			a.Verification = "unknown"
		case "interrupt":
			if !model.Contains([]string{"coordinator", "observer:browser"}, e.Actor) || (a.Intent.Native != nil && e.Actor != "coordinator") || a.Execution != "dispatching" || v.Reason == "" {
				return fmt.Errorf("action cannot be interrupted")
			}
			a.Execution = "uncertain"
			a.Verification = "unknown"
			if a.UncertainSince.IsZero() {
				a.UncertainSince = v.At
			}
		case "cancel":
			if !model.Contains([]string{"cli", "coordinator"}, e.Actor) || !model.ActionHolds(a) || a.CancelRequested || v.Reason == "" {
				return fmt.Errorf("action cannot be cancelled")
			}
			a.CancelRequested = true
			if a.Execution == "queued" {
				a.Execution = "cancelled"
				a.Verification = "unknown"
			} else if a.Execution == "dispatching" {
				a.Execution = "uncertain"
				a.Verification = "unknown"
				if a.UncertainSince.IsZero() {
					a.UncertainSince = v.At
				}
			}
		case "report":
			r := v.Report
			if a.Intent.Browser.Pairing != nil {
				firstFailure := v.Version == 3 && v.ContinuationID == "" && a.Pairing == nil && r != nil && r.Status != "succeeded"
				if !firstFailure && (v.Version != 3 || a.Pairing == nil || a.Pairing.ContinuationDeliveryID == "" || v.ContinuationID != a.Pairing.ContinuationID) {
					return fmt.Errorf("final pairing result requires its dispatched continuation")
				}
			}
			if e.Actor != "observer:browser" || r == nil || !model.Contains([]string{"succeeded", "refused", "failed", "uncertain"}, r.Status) || len(r.Detail) > 512 || !model.Contains([]string{"dispatching", "uncertain"}, a.Execution) {
				return fmt.Errorf("unexpected action report")
			}
			if r.Status == "succeeded" && a.Intent.Browser.Action == "open" && (r.TabID < 1 || r.WindowID < 1 || !model.BrowserURL(r.URL)) {
				return fmt.Errorf("open report lacks runtime identity")
			}
			if r.Status == "succeeded" {
				a.Execution = "api_reported"
				if v.Version == 1 || a.Observation == nil || !model.Contains([]string{"matched", "not_matched"}, a.Observation.Status) {
					a.Verification = "pending"
				}
			} else if r.Status == "refused" && a.UncertainSince.IsZero() && a.Pairing == nil {
				a.Execution = "refused"
				a.Verification = "unknown"
			} else {
				a.Execution = "uncertain"
				a.Verification = "unknown"
				if a.UncertainSince.IsZero() {
					a.UncertainSince = v.At
				}
			}
			a.Report = r
			if v.Version >= 2 {
				a.VerificationAttempts = 0
			}
		case "source_lost":
			p := st.Browsers[a.Intent.Browser.Profile]
			if v.Version != 2 || !model.Contains([]string{"cli", "observer:browser"}, e.Actor) || !model.ActionHolds(a) || !model.Contains([]string{"api_reported", "uncertain"}, a.Execution) || v.Reason == "" || (p.Paired && p.Epoch == a.Intent.Browser.Epoch) {
				return fmt.Errorf("browser source is still current")
			}
			a.Verification = "unknown"
		case "reconcile":
			if v.Version != 2 || e.Actor != "cli" || model.Contains([]string{"queued", "cancelled", "refused"}, a.Execution) || v.Reason == "" {
				return fmt.Errorf("invalid explicit action reconciliation")
			}
			for id, other := range st.Actions {
				if id != a.Intent.ID && other.Intent.SurfaceID == a.Intent.SurfaceID && model.ActionHolds(other) {
					return fmt.Errorf("surface has another unresolved attempt: %w", ErrConflict)
				}
			}
			a.Verification = "pending"
			a.VerificationAttempts = 0
		case "verify":
			if v.Version != 2 || e.Actor != "observer:browser" || v.Observation == nil {
				return fmt.Errorf("invalid browser verification authority")
			}
			p := st.Browsers[a.Intent.Browser.Profile]
			o := v.Observation
			status, detail := model.BrowserOutcomeInState(*st, a, p)
			if !model.OpaqueID.MatchString(o.ID) || o.Status != status || o.Detail != detail || o.SourceEpoch != p.Epoch || o.Digest != model.ContentDigest(p) || !o.ObservedAt.Equal(p.ReceivedAt) || !o.ObservedAt.Equal(v.At) || p.Freshness == nil {
				return fmt.Errorf("verification differs from independent readback")
			}
			a.Observation = o
			a.VerificationAttempts++
			a.Verification = status
		case "observe":
			// C13 supplies independent browser postconditions. C12 can retain only
			// explicit unsupported/unknown observations, never manufactured success.
			o := v.Observation
			if a.Intent.Browser == nil || e.Actor != "observer:browser" || o == nil || !model.OpaqueID.MatchString(o.ID) || !model.Contains([]string{"unknown", "unsupported"}, o.Status) || o.SourceEpoch != a.Intent.Browser.Epoch || !model.TokenHashPattern.MatchString(o.Digest) || o.ObservedAt.IsZero() || o.ObservedAt.After(v.At) || len(o.Detail) > 512 || a.Execution == "queued" {
				return fmt.Errorf("invalid or unsupported verification observation")
			}
			a.Observation = o
			a.Verification = o.Status
		default:
			return fmt.Errorf("unknown action transition")
		}
	}
	if !reflect.DeepEqual(a.Intent, st.Actions[v.ActionID].Intent) {
		return fmt.Errorf("immutable action intent changed")
	}
	a.Revision++
	a.LastReason = v.Reason
	a.UpdatedAt = v.At
	a.LastEventID = e.ID
	st.Actions[v.ActionID] = a
	return nil
}

func validateActionOperation(st model.State, op model.BrowserOperation) error {
	a, ok := st.Actions[op.ID]
	if !ok || op.ActionRef.Validate() != nil || !reflect.DeepEqual(op.ActionRef, a.BrowserRef()) {
		return fmt.Errorf("browser action reference mismatch")
	}
	b := a.Intent.Browser
	if b == nil || op.Recovery != (a.Intent.Workspace != nil) {
		return fmt.Errorf("browser recovery authority changed")
	}
	if !reflect.DeepEqual(op.Pairing, b.Pairing) {
		return fmt.Errorf("browser pairing authority changed")
	}
	if op.Profile != b.Profile || op.Epoch != b.Epoch || op.Action != b.Action || op.URL != b.URL || op.OwnerID != b.OwnerID || !op.CreatedAt.Equal(a.Intent.At) || !op.ExpiresAt.Equal(a.Intent.ExpiresAt) {
		return fmt.Errorf("browser action intent changed")
	}
	if b.Action != "open" && (op.TabID != b.TabID || op.WindowID != b.WindowID || op.ExpectedURL != b.ExpectedURL) {
		return fmt.Errorf("browser target changed")
	}
	if b.Action == "open" && op.Status == "pending" && (op.TabID != 0 || op.WindowID != 0 || op.ExpectedURL != "") {
		return fmt.Errorf("queued open already has a runtime result")
	}
	if b.Action == "open" && op.Status == "succeeded" && (a.Report == nil || a.Report.Status != "succeeded" || op.TabID != a.Report.TabID || op.WindowID != a.Report.WindowID || op.ExpectedURL != a.Report.URL) {
		return fmt.Errorf("browser runtime identity lacks matching attempt report")
	}
	return nil
}

func (s *Store) ActionHistory(ctx context.Context, target, id string, before int64, limit int) ([]Event, error) {
	if !model.ValidActionTarget(target) || !model.OpaqueID.MatchString(id) || before < 0 || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid action history scope/cursor/limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := readState(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if st.Actions[id].Intent.Target != target {
		return nil, fmt.Errorf("action not found for task")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,event_version,ts,subject,verb,actor,entity_id,command_id,payload FROM events WHERE subject='action' AND entity_id=? AND (?=0 OR id<?) ORDER BY id DESC LIMIT ?", id, before, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var stamp string
		var payload []byte
		if err = rows.Scan(&e.ID, &e.Version, &stamp, &e.Subject, &e.Verb, &e.Actor, &e.EntityID, &e.CommandID, &payload); err != nil {
			return nil, err
		}
		e.Payload = payload
		e.TS, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
