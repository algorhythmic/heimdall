package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"slices"
	"sync"
	"time"
)

const OperationMaxBytes = 600 << 10

type OperationRequest struct {
	Version    int      `json:"version"`
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Preview    Preview  `json:"preview"`
	SurfaceIDs []string `json:"surface_ids"`
	Swap       *Preview `json:"swap,omitempty"`
}
type OperationControl struct {
	Version          int    `json:"version"`
	ID               string `json:"id"`
	Target           string `json:"target"`
	OperationID      string `json:"operation_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}
type NativeDispatcher interface {
	Prepare(context.Context, model.DesktopSource, hyprland.Command) (hyprland.PreparedCommand, error)
}
type ApplicationAdapter interface {
	Prepare(context.Context, model.DesktopSource, model.ApplicationSpec, string, *model.SessionBinding) (hyprland.PreparedCommand, error)
	Process(int) (model.ApplicationProcess, error)
}
type OperationService struct {
	Store        *store.Store
	Previews     *PreviewService
	Observer     *hyprland.Observer
	Dispatcher   NativeDispatcher
	Applications ApplicationAdapter
	Clock        func() time.Time
	mu           sync.Mutex
}

func (s *OperationService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// Batch previews the exact event order on an isolated state. The command
// receipt occupies the first cursor, including when a close capture and several
// shared actions commit together.
type operationBatch struct {
	state          model.State
	change         store.Change
	command, actor string
	at             time.Time
	cursor         int64
}

func newOperationBatch(st model.State, command, actor string, at time.Time) *operationBatch {
	return &operationBatch{state: model.Clone(st), change: store.Change{Revision: st.Revision}, command: command, actor: actor, at: at, cursor: st.LastEventID + 2}
}
func (b *operationBatch) add(subject, verb, id string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := store.Apply(&b.state, store.Event{ID: b.cursor, Version: 1, TS: b.at, Subject: subject, Verb: verb, EntityID: id, Actor: b.actor, CommandID: b.command, Payload: raw}); err != nil {
		return err
	}
	b.cursor++
	b.change.Events = append(b.change.Events, store.Pending{Subject: subject, Verb: verb, EntityID: id, Payload: payload})
	return nil
}

func residency(st model.State, observed hyprland.Status, now time.Time) (model.WorkspaceResidency, error) {
	r := model.WorkspaceResidency{Version: 1, ID: model.NewID(), SourceID: st.DesktopSourceHead, AfterEventID: st.LastEventID, ReceivedAt: now, Bindings: []model.ResidentBinding{}}
	if !observed.Fresh || observed.Snapshot == nil || observed.Snapshot.SourceEpoch != st.DesktopSources[r.SourceID].Epoch {
		return r, fmt.Errorf("fresh selected-source residency required")
	}
	p := observed.Snapshot
	r.SourceEpoch, r.SnapshotID, r.StartedAt, r.CapturedAt = p.SourceEpoch, p.ID, p.StartedAt, p.CapturedAt
	present := map[model.WindowIdentity]bool{}
	for _, w := range p.Windows {
		present[w.Identity] = true
	}
	for _, id := range st.ViewportHeads {
		b := st.ViewportBindings[id]
		if b.Active && b.Window != nil {
			r.Bindings = append(r.Bindings, model.ResidentBinding{BindingID: id, Known: b.Window.SourceEpoch == p.SourceEpoch, Present: present[*b.Window]})
		}
	}
	slices.SortFunc(r.Bindings, func(a, b model.ResidentBinding) int { return compareText(a.BindingID, b.BindingID) })
	return r, nil
}
func compareText(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func (s *OperationService) Queue(ctx context.Context, r OperationRequest, actor string) (json.RawMessage, error) {
	if actor != "cli" || r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.Contains([]string{"open", "focus", "close"}, r.Kind) || r.SurfaceIDs == nil || len(r.SurfaceIDs) > 32 || s.Previews == nil || s.Observer == nil {
		return nil, fmt.Errorf("explicit reviewed local workspace operation required")
	}
	raw, _ := json.Marshal(r)
	if len(raw) > OperationMaxBytes {
		return nil, fmt.Errorf("workspace operation exceeds request bound")
	}
	command := "workspace-operation-" + r.ID
	if result, found, err := s.Store.CommandReceipt(ctx, command, raw); err != nil || found {
		return result, err
	}
	for _, v := range []*Preview{&r.Preview, r.Swap} {
		if v == nil {
			continue
		}
		validation, err := s.Previews.Validate(ctx, *v, s.now())
		if err != nil {
			return nil, err
		}
		if !validation.Current {
			return nil, fmt.Errorf("workspace review requires refresh (%s): %w", validation.Issue, store.ErrConflict)
		}
	}
	st, point, err := s.Store.WorkspacePreviewInputs(ctx, r.Preview.Request.Target, r.Preview.Request.SnapshotID)
	if err != nil {
		return nil, err
	}
	var swapPoint store.PointView
	if r.Swap != nil {
		_, swapPoint, err = s.Store.WorkspacePreviewInputs(ctx, r.Swap.Request.Target, r.Swap.Request.SnapshotID)
		if err != nil {
			return nil, err
		}
	}
	checks := s.Previews.sessions(ctx, st, r.Preview.Request.Target, s.now())
	var swapChecks map[string]SessionCheck
	if r.Swap != nil {
		swapChecks = s.Previews.sessions(ctx, st, r.Swap.Request.Target, s.now())
	}
	observed, err := s.Observer.Read(ctx, true)
	if err != nil {
		return nil, err
	}
	now := s.now()
	live := planPreview(st, r.Preview.Request, point, observed, checks, now)
	if previewDigest(live) != r.Preview.Digest || !now.Before(r.Preview.ExpiresAt) {
		return nil, fmt.Errorf("workspace changed after review: %w", store.ErrConflict)
	}
	if r.Swap != nil {
		v := planPreview(st, r.Swap.Request, swapPoint, observed, swapChecks, now)
		if previewDigest(v) != r.Swap.Digest || !now.Before(r.Swap.ExpiresAt) {
			return nil, fmt.Errorf("swap changed after review: %w", store.ErrConflict)
		}
	}
	i := model.WorkspaceOperationIntent{Version: 1, ID: r.ID, Kind: r.Kind, Target: r.Preview.Request.Target, TaskRevision: r.Preview.TaskRevision, ManifestID: r.Preview.Request.ManifestID, SnapshotID: r.Preview.Request.SnapshotID, ContextDigest: model.ActionContextDigest(st, r.Preview.Request.Target), InputDigest: r.Preview.InputDigest, PreviewDigest: r.Preview.Digest, SourceID: r.Preview.SourceID, SourceEpoch: r.Preview.SourceEpoch, SurfaceIDs: slices.Clone(r.SurfaceIDs), Actor: actor, At: now, ExpiresAt: now.Add(time.Minute)}
	if r.Swap != nil {
		v := r.Swap
		i.Swap = &model.WorkspaceSwap{Target: v.Request.Target, TaskRevision: v.TaskRevision, ManifestID: v.Request.ManifestID, SnapshotID: v.Request.SnapshotID, ContextDigest: model.ActionContextDigest(st, v.Request.Target), InputDigest: v.InputDigest, PreviewDigest: v.Digest}
	}
	if err := i.Validate(); err != nil {
		return nil, err
	}
	intents, unsupported := operationActions(st, i, r.Preview, r.Swap)
	q := model.WorkspaceOperationQueued{Intent: i, ActionIDs: []string{}, Unsupported: unsupported}
	for _, a := range intents {
		q.ActionIDs = append(q.ActionIDs, a.ID)
	}
	diff := model.WorkspaceDiff{Version: 1, ID: model.NewID(), OperationID: i.ID, PreviewDigest: i.PreviewDigest, Changes: []model.WorkspaceIssue{}, At: now}
	if i.Swap != nil {
		diff.SwapPreviewDigest = i.Swap.PreviewDigest
	}
	for _, a := range intents {
		kind := ""
		if a.Native != nil {
			kind = a.Native.Kind
		} else {
			kind = a.Browser.Action
		}
		diff.Changes = append(diff.Changes, model.WorkspaceIssue{Target: a.Target, SurfaceID: a.SurfaceID, Reason: kind})
	}
	diff.Changes = append(diff.Changes, unsupported...)
	census, err := residency(st, observed, now)
	if err != nil {
		return nil, err
	}
	var closePoint *model.WorkspacePoint
	var closeBody json.RawMessage
	if r.Kind == "close" || r.Swap != nil {
		target := i.Target
		if i.Swap != nil {
			target = i.Swap.Target
		}
		payload, err := pointPayload(st, target, observed)
		if err != nil {
			return nil, err
		}
		closeBody, _ = json.Marshal(payload)
		closePoint = &model.WorkspacePoint{Version: 2, ID: model.NewID(), Target: target, TaskRevision: st.Tasks[target].Revision, ManifestID: st.WorkspaceHeads[target], SourceID: i.SourceID, SourceEpoch: i.SourceEpoch, PreviousHead: st.SnapshotHeads[target].ID, InputDigest: model.SnapshotInputDigest(st, target), ContentDigest: model.SnapshotContentDigest(payload), PayloadDigest: model.SnapshotHash(closeBody), PayloadBytes: len(closeBody), Kind: "operation", OperationID: i.ID, Coverage: payload.Coverage, Published: payload.Coverage == "complete", ObservedAt: payload.ObservedAt, Actor: actor, At: now}
	}
	return s.Store.TransactChecked(ctx, command, actor, raw, now, func(current model.State) error {
		if err := s.Observer.CheckCapture(observed.Snapshot.ID, observed.Snapshot.CapturedAt); err != nil {
			return fmt.Errorf("native review capture changed: %w", store.ErrConflict)
		}
		return nil
	}, func(current model.State) (store.Change, error) {
		if current.LastEventID != st.LastEventID {
			return store.Change{}, fmt.Errorf("workspace changed during review: %w", store.ErrConflict)
		}
		b := newOperationBatch(current, command, actor, now)
		if err := b.add("workspace", "residency_observed", census.ID, census); err != nil {
			return b.change, err
		}
		if err := b.add("workspace", "operation_queued", i.ID, q); err != nil {
			return b.change, err
		}
		if err := b.add("workspace", "diffed", diff.ID, diff); err != nil {
			return b.change, err
		}
		if closePoint != nil {
			if err := b.add("snapshot", "captured", closePoint.ID, closePoint); err != nil {
				return b.change, err
			}
			b.change.SnapshotPayloads = map[string]json.RawMessage{closePoint.ID: closeBody}
		}
		for _, a := range intents {
			if err := b.add("action", "queued", a.ID, a); err != nil {
				return b.change, err
			}
			if a.Browser != nil {
				intent := a.Browser
				record := b.state.Actions[a.ID]
				op := model.BrowserOperation{ID: a.ID, Profile: intent.Profile, Epoch: intent.Epoch, Action: intent.Action, TabID: intent.TabID, WindowID: intent.WindowID, OwnerID: intent.OwnerID, ExpectedURL: intent.ExpectedURL, URL: intent.URL, Pairing: intent.Pairing, Status: "pending", CreatedAt: a.At, ExpiresAt: a.ExpiresAt, ActionRef: record.BrowserRef()}
				op.Recovery = true
				if err := b.add("browser", "command_queued", a.ID, op); err != nil {
					return b.change, err
				}
			}
		}
		b.change.Result = b.state.WorkspaceOperations[i.ID]
		return b.change, nil
	})
}

func operationActions(st model.State, i model.WorkspaceOperationIntent, primary Preview, swap *Preview) ([]model.ActionIntent, []model.WorkspaceIssue) {
	intents, issues := []model.ActionIntent{}, []model.WorkspaceIssue{}
	add := func(v Preview, selected []string, kind string) {
		for _, row := range v.Surfaces {
			if !model.Contains(selected, row.SurfaceID) {
				continue
			}
			if row.Kind == "browser" {
				a, issue := browserOperationAction(st, i, v, row, kind)
				if issue != "" {
					issues = append(issues, model.WorkspaceIssue{Target: v.Request.Target, SurfaceID: row.SurfaceID, Reason: issue})
				} else {
					intents = append(intents, a)
				}
				continue
			}
			b := st.ViewportBindings[row.ViewportBindingID]
			recipe := st.ApplicationRecipes[st.ApplicationHeads[row.SurfaceID]]
			canLaunch := i.Kind == "open" && kind == "focus" && (row.Kind == "terminal" || row.Kind == "editor") && row.Window == nil && v.Fresh && model.ApplicationRecipeCurrent(st, recipe.ID, v.Request.Target, row.SurfaceID) && (recipe.Spec.SessionBindingID == "" || row.SessionStatus == "current")
			terminalClose := row.Kind == "terminal" && model.ApplicationRecipeCurrent(st, recipe.ID, v.Request.Target, row.SurfaceID) && model.Contains([]string{"graceful_session_end", "detach"}, recipe.Spec.ClosePolicy)
			issue := ""
			if row.Kind == "browser" {
				issue = "Browser surface requires fresh scoped application membership"
			} else if canLaunch {
				// Fresh absence plus an explicit recipe can replace an old epoch.
			} else if !b.Active || b.Window == nil || b.TaskRevision != v.TaskRevision || b.ManifestID != v.Request.ManifestID || b.SourceID != i.SourceID || b.Window.SourceEpoch != i.SourceEpoch {
				issue = "Current explicit native ownership required"
			} else if row.Window == nil {
				if kind == "close" && row.ObservationStatus == "missing" {
					continue
				}
				issue = "Reviewed launch or session attachment required"
			} else if kind == "close" && !terminalClose && (row.Kind != "native" || st.SessionBindings[st.SessionHeads[row.SurfaceID]].Active) {
				issue = "Application-aware graceful close or verified session detach required"
			}
			if issue != "" {
				issues = append(issues, model.WorkspaceIssue{Target: v.Request.Target, SurfaceID: row.SurfaceID, Reason: issue})
				continue
			}
			n := model.NativeIntent{OperationID: i.ID, Kind: kind, ViewportBindingID: b.ID, SourceID: i.SourceID, SourceEpoch: i.SourceEpoch, Window: b.Window}
			version := 3
			if canLaunch {
				version = 4
				n.Kind, n.Window = "open", nil
				n.Launch = &model.ApplicationLaunch{RecipeID: recipe.ID, PreviousViewport: row.ViewportBindingID, SessionBindingID: recipe.Spec.SessionBindingID, Editor: recipe.Spec.Editor != nil}
				n.ViewportBindingID = row.ViewportBindingID
			}
			if kind == "close" && terminalClose {
				version = 4
				n.RecipeID = recipe.ID
				n.SessionBindingID = recipe.Spec.SessionBindingID
			}
			intents = append(intents, model.ActionIntent{Version: version, ID: model.NewID(), Target: v.Request.Target, TaskRevision: v.TaskRevision, ManifestID: v.Request.ManifestID, SurfaceID: row.SurfaceID, ContextDigest: model.ActionContextDigest(st, v.Request.Target), SnapshotID: v.Request.SnapshotID, Adapter: "hyprland", AttemptID: model.NewID(), Authority: "cli", AuthorityRef: "workspace-operation-" + i.ID, Native: &n, Expected: model.NativePostcondition(n), At: i.At, ExpiresAt: i.At.Add(30 * time.Second)})
		}
	}
	if swap != nil {
		ids := []string{}
		for _, surface := range st.WorkspaceManifests[swap.Request.ManifestID].Surfaces {
			ids = append(ids, surface.ID)
		}
		add(*swap, ids, "close")
	}
	kind := i.Kind
	if kind == "open" {
		kind = "focus"
	}
	add(primary, i.SurfaceIDs, kind)
	return intents, issues
}

func (s *OperationService) Show(ctx context.Context, target, id string) (model.WorkspaceOperation, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return model.WorkspaceOperation{}, err
	}
	op := st.WorkspaceOperations[id]
	if !model.ValidID(target) || op.Intent.Target != target {
		return op, fmt.Errorf("workspace operation not found for task")
	}
	return op, nil
}
func (s *OperationService) List(ctx context.Context, target string) ([]model.WorkspaceOperation, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if !model.ValidID(target) || st.Tasks[target].Revision == 0 {
		return nil, fmt.Errorf("explicit existing task required")
	}
	out := []model.WorkspaceOperation{}
	for _, op := range st.WorkspaceOperations {
		if op.Intent.Target == target {
			out = append(out, op)
		}
	}
	slices.SortFunc(out, func(a, b model.WorkspaceOperation) int {
		if a.LastEventID > b.LastEventID {
			return -1
		}
		return 1
	})
	return out[:min(100, len(out))], nil
}
func (s *OperationService) Control(ctx context.Context, r OperationControl, kind, actor string) (json.RawMessage, error) {
	if actor != "cli" || r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.OpaqueID.MatchString(r.OperationID) || !model.ValidID(r.Target) || r.ExpectedRevision < 1 || r.Reason == "" || len(r.Reason) > 512 || !model.Contains([]string{"cancel", "reconcile"}, kind) {
		return nil, fmt.Errorf("explicit scoped operation control required")
	}
	raw, _ := json.Marshal(r)
	command := "workspace-" + kind + "-" + r.ID
	now := s.now()
	return s.Store.Transact(ctx, command, actor, raw, now, func(st model.State) (store.Change, error) {
		op := st.WorkspaceOperations[r.OperationID]
		if op.Intent.Target != r.Target || op.Revision != r.ExpectedRevision {
			return store.Change{}, fmt.Errorf("workspace operation scope or revision changed: %w", store.ErrConflict)
		}
		b := newOperationBatch(st, command, actor, now)
		verb := "operation_cancelled"
		if kind == "reconcile" {
			verb = "operation_reconciled"
		}
		v := model.WorkspaceOperationChange{Version: 1, ID: r.ID, OperationID: r.OperationID, PreviousRevision: r.ExpectedRevision, Reason: r.Reason, At: now}
		if err := b.add("workspace", verb, r.OperationID, v); err != nil {
			return b.change, err
		}
		for _, id := range op.ActionIDs {
			a := b.state.Actions[id]
			if kind == "cancel" && (!model.ActionHolds(a) || a.CancelRequested) {
				continue
			}
			if kind == "reconcile" && model.Contains([]string{"queued", "refused", "cancelled"}, a.Execution) {
				continue
			}
			t := actions.Transition(a, kind, r.Reason, actor, now)
			if kind == "reconcile" {
				t.Version = 2
			}
			if err := b.add("action", "transitioned", id, t); err != nil {
				return b.change, err
			}
		}
		b.change.Result = b.state.WorkspaceOperations[r.OperationID]
		return b.change, nil
	})
}
