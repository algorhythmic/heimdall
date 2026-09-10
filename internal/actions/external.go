package actions

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/authz"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

type IntentRequest struct {
	Version   int                       `json:"version"`
	ID        string                    `json:"id"`
	Target    string                    `json:"target"`
	Purpose   string                    `json:"purpose"`
	SurfaceID string                    `json:"surface_id,omitempty"`
	Expected  model.ActionPostcondition `json:"expected"`
	Steps     int                       `json:"steps,omitempty"`
}
type ReportRequest struct {
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Target        string `json:"target"`
	IntentID      string `json:"intent_id"`
	Step          int    `json:"step"`
	Outcome       string `json:"outcome"`
	WCURequestID  string `json:"wcu_request_id,omitempty"`
	MetricsDigest string `json:"metrics_digest,omitempty"`
}
type Registration struct {
	Action    model.ActionRecord  `json:"action"`
	Window    model.DesktopWindow `json:"window"`
	Workspace string              `json:"workspace"`
}
type ExternalObserver interface {
	ReadFocused(context.Context) (hyprland.Status, error)
	Check(string) error
}
type ExternalBrowserReader interface {
	ObserveWorkspace(context.Context, string, []string, time.Time) error
	RecoveryFresh(model.BrowserProfile, int64, time.Time) bool
}
type ExternalService struct {
	Browser  ExternalBrowserReader
	Store    *store.Store
	Observer ExternalObserver
	Clock    func() time.Time
	// Optional corroboration is read-only and cannot affect postcondition matching.
	Corroborate func(context.Context, model.DesktopWindow, model.ActionRecord) (*model.ExternalObservation, error)
}

func (s *ExternalService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
func (s *ExternalService) grant(st model.State, token, target string) (model.Grant, error) {
	g, err := authz.Authenticate(st, token, s.now())
	if err != nil || g.Version != 2 || !g.ActionWrite || !g.Contains(st, target) {
		return g, authz.ErrDenied
	}
	return g, nil
}
func workspaceName(p *model.DesktopSnapshot, w model.DesktopWindow) string {
	for _, v := range p.Workspaces {
		if v.ID == w.WorkspaceID {
			return v.Name
		}
	}
	return ""
}
func (s *ExternalService) Register(ctx context.Context, r IntentRequest, token string) (json.RawMessage, error) {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || (r.SurfaceID != "" && !model.OpaqueID.MatchString(r.SurfaceID)) || model.ValidateExternalExpected(r.Expected) != nil {
		return nil, fmt.Errorf("invalid external intent request")
	}
	if r.Steps == 0 {
		r.Steps = 1
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.grant(st, token, r.Target)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(r)
	actor := "grant:" + g.ID
	command := "external-intent-" + g.ID + "-" + r.ID
	if _, exists := st.Actions[r.ID]; exists {
		return s.Store.TransactChecked(ctx, command, actor, raw, s.now(), func(current model.State) error {
			actual, err := s.grant(current, token, r.Target)
			a := current.Actions[r.ID]
			if err != nil || actual.ID != g.ID || a.Intent.AuthorityRef != g.ID || !s.now().Before(a.Intent.ExpiresAt) || !model.ExternalInputsCurrent(current, a.Intent) {
				return authz.ErrDenied
			}
			return nil
		}, func(model.State) (store.Change, error) { return store.Change{}, store.ErrConflict })
	}
	if s.Observer == nil {
		return nil, fmt.Errorf("desktop observer unavailable")
	}
	registrationAfter := st.LastEventID
	browserSurfaces := []string{}
	for _, owned := range model.OwnedWindows(st, r.Target) {
		if owned.Browser && (r.SurfaceID == "" || r.SurfaceID == owned.SurfaceID) {
			browserSurfaces = append(browserSurfaces, owned.SurfaceID)
		}
	}
	if len(browserSurfaces) > 0 && s.Browser != nil {
		task, _, _ := model.ResolveTarget(st, r.Target)
		if err := s.Browser.ObserveWorkspace(ctx, task.Task.ID, browserSurfaces, s.now()); err != nil {
			return nil, err
		}
		st, err = s.Store.State(ctx)
		if err != nil {
			return nil, err
		}
	}
	observed, err := s.Observer.ReadFocused(ctx)
	if err != nil || !observed.Fresh || observed.Snapshot == nil {
		return nil, fmt.Errorf("fresh desktop observation required")
	}
	var selected model.OwnedWindow
	var window model.DesktopWindow
	found := 0
	for _, owned := range model.OwnedWindows(st, r.Target) {
		if r.SurfaceID != "" && owned.SurfaceID != r.SurfaceID {
			continue
		}
		for _, w := range observed.Snapshot.Windows {
			if w.Identity != owned.Window || (r.SurfaceID == "" && (!observed.FocusKnown || observed.FocusedWindow == nil || *observed.FocusedWindow != w.Identity)) {
				continue
			}
			selected, window = owned, w
			found++
		}
	}
	if found != 1 {
		return nil, fmt.Errorf("one explicitly owned live window required; no implicit focus ownership")
	}
	task, _, _ := model.ResolveTarget(st, r.Target)
	now := s.now()
	expires := now.Add(120 * time.Second)
	if g.ExpiresAt.Before(expires) {
		expires = g.ExpiresAt
	}
	intent := model.ActionIntent{Version: 6, ID: r.ID, Target: r.Target, TaskRevision: task.Revision, ManifestID: st.WorkspaceHeads[task.Task.ID], SurfaceID: selected.SurfaceID, ContextDigest: model.ActionContextDigest(st, r.Target), Adapter: "wcu", AttemptID: model.NewID(), Authority: "grant", AuthorityRef: g.ID, External: &model.ExternalIntent{Owned: selected, Purpose: r.Purpose, Steps: r.Steps}, Expected: r.Expected, At: now, ExpiresAt: expires}
	if selected.Browser {
		pin, ok := model.ExternalBrowserScope(st, selected)
		pin.AfterEventID = registrationAfter
		if !ok || s.Browser == nil || !s.Browser.RecoveryFresh(st.Browsers[pin.Profile], registrationAfter, s.now()) || !model.ExternalBrowserTitle(st, pin, window.Title) {
			return nil, fmt.Errorf("fresh exact owned-tab scope and matching native title required")
		}
		intent.External.Browser = &pin
	}
	if err := intent.ValidateExternal(); err != nil {
		return nil, err
	}
	authorize := func(current model.State) error {
		actual, err := s.grant(current, token, r.Target)
		if err != nil || actual.ID != g.ID || !model.ExternalInputsCurrent(current, intent) {
			return authz.ErrDenied
		}
		if existing, ok := current.Actions[r.ID]; ok && !s.now().Before(existing.Intent.ExpiresAt) {
			return authz.ErrDenied
		}
		if pin := intent.External.Browser; pin != nil {
			scope, ok := model.ExternalBrowserScope(current, selected)
			scope.AfterEventID = pin.AfterEventID
			if !ok || scope != *pin || !s.Browser.RecoveryFresh(current.Browsers[pin.Profile], pin.AfterEventID, s.now()) {
				return authz.ErrDenied
			}
		}
		return s.Observer.Check(observed.Snapshot.ID)
	}
	return s.Store.TransactChecked(ctx, command, actor, raw, now, authorize, func(current model.State) (store.Change, error) {
		p := store.Pending{Subject: "action", Verb: "queued", EntityID: intent.ID, Payload: intent}
		if err := ApplyPending(&current, p, command, actor, now); err != nil {
			return store.Change{}, err
		}
		return store.Change{Revision: current.Revision, Events: []store.Pending{p}, Result: Registration{Action: current.Actions[r.ID], Window: window, Workspace: workspaceName(observed.Snapshot, window)}}, nil
	})
}
func (s *ExternalService) Report(ctx context.Context, r ReportRequest, token string) (result json.RawMessage, resultErr error) {
	defer func() {
		if errors.Is(resultErr, authz.ErrDenied) {
			resultErr = s.refusal(ctx, r, token, resultErr)
		}
	}()

	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.OpaqueID.MatchString(r.IntentID) {
		return nil, fmt.Errorf("invalid external report")
	}
	raw, _ := json.Marshal(r)
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.grant(st, token, r.Target)
	if err != nil {
		return nil, err
	}
	actor := "grant:" + g.ID
	command := "external-report-" + g.ID + "-" + r.ID
	now := s.now()
	authorize := func(st model.State) error {
		current, err := s.grant(st, token, r.Target)
		a := st.Actions[r.IntentID]
		if err != nil || current.ID != g.ID || a.Intent.External == nil || a.Intent.AuthorityRef != g.ID || a.Intent.Target != r.Target || a.CancelRequested || !s.now().Before(a.Intent.ExpiresAt) || !model.ExternalInputsCurrent(st, a.Intent) {
			return authz.ErrDenied
		}
		return nil
	}
	return s.Store.TransactChecked(ctx, command, actor, raw, now, authorize, func(st model.State) (store.Change, error) {
		a := st.Actions[r.IntentID]
		v := Transition(a, "report", "Agent-reported external input outcome", actor, now)
		v.Version = 5
		v.Report = &model.ActionReport{Step: r.Step, Final: r.Step == a.Intent.External.Steps, Status: r.Outcome, WCURequestID: r.WCURequestID, MetricsDigest: r.MetricsDigest}
		p := Pending(v)
		if err := ApplyPending(&st, p, command, actor, now); err != nil {
			return store.Change{}, err
		}
		return store.Change{Revision: st.Revision, Events: []store.Pending{p}, Result: st.Actions[r.IntentID]}, nil
	})
}
func (s *ExternalService) Observe(ctx context.Context, id string) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	a := st.Actions[id]
	if a.Intent.External == nil {
		return fmt.Errorf("external intent required")
	}
	if a.Intent.External.Browser != nil && s.Browser != nil {
		task, _, _ := model.ResolveTarget(st, a.Intent.Target)
		_ = s.Browser.ObserveWorkspace(ctx, task.Task.ID, []string{a.Intent.SurfaceID}, s.now())
		st, err = s.Store.State(ctx)
		if err != nil {
			return err
		}
		if st.Actions[id].Revision != a.Revision {
			return store.ErrConflict
		}
	}
	o, snapshotID := s.readObservation(ctx, st, a, true)
	now := s.now()
	command := "external-observe-" + model.NewID()
	_, err = s.Store.Transact(ctx, command, "coordinator", []byte(id), now, func(current model.State) (store.Change, error) {
		currentAction := current.Actions[id]
		if currentAction.Revision != a.Revision {
			return store.Change{}, store.ErrConflict
		}
		if snapshotID != "" && s.Observer.Check(snapshotID) != nil {
			o.Native = nil
			o.External = nil
			o.Digest = ""
		}
		if pin := a.Intent.External.Browser; pin != nil && (s.Browser == nil || !s.Browser.RecoveryFresh(current.Browsers[pin.Profile], a.LastEventID, s.now())) {
			o.Browser = nil
		}
		o.Status, o.Detail = model.ExternalOutcome(current, a, o, now)
		v := Transition(a, "reconcile", "Independent external-action reconciliation", "coordinator", now)
		v.Version = 5
		if o.External != nil && o.External.TargetMismatch {
			v.Reason = "target_mismatch"
		}
		v.Observation = o
		p := Pending(v)
		if err := ApplyPending(&current, p, command, "coordinator", now); err != nil {
			return store.Change{}, err
		}
		return store.Change{Revision: current.Revision, Events: []store.Pending{p}, Result: current.Actions[id]}, nil
	})
	return err
}
func (s *ExternalService) Settle(ctx context.Context, restart bool) error {
	now := s.now()
	_, err := s.Store.Transact(ctx, "external-settle-"+model.NewID(), "coordinator", []byte("settle"), now, func(st model.State) (store.Change, error) {
		change := store.Change{Revision: st.Revision}
		ids := []string{}
		for id, a := range st.Actions {
			if a.Intent.External != nil && (a.Execution == "external" || !a.CancelRequested) {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			a := st.Actions[id]
			kind := ""
			g := st.Grants[a.Intent.AuthorityRef]
			if !a.CancelRequested && (g.RevokedAt != nil || (a.Execution == "external" && !now.Before(g.ExpiresAt))) {
				kind = "cancel"
			} else if a.Execution == "external" {
				if restart {
					kind = "interrupt"
				} else if !now.Before(a.Intent.ExpiresAt) {
					kind = "expire"
				}
			}
			if kind != "" {
				v := Transition(a, kind, "External reporting authority ended; input is never repeated", "coordinator", now)
				v.Version = 5
				change.Events = append(change.Events, Pending(v))
			}
		}
		return change, nil
	})
	return err
}
func (s *ExternalService) Run(ctx context.Context) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = s.Settle(ctx, false)
			st, err := s.Store.State(ctx)
			if err != nil {
				continue
			}
			ids := []string{}
			for id, a := range st.Actions {
				if a.Intent.External != nil && a.Execution != "external" && a.Verification != "matched" && a.VerificationAttempts < 8 {
					ids = append(ids, id)
				}
			}
			sort.Strings(ids)
			for _, id := range ids {
				bounded, cancel := context.WithTimeout(ctx, 6*time.Second)
				_ = s.Observe(bounded, id)
				cancel()
			}
		}
	}
}

// Refusal is a durable command receipt, never an action report or new authority.
// Unknown credentials and foreign intents receive the ordinary opaque denial.
type Refusal struct{ Receipt json.RawMessage }

func (r *Refusal) Error() string { return "external report refused" }
func (r *Refusal) Unwrap() error { return authz.ErrDenied }
func (s *ExternalService) refusal(ctx context.Context, r ReportRequest, token string, denied error) error {
	if !model.TokenHashPattern.MatchString(token) {
		return denied
	}
	hash := authz.HashToken(token)
	raw, _ := json.Marshal(r)
	now := s.now()
	state, err := s.Store.State(ctx)
	if err != nil {
		return denied
	}
	known := state.Actions[r.IntentID]
	grant := state.Grants[known.Intent.AuthorityRef]
	if known.Intent.External == nil || known.Intent.Target != r.Target || subtle.ConstantTimeCompare([]byte(grant.TokenHash), []byte(hash)) != 1 {
		return denied
	}
	result, err := s.Store.Transact(ctx, "external-refusal-"+grant.ID+"-"+r.ID, "grant:"+grant.ID, raw, now, func(st model.State) (store.Change, error) {
		a := st.Actions[r.IntentID]
		g := st.Grants[a.Intent.AuthorityRef]
		if a.Intent.External == nil || a.Intent.Target != r.Target || subtle.ConstantTimeCompare([]byte(g.TokenHash), []byte(hash)) != 1 {
			return store.Change{}, authz.ErrDenied
		}
		return store.Change{Revision: st.Revision, Result: map[string]any{"request_id": r.ID, "intent_id": r.IntentID, "status": "refused", "code": "access_denied"}}, nil
	})
	if err != nil {
		return denied
	}
	return &Refusal{Receipt: result}
}

func (s *ExternalService) readObservation(ctx context.Context, st model.State, a model.ActionRecord, corroborate bool) (*model.ActionObservation, string) {
	o := &model.ActionObservation{ID: model.NewID(), ObservedAt: s.now()}
	var snapshotID string
	if s.Observer != nil {
		observed, readErr := s.Observer.ReadFocused(ctx)
		if readErr == nil && observed.Fresh && observed.Snapshot != nil {
			p := observed.Snapshot
			snapshotID = p.ID
			read := &model.NativeReadback{Version: 1, ActionID: a.Intent.ID, AttemptID: a.Intent.AttemptID, SourceID: a.Intent.External.Owned.SourceID, SourceEpoch: p.SourceEpoch, SnapshotID: p.ID, AfterEventID: st.LastEventID, StartedAt: p.StartedAt, CapturedAt: p.CapturedAt, ReceivedAt: s.now(), Complete: true, FocusKnown: observed.FocusKnown, FocusedWindow: observed.FocusedWindow}
			for _, w := range p.Windows {
				if w.Identity == a.Intent.External.Owned.Window {
					copy := w
					read.Window = &copy
					read.Workspace = workspaceName(p, w)
				}
			}
			if a.Intent.External.Browser != nil && read.Window != nil {
				read.Window.Title = ""
				read.Window.Class = ""
			}
			o.Native = read
			o.SourceEpoch = p.SourceEpoch
			o.ObservedAt = p.CapturedAt
			o.Digest = model.ContentDigest(read)
			if corroborate && s.Corroborate != nil && read.Window != nil {
				o.External, _ = s.Corroborate(ctx, *read.Window, a)
			}
		}
	}

	if pin := a.Intent.External.Browser; pin != nil && s.Browser != nil {
		p := st.Browsers[pin.Profile]
		if s.Browser.RecoveryFresh(p, a.LastEventID, s.now()) {
			o.Browser = &model.ExternalBrowserObservation{Profile: p.ID, Epoch: p.Epoch, Sequence: p.LastSequence, Digest: model.ContentDigest(p)}
		}
	}
	return o, snapshotID
}
func (s *ExternalService) PrepareCompletion(ctx context.Context, st model.State, target string) error {
	for _, id := range model.ActionChecks(st, target) {
		a, ok := model.VerifiedAction(st, target, id)
		if !ok {
			continue
		}
		if a.Intent.External.Browser != nil {
			if s.Browser == nil {
				return fmt.Errorf("browser readback unavailable")
			}
			task, _, _ := model.ResolveTarget(st, target)
			if err := s.Browser.ObserveWorkspace(ctx, task.Task.ID, []string{a.Intent.SurfaceID}, s.now()); err != nil {
				return err
			}
		}
	}
	return nil
}

// Called under the task writer: observe only; never call a daemon/store method
// from here. Browser challenges were prepared before entering the transaction.
func (s *ExternalService) ValidateCompletion(ctx context.Context, st model.State, target string) error {
	for _, id := range model.ActionChecks(st, target) {
		a, ok := model.VerifiedAction(st, target, id)
		if !ok {
			continue
		}
		o, snapshot := s.readObservation(ctx, st, a, false)
		if snapshot == "" || s.Observer.Check(snapshot) != nil {
			return fmt.Errorf("fresh desktop unavailable")
		}
		if status, detail := model.ExternalOutcome(st, a, o, s.now()); status != "matched" {
			return fmt.Errorf("action %s: %s", id, detail)
		}
	}
	return nil
}
