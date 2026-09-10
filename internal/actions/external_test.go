package actions_test

import (
	"context"
	"encoding/json"
	"errors"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/authz"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/testdesktop"
	"heimdall/internal/workspace"
	"reflect"
	"strings"
	"testing"
	"time"
)

type externalFixture struct {
	*fixture
	service      *actions.ExternalService
	fake         *testdesktop.Fake
	token, grant string
	request      actions.IntentRequest
}

func externalSetup(t *testing.T, actionID ...string) *externalFixture {
	done := []model.Done{}
	if len(actionID) > 0 {
		done = append(done, model.Done{Text: "Verify fixture focus", Checks: []model.Check{{ID: "focus", Kind: "action.verified", ActionID: actionID[0]}}})
	}
	f := setup(t, done...)
	f.surface = model.NewID()
	raw, err := (workspace.Service{Store: f.e.Store}).Execute(f.ctx, workspace.Request{Version: 1, ID: model.NewID(), Op: "workspace.accept", Target: "alpha", ExpectedTaskRevision: 1, Manifest: &workspace.ManifestInput{Previous: f.manifest, Name: "Native input", Surfaces: []model.DesiredSurface{{ID: f.surface, Kind: "native", Label: "Fixture", Required: true, RestorePolicy: "manual"}}}}, "cli", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var manifest model.WorkspaceManifest
	json.Unmarshal(raw, &manifest)
	f.manifest = manifest.ID
	fake := testdesktop.New()
	fake.Set("activewindow", testdesktop.Window("18000001", "0x64", 1, "planning"))
	observer := hyprland.New()
	observer.Connector = fake.Connect
	t.Cleanup(observer.Close)
	viewport := workspace.ViewportService{Store: f.e.Store, Observer: observer}
	source := workspace.ViewportRequest{Version: 1, ID: model.NewID(), Op: "select", Previous: "none", Source: &workspace.SourceInput{SocketDir: "/synthetic/hypr", Host: strings.Repeat("c", 64), Epoch: strings.Repeat("a", 64)}}
	if _, err := viewport.Execute(f.ctx, source, "cli", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	read, err := observer.Read(f.ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	bind := workspace.ViewportRequest{Version: 1, ID: model.NewID(), Op: "bind", Previous: "none", Target: "alpha", ExpectedTaskRevision: 1, Binding: &workspace.ViewportInput{ManifestID: f.manifest, SurfaceID: f.surface, SourceID: source.ID, SnapshotID: read.Snapshot.ID, Window: &read.Snapshot.Windows[0].Identity}}
	if _, err := viewport.Execute(f.ctx, bind, "cli", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	x := &externalFixture{fixture: f, fake: fake, token: strings.Repeat("d", 64), grant: model.NewID(), service: &actions.ExternalService{Store: f.e.Store, Observer: observer}}
	if _, err := (authz.Service{Store: f.e.Store}).Execute(f.ctx, authz.Request{Version: 1, ID: x.grant, Op: "grant.issue", Grant: &authz.IssueInput{Name: "WCU fixture", Target: "alpha", ActionWrite: true, TokenHash: authz.HashToken(x.token), ExpiresAt: time.Now().Add(time.Hour)}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	x.request = actions.IntentRequest{Version: 1, ID: model.NewID(), Target: "alpha", Purpose: "Observe fixture focus", Expected: model.ActionPostcondition{Kind: "window_focused"}, Steps: 2}
	if len(actionID) > 0 {
		x.request.ID = actionID[0]
	}
	return x
}
func (x *externalFixture) register() model.ActionRecord {
	x.t.Helper()
	raw, err := x.service.Register(x.ctx, x.request, x.token)
	if err != nil {
		x.t.Fatal(err)
	}
	var result actions.Registration
	if err = json.Unmarshal(raw, &result); err != nil {
		x.t.Fatal(err)
	}
	if result.Window.Address != "0x64" || result.Workspace != "planning" {
		x.t.Fatal("missing exact fresh container", string(raw))
	}
	return result.Action
}
func (x *externalFixture) report(step int) actions.ReportRequest {
	return actions.ReportRequest{Version: 1, ID: model.NewID(), Target: "alpha", IntentID: x.request.ID, Step: step, Outcome: "succeeded", WCURequestID: "fixture-request"}
}
func (x *externalFixture) replay() {
	x.t.Helper()
	before, _ := x.e.Store.State(x.ctx)
	after, err := x.e.Store.Replay(x.ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		x.t.Fatal("external replay", err)
	}
}
func TestExternalReportsRequireIndependentObservationAndReplay(t *testing.T) {
	x := externalSetup(t)
	a := x.register()
	if a.Execution != "external" || a.Verification != "pending" {
		t.Fatal(a)
	}
	bad := x.report(2)
	if _, err := x.service.Report(x.ctx, bad, x.token); err == nil {
		t.Fatal("out of order report accepted")
	}
	r := x.report(1)
	first, err := x.service.Report(x.ctx, r, x.token)
	if err != nil {
		t.Fatal(err)
	}
	again, err := x.service.Report(x.ctx, r, x.token)
	if err != nil || string(first) != string(again) {
		t.Fatal("retry changed result", err)
	}
	r.Outcome = "failed"
	if _, err := x.service.Report(x.ctx, r, x.token); err == nil {
		t.Fatal("changed retry accepted")
	}
	if _, err = x.service.Report(x.ctx, x.report(2), x.token); err != nil {
		t.Fatal(err)
	}
	a = x.action(a.Intent.ID)
	if a.Verification != "pending" || len(a.Reports) != 2 || !a.FinalReport {
		t.Fatal(a)
	}
	if err = x.service.Observe(x.ctx, a.Intent.ID); err != nil {
		t.Fatal(err)
	}
	a = x.action(a.Intent.ID)
	if a.Verification != "matched" || a.Observation.Native == nil || a.Observation.Digest != model.ContentDigest(a.Observation.Native) {
		t.Fatal(a)
	}
	if _, err = x.service.Report(x.ctx, x.report(2), x.token); err == nil {
		t.Fatal("extra final accepted")
	}
	x.replay()
}
func TestExternalRevocationRefusesCachedAndNewReportsButReconciles(t *testing.T) {
	x := externalSetup(t)
	a := x.register()
	r := x.report(1)
	if _, err := x.service.Report(x.ctx, r, x.token); err != nil {
		t.Fatal(err)
	}
	if _, err := (authz.Service{Store: x.e.Store}).Execute(x.ctx, authz.Request{Version: 1, ID: model.NewID(), Op: "grant.revoke", GrantID: x.grant}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, report := range []actions.ReportRequest{r, x.report(2)} {
		if _, err := x.service.Report(x.ctx, report, x.token); !errors.Is(err, authz.ErrDenied) {
			t.Fatal("revocation bypass", err)
		} else {
			var refusal *actions.Refusal
			if !errors.As(err, &refusal) {
				t.Fatal("missing refusal receipt", err)
			}
			request, _ := json.Marshal(report)
			saved, found, readErr := x.e.Store.CommandReceipt(x.ctx, "external-refusal-"+x.grant+"-"+report.ID, request)
			if readErr != nil || !found || string(saved) != string(refusal.Receipt) {
				t.Fatal("refusal was not durable", readErr)
			}
		}
	}
	if err := x.service.Settle(x.ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := x.service.Observe(x.ctx, a.Intent.ID); err != nil {
		t.Fatal(err)
	}
	a = x.action(a.Intent.ID)
	if !a.CancelRequested || a.Execution != "uncertain" || a.Verification != "matched" || len(a.Reports) != 1 {
		t.Fatal(a)
	}
	x.replay()
}
func TestExternalInterruptedAndExpiredNeverDispatch(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "restart"}[restart], func(t *testing.T) {
			x := externalSetup(t)
			a := x.register()
			if !restart {
				x.service.Clock = func() time.Time { return a.Intent.ExpiresAt }
			}
			if err := x.service.Settle(x.ctx, restart); err != nil {
				t.Fatal(err)
			}
			a = x.action(a.Intent.ID)
			if a.Execution != "uncertain" || a.FinalReport || len(a.Reports) != 0 {
				t.Fatal(a)
			}
			if _, err := x.service.Report(x.ctx, x.report(1), x.token); err == nil {
				t.Fatal("report accepted after interruption")
			}
			before, _ := x.e.Store.State(x.ctx)
			if err := x.service.Settle(x.ctx, false); err != nil {
				t.Fatal(err)
			}
			after, _ := x.e.Store.State(x.ctx)
			if before.LastEventID != after.LastEventID {
				t.Fatal("idle sweep grew log")
			}
			x.replay()
		})
	}
}
func TestExternalScopeAndObservationAreNotInferred(t *testing.T) {
	x := externalSetup(t)
	r := x.request
	r.Target = "beta"
	if _, err := x.service.Register(x.ctx, r, x.token); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("cross-task scope", err)
	}
	x.fake.Set("activewindow", testdesktop.Window("18000002", "0x65", 1, "planning"))
	if _, err := x.service.Register(x.ctx, x.request, x.token); err == nil {
		t.Fatal("unowned focused window admitted")
	}
	x.request.SurfaceID = x.surface
	a := x.register()
	if _, err := x.service.Report(x.ctx, x.report(1), x.token); err != nil {
		t.Fatal(err)
	}
	if _, err := x.service.Report(x.ctx, x.report(2), x.token); err != nil {
		t.Fatal(err)
	}
	if err := x.service.Observe(x.ctx, a.Intent.ID); err != nil {
		t.Fatal(err)
	}
	if got := x.action(a.Intent.ID).Verification; got != "not_matched" {
		t.Fatal("report overrode observed focus", got)
	}
	x.fake.ChangeEpoch()
	if err := x.service.Observe(x.ctx, a.Intent.ID); err != nil {
		t.Fatal(err)
	}
	if got := x.action(a.Intent.ID).Verification; got != "unknown" {
		t.Fatal("lost epoch verified", got)
	}
	x.replay()
}

func TestExternalObservationRejectsFuturePartialAndForgedReadback(t *testing.T) {
	x := externalSetup(t)
	a := x.register()
	if _, err := x.service.Report(x.ctx, x.report(1), x.token); err != nil {
		t.Fatal(err)
	}
	if _, err := x.service.Report(x.ctx, x.report(2), x.token); err != nil {
		t.Fatal(err)
	}
	before, _ := x.e.Store.State(x.ctx)
	pending := before.Actions[a.Intent.ID]
	if err := x.service.Observe(x.ctx, a.Intent.ID); err != nil {
		t.Fatal(err)
	}
	observed := x.action(a.Intent.ID)
	now := observed.UpdatedAt
	if status, _ := model.ExternalOutcome(before, pending, observed.Observation, now); status != "matched" {
		t.Fatal(status)
	}
	for _, kind := range []string{"future", "partial", "wrong_window", "old_cursor", "old_start", "forged_digest", "future_received"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(observed.Observation)
			var o model.ActionObservation
			json.Unmarshal(raw, &o)
			switch kind {
			case "future":
				o.Native.CapturedAt = now.Add(time.Second)
				o.ObservedAt = o.Native.CapturedAt
			case "partial":
				o.Native.Complete = false
			case "wrong_window":
				o.Native.Window.Identity.StableID = "18000999"
			case "old_cursor":
				o.Native.AfterEventID = 0
			case "old_start":
				o.Native.StartedAt = pending.UpdatedAt.Add(-time.Nanosecond)
			case "future_received":
				o.Native.ReceivedAt = now.Add(time.Second)
			}
			o.Digest = model.ContentDigest(o.Native)
			if kind == "forged_digest" {
				o.Digest = strings.Repeat("0", 64)
			}
			if status, _ := model.ExternalOutcome(before, pending, &o, now); status != "unknown" {
				t.Fatal("invalid observation verified", status)
			}
		})
	}
}

func TestExternalVerifiedCheckpointRoundTrip(t *testing.T) {
	x := externalSetup(t)
	service := continuity.Service{Store: x.e.Store}
	revision := int64(1)
	contract := model.NewID()
	if _, err := service.Execute(x.ctx, continuity.Request{Version: 1, ID: contract, Op: "contract.accept", Target: "alpha", ExpectedTaskRevision: &revision, Contract: &continuity.ContractInput{Previous: "none", Objective: "Observe fixture focus"}}, "cli", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	a := x.register()
	cp := continuity.Request{Version: 3, ID: model.NewID(), Op: "checkpoint.record", Target: "alpha", ExpectedTaskRevision: &revision, Checkpoint: &continuity.CheckpointInput{Previous: "none", ContractID: contract, Summary: "Fixture focus recorded", NextAction: "Review observation", Actions: []string{a.Intent.ID}}}
	if _, err := service.Execute(x.ctx, cp, "cli", time.Now().UTC()); err == nil {
		t.Fatal("unobserved action cited")
	}
	for i := 1; i <= 2; i++ {
		if _, err := x.service.Report(x.ctx, x.report(i), x.token); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.service.Observe(x.ctx, a.Intent.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := service.Execute(x.ctx, cp, "cli", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var saved model.Checkpoint
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version != 4 || len(saved.Actions) != 1 || saved.Actions[0] != a.Intent.ID {
		t.Fatal(saved)
	}
	if _, err = service.ExecuteClient(x.ctx, cp, x.token, time.Now); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("action grant wrote checkpoint", err)
	}
	cp.ID = model.NewID()
	cp.Checkpoint.Previous = saved.ID
	token := strings.Repeat("e", 64)
	grant := model.NewID()
	if _, err := (authz.Service{Store: x.e.Store}).Execute(x.ctx, authz.Request{Version: 1, ID: grant, Op: "grant.issue", Grant: &authz.IssueInput{Name: "Checkpoint writer", Target: "alpha", CheckpointWrite: true, TokenHash: authz.HashToken(token), ExpiresAt: time.Now().Add(time.Hour)}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	raw, err = service.ExecuteClient(x.ctx, cp, token, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.ExecuteClient(x.ctx, cp, token, time.Now)
	if err != nil || string(raw) != string(again) {
		t.Fatal("checkpoint retry", err)
	}
	x.replay()
}

func TestExternalCompletionCitesObservationAndRevalidates(t *testing.T) {
	id := model.NewID()
	x := externalSetup(t, id)
	x.e.ValidateEvidence = x.service.ValidateCompletion
	a := x.register()
	for i := 1; i <= 2; i++ {
		if _, err := x.service.Report(x.ctx, x.report(i), x.token); err != nil {
			t.Fatal(err)
		}
	}
	st, _ := x.e.Store.State(x.ctx)
	if got := core.Evaluate(st, "alpha")[0]; got.Status == "matched" {
		t.Fatal("report became evidence")
	}
	if err := x.service.Observe(x.ctx, id); err != nil {
		t.Fatal(err)
	}
	a = x.action(id)
	if _, err := x.e.Execute(x.ctx, core.Command{ID: model.NewID(), Op: "tick"}, "scheduler", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	st, _ = x.e.Store.State(x.ctx)
	var proposal model.Proposal
	for _, p := range st.Proposals {
		if p.Target == "alpha" && p.Status == "pending" {
			proposal = p
		}
	}
	if proposal.ID == "" || !strings.Contains(strings.Join(proposal.Evidence, " "), a.Observation.ID) {
		t.Fatal("missing observation citation", proposal)
	}
	x.fake.Set("activewindow", testdesktop.Window("18000002", "0x65", 1, "planning"))
	cmd := core.Command{ID: model.NewID(), Op: "ratify", Target: proposal.ID, Action: "accept"}
	if _, err := x.e.Execute(x.ctx, cmd, "cli", time.Now().UTC()); err == nil {
		t.Fatal("drifted focus completed task")
	}
	x.fake.Set("activewindow", testdesktop.Window("18000001", "0x64", 1, "planning"))
	if _, err := x.e.Execute(x.ctx, cmd, "cli", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	st, _ = x.e.Store.State(x.ctx)
	if st.Tasks["alpha"].Task.Status != "done" {
		t.Fatal("not completed")
	}
	x.replay()
}

func TestExternalCorroborationMismatchNeverChangesBindingOrVerification(t *testing.T) {
	x := externalSetup(t)
	a := x.register()
	before, _ := x.e.Store.State(x.ctx)
	x.service.Corroborate = func(context.Context, model.DesktopWindow, model.ActionRecord) (*model.ExternalObservation, error) {
		return &model.ExternalObservation{Source: "wcu", Revision: "fixture:1", Freshness: false, Partial: true, Digest: model.ContentDigest("partial"), ReportsDigest: model.ContentDigest("mismatched-report"), ReportCount: 1, TargetMismatch: true}, nil
	}
	for i := 1; i <= 2; i++ {
		if _, err := x.service.Report(x.ctx, x.report(i), x.token); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.service.Observe(x.ctx, a.Intent.ID); err != nil {
		t.Fatal(err)
	}
	a = x.action(a.Intent.ID)
	if a.Verification != "matched" || a.LastReason != "target_mismatch" || a.Observation.External == nil {
		t.Fatal(a)
	}
	after, _ := x.e.Store.State(x.ctx)
	if !reflect.DeepEqual(before.ViewportBindings, after.ViewportBindings) {
		t.Fatal("WCU changed binding")
	}
	x.replay()
}

func TestExternalCancelByIntentRetainsRetryAfterReconciliation(t *testing.T) {
	x := externalSetup(t)
	x.register()
	r := actions.CancelRequest{Version: 2, ID: model.NewID(), ActionID: x.request.ID, Reason: "Operator cancelled external reporting"}
	service := actions.Service{Store: x.e.Store}
	first, err := service.Cancel(x.ctx, r, "cli", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.service.Report(x.ctx, x.report(1), x.token); !errors.Is(err, authz.ErrDenied) {
		t.Fatalf("cancelled report accepted: %v", err)
	}
	if err = x.service.Observe(x.ctx, x.request.ID); err != nil {
		t.Fatal(err)
	}
	retry, err := service.Cancel(x.ctx, r, "cli", time.Now().UTC())
	if err != nil || string(first) != string(retry) {
		t.Fatalf("retry: %v", err)
	}
	r.ID = model.NewID()
	r.ActionID = model.NewID()
	if _, err = service.Cancel(x.ctx, r, "cli", time.Now().UTC()); err == nil {
		t.Fatal("unknown intent accepted")
	}
	x.replay()
}

func TestExternalFinalReportSurvivesGrantExpiry(t *testing.T) {
	x := externalSetup(t)
	x.register()
	for i := 1; i <= 2; i++ {
		if _, err := x.service.Report(x.ctx, x.report(i), x.token); err != nil {
			t.Fatal(err)
		}
	}
	st, _ := x.e.Store.State(x.ctx)
	x.service.Clock = func() time.Time { return st.Grants[x.grant].ExpiresAt.Add(time.Second) }
	if err := x.service.Settle(x.ctx, false); err != nil {
		t.Fatal(err)
	}
	st, _ = x.e.Store.State(x.ctx)
	if a := st.Actions[x.request.ID]; a.Execution != "api_reported" || a.CancelRequested {
		t.Fatalf("final report changed: %+v", a)
	}
	x.replay()
}
