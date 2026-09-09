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
	"time"
)

func (s *OperationService) Run(ctx context.Context) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			st, err := s.Store.State(ctx)
			if err != nil {
				continue
			}
			due := false
			for _, op := range st.WorkspaceOperations {
				if op.Status == "queued" || op.Status == "running" || (op.Status == "uncertain" && op.Outcome == "pending") {
					due = true
				}
			}
			for _, a := range st.Actions {
				if a.Intent.Workspace != nil && model.WorkspaceOperationHolds(st.WorkspaceOperations[a.Intent.Workspace.OperationID]) && !model.ActionHolds(a) {
					due = true
				}
				if a.Intent.Native != nil && ((a.Execution == "queued" && !a.CancelRequested) || (model.ActionHolds(a) && a.VerificationAttempts < 8 && a.Execution != "refused" && a.Execution != "cancelled")) {
					due = true
				}
			}
			if due {
				_ = s.Step(ctx)
			}
		}
	}
}

// Step is serialized across explicit calls and the background coordinator.
// Reports and reconciliation use the original attempt; no possibly dispatched
// action is ever prepared again.
func (s *OperationService) Step(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Observer == nil || s.Dispatcher == nil {
		return fmt.Errorf("native operation adapter unavailable")
	}
	if err := (actions.Service{Store: s.Store}).Sweep(ctx, s.now()); err != nil {
		return err
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	ids := []string{}
	for id, op := range st.WorkspaceOperations {
		if model.WorkspaceOperationHolds(op) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		for _, actionID := range st.WorkspaceOperations[id].ActionIDs {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			current, err := s.Store.State(ctx)
			if err != nil {
				return err
			}
			a := current.Actions[actionID]
			if a.Intent.Native == nil {
				continue
			}
			if a.Execution == "queued" {
				op := current.WorkspaceOperations[id]
				if a.Intent.Target == op.Intent.Target && !model.WorkspaceSwapReady(current, op) {
					if swapCannotProceed(current, op) {
						_ = s.refuse(ctx, a.Intent.ID, "Named swap did not verify closure; incoming input stopped")
					}
					continue
				}
				_ = s.dispatch(ctx, a)
			}
			current, err = s.Store.State(ctx)
			if err != nil {
				return err
			}
			a = current.Actions[actionID]
			if model.ActionHolds(a) && !model.Contains([]string{"queued", "cancelled", "refused"}, a.Execution) && a.VerificationAttempts < 8 {
				_ = s.verify(ctx, a)
			}
		}
	}
	return s.settle(ctx)
}

func swapCannotProceed(st model.State, op model.WorkspaceOperation) bool {
	if op.Intent.Swap == nil {
		return false
	}
	for _, issue := range op.Unsupported {
		if issue.Target == op.Intent.Swap.Target {
			return true
		}
	}
	for _, id := range op.ActionIDs {
		a := st.Actions[id]
		if a.Intent.Target != op.Intent.Swap.Target || a.Verification == "matched" {
			continue
		}
		if !model.Contains([]string{"queued", "dispatching"}, a.Execution) && (!model.ActionHolds(a) || a.VerificationAttempts >= 8) {
			return true
		}
	}
	return false
}

func nativeObservation(st model.State, a model.ActionRecord, observed hyprland.Status, now time.Time) (model.ActionObservation, error) {
	if !observed.Fresh || observed.Snapshot == nil || observed.Snapshot.SourceEpoch != a.Intent.Native.SourceEpoch {
		return model.ActionObservation{}, fmt.Errorf("fresh exact native source unavailable")
	}
	p := observed.Snapshot
	r := &model.NativeReadback{Version: 1, ActionID: a.Intent.ID, AttemptID: a.Intent.AttemptID, SourceID: a.Intent.Native.SourceID, SourceEpoch: p.SourceEpoch, SnapshotID: p.ID, AfterEventID: st.LastEventID, StartedAt: p.StartedAt, CapturedAt: p.CapturedAt, ReceivedAt: now, Complete: true, FocusKnown: observed.FocusKnown}
	for _, w := range p.Windows {
		if a.Intent.Native.Window != nil && w.Identity == *a.Intent.Native.Window {
			copy := w
			r.Window = &copy
			for _, ws := range p.Workspaces {
				if ws.ID == w.WorkspaceID {
					r.Workspace = ws.Name
				}
			}
		}
	}
	if observed.FocusedWindow != nil && a.Intent.Native.Window != nil && *observed.FocusedWindow == *a.Intent.Native.Window {
		copy := *observed.FocusedWindow
		r.FocusedWindow = &copy
	}
	return model.ActionObservation{ID: model.NewID(), Status: "unknown", SourceEpoch: p.SourceEpoch, Digest: model.ContentDigest(r), ObservedAt: p.CapturedAt, Native: r}, nil
}

func (s *OperationService) dispatch(ctx context.Context, a model.ActionRecord) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	n := a.Intent.Native
	if !model.ActionInputsCurrent(st, a.Intent) {
		return s.refuse(ctx, a.Intent.ID, "Task, ownership or operation inputs changed")
	}
	var prepared hyprland.PreparedCommand
	if n.Launch != nil {
		if s.Applications == nil {
			return s.refuse(ctx, a.Intent.ID, "Application adapter unavailable")
		}
		prepared, err = s.Applications.Prepare(ctx, st.DesktopSources[n.SourceID], *st.ApplicationRecipes[n.Launch.RecipeID].Spec, a.Intent.AttemptID, applicationSession(st, a))
	} else {
		if n.SessionBindingID != "" {
			binding := st.ViewportBindings[n.ViewportBindingID]
			owner := st.Actions[binding.ApplicationActionID]
			if s.Applications == nil || owner.Report == nil || owner.Report.Native == nil || owner.Report.Native.Process == nil {
				return s.refuse(ctx, a.Intent.ID, "Original attach process receipt unavailable")
			}
			p, err := s.Applications.Process(owner.Report.Native.Process.PID)
			if err != nil || p != *owner.Report.Native.Process {
				return s.refuse(ctx, a.Intent.ID, "Original attach process changed")
			}
		}
		prepared, err = s.Dispatcher.Prepare(ctx, st.DesktopSources[n.SourceID], hyprland.Command{Kind: n.Kind, Window: *n.Window, Workspace: n.Workspace})
	}
	if err != nil {
		return s.refuse(ctx, a.Intent.ID, "Native preparation refused: "+boundedReason(err.Error()))
	}
	defer prepared.Close()
	committed := false
	receipt := prepared.Send(ctx, func() error {
		now := s.now()
		observed := prepared.Observation()
		o, err := nativeObservation(st, a, observed, now)
		if err != nil {
			return err
		}
		o.Detail = "Fresh exact owned window before native input"
		if n.Launch != nil {
			if err := launchAbsent(st, a, observed); err != nil {
				return err
			}
			o.Detail = "Fresh absence before reviewed application launch"
		}
		if err := s.observeApplicationSession(ctx, st, a, &o); err != nil {
			return err
		}
		if n.SessionBindingID != "" {
			owner := st.Actions[st.ViewportBindings[n.ViewportBindingID].ApplicationActionID]
			if o.Native.Window == nil || o.Native.Window.PID != owner.Report.Native.Process.PID {
				return fmt.Errorf("detach view process changed")
			}
		}
		v := actions.Transition(a, "dispatch", "Persisted native input boundary", "coordinator", now)
		v.Version, v.DeliveryID, v.Observation = 4, model.NewID(), &o
		raw, _ := json.Marshal(v)
		_, err = s.Store.TransactChecked(ctx, "native-dispatch-"+v.ID, "coordinator", raw, now, func(model.State) error { return prepared.Check() }, func(current model.State) (store.Change, error) {
			if current.LastEventID != st.LastEventID {
				return store.Change{}, fmt.Errorf("action context changed before dispatch: %w", store.ErrConflict)
			}
			b := newOperationBatch(current, "native-dispatch-"+v.ID, "coordinator", now)
			if err := b.add("action", "transitioned", a.Intent.ID, v); err != nil {
				return b.change, err
			}
			b.change.Result = b.state.Actions[a.Intent.ID]
			return b.change, nil
		})
		committed = err == nil
		return err
	})
	if !committed {
		return s.refuse(ctx, a.Intent.ID, "Native input not authorized: "+boundedReason(receipt.Detail))
	}
	// A cancellation after this boundary cannot erase the possibly sent input.
	return s.report(ctx, a.Intent.ID, receipt)
}
func boundedReason(s string) string {
	if len(s) > 420 {
		return s[:420]
	}
	return s
}
func (s *OperationService) refuse(ctx context.Context, id, reason string) error {
	now := s.now()
	command := "native-refuse-" + model.NewID()
	_, err := s.Store.Transact(ctx, command, "coordinator", []byte(`{"version":1}`), now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision, Result: map[string]bool{"refused": true}}
		a := st.Actions[id]
		if a.Execution != "queued" {
			return c, nil
		}
		c.Events = []store.Pending{actions.Pending(actions.Transition(a, "refuse", reason, "coordinator", now))}
		return c, nil
	})
	return err
}
func (s *OperationService) report(ctx context.Context, id string, receipt hyprland.DispatchReceipt) error {
	now := s.now()
	command := "native-report-" + id
	raw, _ := json.Marshal(receipt)
	_, err := s.Store.Transact(ctx, command, "coordinator", raw, now, func(st model.State) (store.Change, error) {
		a := st.Actions[id]
		v := actions.Transition(a, "report", "Native dispatch receipt; postcondition requires readback", "coordinator", now)
		v.Version = 4
		status := "uncertain"
		if receipt.Acknowledged {
			status = "succeeded"
		}
		v.Report = &model.ActionReport{Status: status, Detail: boundedReason(receipt.Detail), Native: &model.NativeDispatchReport{Submitted: receipt.Submitted, Acknowledged: receipt.Acknowledged, Process: receipt.Process}}
		b := newOperationBatch(st, command, "coordinator", now)
		if err := b.add("action", "transitioned", id, v); err != nil {
			return b.change, err
		}
		b.change.Result = b.state.Actions[id]
		return b.change, nil
	})
	return err
}
func (s *OperationService) verify(ctx context.Context, a model.ActionRecord) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	var observed hyprland.Status
	if a.Intent.Native.Kind == "focus" {
		observed, err = s.Observer.ReadFocused(ctx)
	} else {
		observed, err = s.Observer.Read(ctx, true)
	}
	if err != nil {
		return s.unavailable(ctx, a.Intent.ID)
	}
	now := s.now()
	o, err := nativeObservation(st, a, observed, now)
	if err != nil {
		return s.unavailable(ctx, a.Intent.ID)
	}
	o.Status, o.Detail = model.NativeOutcomeInState(st, a, *o.Native)
	if a.Intent.Native.Launch != nil {
		s.observeLaunch(st, a, observed, &o)
	}
	_ = s.observeApplicationSession(ctx, st, a, &o)
	o.Status, o.Detail = model.NativeOutcomeInState(st, a, *o.Native)
	v := actions.Transition(a, "verify", "Independent native readback", "observer:hyprland", now)
	v.Version, v.Observation = 4, &o
	raw, _ := json.Marshal(v)
	command := "native-verify-" + v.ID
	_, err = s.Store.TransactChecked(ctx, command, "observer:hyprland", raw, now, func(model.State) error {
		return s.Observer.CheckCapture(observed.Snapshot.ID, observed.Snapshot.CapturedAt)
	}, func(current model.State) (store.Change, error) {
		if current.LastEventID != st.LastEventID {
			return store.Change{}, fmt.Errorf("action changed during readback: %w", store.ErrConflict)
		}
		b := newOperationBatch(current, command, "observer:hyprland", now)
		if err := b.add("action", "transitioned", a.Intent.ID, v); err != nil {
			return b.change, err
		}
		if a.Intent.Native.Launch != nil && o.Status == "matched" {
			binding := model.ViewportBinding{Version: 3, ID: v.ID, ApplicationActionID: a.Intent.ID, Target: a.Intent.Target, TaskRevision: a.Intent.TaskRevision, ManifestID: a.Intent.ManifestID, SurfaceID: a.Intent.SurfaceID, Previous: current.ViewportHeads[a.Intent.SurfaceID], Active: true, SourceID: o.Native.SourceID, SnapshotID: o.Native.SnapshotID, Window: &o.Native.Window.Identity, SessionBindingID: a.Intent.Native.Launch.SessionBindingID, Actor: "coordinator", At: now}
			if err := b.add("viewport", "bound", binding.ID, binding); err != nil {
				return b.change, err
			}
		}
		b.change.Result = b.state.Actions[a.Intent.ID]
		return b.change, nil
	})
	return err
}

func (s *OperationService) unavailable(ctx context.Context, id string) error {
	now := s.now()
	command := "native-unavailable-" + model.NewID()
	_, err := s.Store.Transact(ctx, command, "observer:hyprland", []byte(`{"version":1}`), now, func(st model.State) (store.Change, error) {
		a := st.Actions[id]
		v := actions.Transition(a, "observe", "Native observation unavailable", "observer:hyprland", now)
		v.Version = 4
		v.Observation = &model.ActionObservation{ID: model.NewID(), Status: "unknown", SourceEpoch: a.Intent.Native.SourceEpoch, Digest: model.ContentDigest(a.Intent.Native), ObservedAt: now, Detail: "Native readback unavailable; no input repeated"}
		b := newOperationBatch(st, command, "observer:hyprland", now)
		if err := b.add("action", "transitioned", id, v); err != nil {
			return b.change, err
		}
		b.change.Result = b.state.Actions[id]
		return b.change, nil
	})
	return err
}

func (s *OperationService) settle(ctx context.Context) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	observed, err := s.Observer.Read(ctx, true)
	if err != nil {
		return err
	}
	now := s.now()
	r, err := residency(st, observed, now)
	if err != nil {
		return err
	}
	command := "workspace-residency-" + r.ID
	raw, _ := json.Marshal(r)
	_, err = s.Store.TransactChecked(ctx, command, "observer:hyprland", raw, now, func(model.State) error {
		return s.Observer.CheckCapture(observed.Snapshot.ID, observed.Snapshot.CapturedAt)
	}, func(current model.State) (store.Change, error) {
		if current.LastEventID != st.LastEventID {
			return store.Change{}, fmt.Errorf("workspace changed during residency readback: %w", store.ErrConflict)
		}
		b := newOperationBatch(current, command, "observer:hyprland", now)
		if err := b.add("workspace", "residency_observed", r.ID, r); err != nil {
			return b.change, err
		}
		b.change.Result = map[string]bool{"observed": true}
		return b.change, nil
	})
	if err != nil {
		return err
	}
	st, err = s.Store.State(ctx)
	if err != nil {
		return err
	}
	ids := []string{}
	for id, op := range st.WorkspaceOperations {
		if model.WorkspaceOperationHolds(op) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		now = s.now()
		command = "workspace-settle-" + model.NewID()
		_, err = s.Store.Transact(ctx, command, "coordinator", []byte(`{"version":1}`), now, func(current model.State) (store.Change, error) {
			op := current.WorkspaceOperations[id]
			v := model.WorkspaceOperationChange{Version: 1, ID: model.NewID(), OperationID: id, PreviousRevision: op.Revision, Reason: "Shared attempts compared with fresh owned residency", At: now}
			// The reducer derives the outcome. Only its closed form can emit closed.
			for _, verb := range []string{"closed", "operation_settled"} {
				b := newOperationBatch(current, command, "coordinator", now)
				if err := b.add("workspace", verb, id, v); err != nil {
					if verb == "closed" {
						continue
					}
					return b.change, err
				}
				b.change.Result = b.state.WorkspaceOperations[id]
				return b.change, nil
			}
			return store.Change{}, fmt.Errorf("workspace settlement unavailable")
		})
		if err != nil {
			return err
		}
	}
	return nil
}
