// Package actions owns shared task-bound intent and attempt history. The first
// adapter is browser transport; replay and result reconciliation never dispatch.
package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"slices"
	"time"
)

const MaxRequest = 64 << 10

type Request struct {
	Version              int                 `json:"version"`
	ID                   string              `json:"id"`
	Target               string              `json:"target"`
	ExpectedTaskRevision int64               `json:"expected_task_revision"`
	ManifestID           string              `json:"manifest_id"`
	SurfaceID            string              `json:"surface_id"`
	ContextDigest        string              `json:"context_digest"`
	SnapshotID           string              `json:"snapshot_id,omitempty"`
	Browser              model.BrowserIntent `json:"browser"`
}
type CancelRequest struct {
	Version          int    `json:"version"`
	ID               string `json:"id"`
	Target           string `json:"target"`
	ActionID         string `json:"action_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}
type ContextView struct {
	Target        string `json:"target"`
	TaskRevision  int64  `json:"task_revision"`
	ManifestID    string `json:"manifest_id"`
	ContextDigest string `json:"context_digest"`
}
type Service struct{ Store *store.Store }

func Decode(raw []byte) (Request, error) {
	var r Request
	if len(raw) > MaxRequest {
		return r, fmt.Errorf("action request exceeds 64 KiB")
	}
	if err := model.StrictJSON(raw, &r); err != nil {
		return r, err
	}
	return r, r.intent(time.Unix(1, 0), model.NewID()).Validate()
}
func (r Request) intent(now time.Time, attempt string) model.ActionIntent {
	return model.ActionIntent{Version: r.Version, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, ManifestID: r.ManifestID, SurfaceID: r.SurfaceID, ContextDigest: r.ContextDigest, SnapshotID: r.SnapshotID, Adapter: "browser", AttemptID: attempt, Authority: "cli", AuthorityRef: "action-" + r.ID, Browser: &r.Browser, Expected: model.BrowserPostcondition(r.Browser), At: now.UTC(), ExpiresAt: now.UTC().Add(30 * time.Second)}
}
func (s Service) Context(ctx context.Context, target string) (ContextView, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return ContextView{}, err
	}
	if !model.ValidID(target) || st.Tasks[target].Revision < 1 {
		return ContextView{}, fmt.Errorf("existing task required")
	}
	return ContextView{target, st.Tasks[target].Revision, st.WorkspaceHeads[target], model.ActionContextDigest(st, target)}, nil
}
func (s Service) Queue(ctx context.Context, r Request, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("action queue requires local CLI authority")
	}
	raw, _ := json.Marshal(r)
	intent := r.intent(now, model.NewID())
	if err := intent.Validate(); err != nil {
		return nil, err
	}
	if result, found, err := s.Store.CommandReceipt(ctx, "action-"+r.ID, raw); err != nil || found {
		return result, err
	}
	if r.SnapshotID != "" {
		point, err := s.Store.WorkspacePoint(ctx, r.Target, r.SnapshotID)
		if err != nil {
			return nil, err
		}
		if point.Pruned || point.Payload == nil {
			return nil, fmt.Errorf("referenced snapshot payload unavailable")
		}
	}
	return s.Store.Transact(ctx, "action-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		queued := store.Pending{Subject: "action", Verb: "queued", EntityID: intent.ID, Payload: intent}
		if err := ApplyPending(&st, queued, "action-"+r.ID, actor, now); err != nil {
			return c, err
		}
		a := st.Actions[intent.ID]
		b := intent.Browser
		op := model.BrowserOperation{Pairing: b.Pairing, ID: r.ID, Profile: b.Profile, Epoch: b.Epoch, Action: b.Action, TabID: b.TabID, WindowID: b.WindowID, OwnerID: b.OwnerID, ExpectedURL: b.ExpectedURL, URL: b.URL, Status: "pending", CreatedAt: intent.At, ExpiresAt: intent.ExpiresAt, ActionRef: a.BrowserRef()}
		c.Events = []store.Pending{queued, {Subject: "browser", Verb: "command_queued", EntityID: op.ID, Payload: op}}
		c.Result = a
		return c, nil
	})
}

// ApplyPending is a pure reducer preview, used when several transitions share
// one real transaction. This helper is for its first domain event: the command
// receipt event is inserted first, so the predicted cursor advances by two.
func ApplyPending(st *model.State, p store.Pending, command, actor string, now time.Time) error {
	*st = model.Clone(*st)
	raw, _ := json.Marshal(p.Payload)
	return store.Apply(st, store.Event{ID: st.LastEventID + 2, Version: 1, TS: now.UTC(), Subject: p.Subject, Verb: p.Verb, EntityID: p.EntityID, Actor: actor, CommandID: command, Payload: raw})
}
func Transition(a model.ActionRecord, kind, reason, actor string, now time.Time) model.ActionTransition {
	return model.ActionTransition{Version: 1, ID: model.NewID(), ActionID: a.Intent.ID, AttemptID: a.Intent.AttemptID, PreviousRevision: a.Revision, Kind: kind, Reason: reason, Actor: actor, At: now.UTC()}
}
func Pending(v model.ActionTransition) store.Pending {
	return store.Pending{Subject: "action", Verb: "transitioned", EntityID: v.ActionID, Payload: v}
}
func (s Service) Cancel(ctx context.Context, r CancelRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" || (r.Version != 1 && r.Version != 2) || !model.OpaqueID.MatchString(r.ID) || !model.OpaqueID.MatchString(r.ActionID) || (r.Version == 1 && (!model.ValidActionTarget(r.Target) || r.ExpectedRevision < 1)) || (r.Version == 2 && (r.Target != "" || r.ExpectedRevision != 0)) || r.Reason == "" || len(r.Reason) > 512 {
		return nil, fmt.Errorf("invalid explicit action cancellation")
	}
	raw, _ := json.Marshal(r)
	return s.Store.Transact(ctx, "action-cancel-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		a := st.Actions[r.ActionID]
		if (r.Version == 1 && (a.Intent.Target != r.Target || a.Revision != r.ExpectedRevision)) || (r.Version == 2 && a.Intent.External == nil) {
			return c, fmt.Errorf("action scope/revision: %w", store.ErrConflict)
		}
		v := Transition(a, "cancel", r.Reason, actor, now)
		if a.Intent.External != nil {
			v.Version = 5
		}
		p := Pending(v)
		if err := ApplyPending(&st, p, "action-cancel-"+r.ID, actor, now); err != nil {
			return c, err
		}
		c.Events = []store.Pending{p}
		if op, ok := st.BrowserOperations[r.ActionID]; ok {
			op.Status = "cancelled"
			op.Detail = "cancellation requested; in-flight effects require reconciliation"
			c.Events = append(c.Events, store.Pending{Subject: "browser", Verb: "command_finished", EntityID: op.ID, Payload: op})
		}
		c.Result = st.Actions[r.ActionID]
		return c, nil
	})
}
func (s Service) Recover(ctx context.Context, now time.Time) error {
	_, err := s.Store.Transact(ctx, "action-recovery-"+s.Store.RuntimeID(), "coordinator", []byte(`{"version":1,"op":"recover"}`), now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision, Result: map[string]bool{"recovered": true}}
		ids := []string{}
		for id, a := range st.Actions {
			if a.Execution == "dispatching" {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		for _, id := range ids {
			c.Events = append(c.Events, Pending(Transition(st.Actions[id], "interrupt", "daemon restarted after possible dispatch", "coordinator", now)))
		}
		return c, nil
	})
	return err
}

// Deadlines settle only delivery eligibility. A possibly submitted operation
// becomes uncertain, and its surface/reference remains held for reconciliation.
func (s Service) Sweep(ctx context.Context, now time.Time) error {
	_, err := s.Store.Transact(ctx, "action-sweep-"+model.NewID(), "coordinator", []byte(`{"version":1,"op":"sweep"}`), now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision, Result: map[string]bool{"swept": true}}
		ids := []string{}
		for id, a := range st.Actions {
			if (a.Execution == "queued" || a.Execution == "dispatching") && !now.Before(a.Intent.ExpiresAt) {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		for _, id := range ids {
			a := st.Actions[id]
			kind, reason := "refuse", "delivery deadline passed before dispatch"
			if a.Execution == "dispatching" {
				kind, reason = "interrupt", "delivery deadline passed after possible dispatch"
			}
			c.Events = append(c.Events, Pending(Transition(a, kind, reason, "coordinator", now)))
			if op, ok := st.BrowserOperations[id]; a.Execution == "queued" && ok {
				op.Status = "refused"
				op.Detail = reason
				c.Events = append(c.Events, store.Pending{Subject: "browser", Verb: "command_finished", EntityID: id, Payload: op})
			}
		}
		return c, nil
	})
	return err
}
func (s Service) Show(ctx context.Context, target, id string) (model.ActionRecord, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return model.ActionRecord{}, err
	}
	a, ok := st.Actions[id]
	if !ok || a.Intent.Target != target {
		return a, fmt.Errorf("action not found for task")
	}
	return a, nil
}
func (s Service) List(ctx context.Context, target string, before int64, limit int) ([]model.ActionRecord, error) {
	if !model.ValidActionTarget(target) || before < 0 || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("action list requires task, nonnegative cursor and limit 1..100")
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if _, _, err := model.ResolveTarget(st, target); err != nil {
		return nil, fmt.Errorf("task not found")
	}
	out := []model.ActionRecord{}
	for _, a := range st.Actions {
		if a.Intent.Target == target && (before == 0 || a.LastEventID < before) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b model.ActionRecord) int {
		if a.LastEventID > b.LastEventID {
			return -1
		}
		if a.LastEventID < b.LastEventID {
			return 1
		}
		return 0
	})
	return out[:min(limit, len(out))], nil
}

type LegacyBrowserAction struct {
	Operation    model.BrowserOperation `json:"operation"`
	Execution    string                 `json:"execution"`
	Verification string                 `json:"verification"`
	Scope        string                 `json:"scope"`
}

func Legacy(o model.BrowserOperation) LegacyBrowserAction {
	execution := "uncertain"
	switch o.Status {
	case "pending":
		execution = "queued"
	case "succeeded":
		execution = "api_reported"
	case "refused":
		execution = "refused"
	case "cancelled":
		execution = "cancelled"
	}
	return LegacyBrowserAction{o, execution, "unsupported", "unscoped_legacy_browser"}
}

// Reconcile requests fresh observation of the same attempt and never dispatches it.
func (s Service) Reconcile(ctx context.Context, r CancelRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" || r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.OpaqueID.MatchString(r.ActionID) || !model.ValidActionTarget(r.Target) || r.ExpectedRevision < 1 || r.Reason == "" || len(r.Reason) > 512 {
		return nil, fmt.Errorf("invalid explicit reconciliation")
	}
	raw, _ := json.Marshal(r)
	return s.Store.Transact(ctx, "action-reconcile-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		a := st.Actions[r.ActionID]
		if a.Intent.Target != r.Target || a.Revision != r.ExpectedRevision {
			return c, fmt.Errorf("action scope/revision: %w", store.ErrConflict)
		}
		v := Transition(a, "reconcile", r.Reason, actor, now)
		v.Version = 2
		if a.Intent.External != nil {
			v.Version = 5
		}
		p := Pending(v)
		if err := ApplyPending(&st, p, "action-reconcile-"+r.ID, actor, now); err != nil {
			return c, err
		}
		c.Events = []store.Pending{p}
		c.Result = st.Actions[r.ActionID]
		return c, nil
	})
}
