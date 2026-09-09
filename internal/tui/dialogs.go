package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/checks"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
)

type field struct {
	label, value string
	limit        int
}
type dialog struct {
	kind, title, target  string
	generation           int
	state                model.State
	lines                []line
	fields               []field
	field, offset, index int
	busy                 bool
	err                  string
	draft                *draft
	progress             *continuity.ProgressView
	workspace            *workspace.View
	snapshotStatus       *workspace.SnapshotStatus
	workspacePreview     *workspace.Preview
	pending              *savedRequest
	journal              string
	proposal             string
}

func (a *App) newDialog(kind, title, target string) *dialog {
	a.generation++
	d := &dialog{kind: kind, title: title, target: target, generation: a.generation, field: -1, state: a.data.State}
	a.modal = d
	return d
}
func (a *App) openSelected() {
	if a.panel == 0 {
		ns := a.needs()
		if len(ns) == 0 {
			return
		}
		n := ns[a.needIndex]
		switch n.Kind {
		case "decision", "artifact":
			a.openProgress(n)
		case "unsaved":
			a.openDraft(n.Target)
		case "link":
			a.openCapture(n)
		default:
			a.openStep(n.Target, n.ID)
		}
	} else if a.selected != "" {
		a.openStep(a.selected, "")
	}
}
func (a *App) openStep(target, proposal string) {
	d := a.newDialog("step", strings.ReplaceAll(target, "#", " › "), target)
	d.proposal = proposal
	d.busy = true
	a.async("step", target, d.generation, func(ctx context.Context) (any, error) { return fetch(ctx, a.call, target) })
}
func (a *App) stepLines(d *dialog, v snapshot) {
	d.state = v.State
	d.lines = nil
	r, s, err := model.ResolveTarget(v.State, d.target)
	if err != nil {
		d.err = err.Error()
		return
	}
	title, status, done, estimate := r.Task.Title, r.Task.Status, r.Task.Done, r.Task.Estimate
	if s != nil {
		title, status, done, estimate = s.Title, s.Status, s.Done, s.Estimate
	}
	d.lines = append(d.lines, line{{"title       ", gray}, {title, fg}}, line{{"status      ", gray}, {status, statusColor(status)}})
	if estimate != nil {
		d.lines = append(d.lines, plain(fmt.Sprintf("estimate    %dm", *estimate)))
	}
	d.lines = append(d.lines, plain(""), line{{"done when   ", gray}, {done.Text, fg}})
	if d.proposal == "" {
		for _, p := range v.State.Proposals {
			if p.Target == d.target && p.Status == "pending" {
				d.proposal = p.ID
				break
			}
		}
	}
	if d.proposal != "" {
		p := v.State.Proposals[d.proposal]
		d.lines = append(d.lines, line{{"review      ", gray}, {"completion " + p.Status + " · recorded " + age(p.CreatedAt, a.now()), red}})
	}
	results := core.Evaluate(v.State, d.target)
	if len(results) == 0 {
		d.lines = append(d.lines, line{{"checks      ", gray}, {"No configured checks; manual attestation remains separate.", gray}})
	}
	for _, c := range results {
		symbol := "?"
		if c.Status == "matched" {
			symbol = "✓"
		}
		if c.Status == "not_matched" {
			symbol = "×"
		}
		d.lines = append(d.lines, line{{"            " + symbol + " ", checkColor(c.Status)}, {c.ID + " · " + c.Kind + " · ", fg}, {c.Status, checkColor(c.Status)}})
	}
	d.lines = append(d.lines, plain(""), line{{"✓ recorded pass   ? unknown / unsupported   × failed", gray}}, plain(""))
	evidence := []model.Evidence{}
	for _, e := range v.State.Evidence {
		if e.Target == d.target {
			evidence = append(evidence, e)
		}
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].StartedAt.After(evidence[j].StartedAt) })
	for i, e := range evidence {
		if i >= 12 {
			d.lines = append(d.lines, plain("Earlier checks available through heimdall evidence list."))
			break
		}
		status := e.Outcome
		if !model.EvidenceCurrent(v.State, e) {
			status = "stale"
		}
		d.lines = append(d.lines, line{{"evidence    ", gray}, {age(e.StartedAt, a.now()) + " · " + status + " · " + e.Reason, checkColor(status)}})
	}
	if v.DetailError != "" {
		d.lines = append(d.lines, line{{"Files unavailable: " + v.DetailError, red}})
	}
	if v.Resume != nil {
		for _, r := range v.Resume.Resources {
			d.lines = append(d.lines, line{{"file        ", gray}, {r.Resource.Path + " · ", fg}, {r.Status, checkColor(r.Status)}})
		}
	}
	d.lines = append(d.lines, plain(""), line{{"Accept revalidates evidence against current inputs.", gray}}, line{{"Refreshing inspects files; evaluator commands run only from e.", gray}})
}
func (a *App) openProgress(n need) {
	d := a.newDialog("progress", n.Kind+" review · "+n.Target, n.Target)
	d.proposal = n.ID
	d.fields = []field{{"review note", "", 4096}}
	d.busy = true
	a.loadProgress(d)
}
func (a *App) loadProgress(d *dialog) {
	a.async("progress", d.target, d.generation, func(ctx context.Context) (any, error) {
		raw, err := a.call(ctx, "GET", "/state", nil)
		if err != nil {
			return nil, err
		}
		var st model.State
		if err = json.Unmarshal(raw, &st); err != nil {
			return nil, err
		}
		raw, err = a.call(ctx, "GET", "/progress/show?target="+url.QueryEscape(d.target)+"&id="+url.QueryEscape(d.proposal), nil)
		if err != nil {
			return nil, err
		}
		var v continuity.ProgressView
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return struct {
			State model.State
			View  continuity.ProgressView
		}{st, v}, nil
	})
}
func (a *App) progressLines(d *dialog) {
	v := d.progress
	p := v.Proposal
	d.lines = []line{line{{p.Text, fg}}, plain(""), line{{"status      ", gray}, {v.Status + " · " + v.Freshness, checkColor(v.Freshness)}}}
	c := d.state.Contracts[p.ContractID]
	d.lines = append(d.lines, line{{"contract    ", gray}, {c.Objective, fg}}, line{{"done when   ", gray}, {c.Acceptance.Text, fg}})
	for _, s := range c.Constraints {
		d.lines = append(d.lines, line{{"constraint  ", gray}, {s, fg}})
	}
	for _, pin := range p.Artifacts {
		d.lines = append(d.lines, plain(""), line{{"version     ", gray}, {pin.VersionID, fg}}, line{{"sha256      ", gray}, {pin.Observation.Digest, fg}})
	}
	for _, c := range v.Artifacts {
		d.lines = append(d.lines, line{{"file        ", gray}, {c.Record.Path + " · ", fg}, {c.Status, checkColor(c.Status)}})
	}
	if v.Review != nil {
		d.lines = append(d.lines, plain(""), line{{"last review ", gray}, {v.Review.Status + " · " + v.Review.Note, fg}})
	}
	d.lines = append(d.lines, plain(""), line{{"This review does not complete a task or step.", gray}})
}
func (a *App) openDraft(target string) {
	if target == "" {
		return
	}
	d := a.newDialog("draft", "save progress · "+target, target)
	d.busy = true
	a.async("draft", target, d.generation, func(ctx context.Context) (any, error) {
		v, err := fetch(ctx, a.call, target)
		if err != nil {
			return nil, err
		}
		if v.Resume == nil {
			return nil, fmt.Errorf("%s", v.DetailError)
		}
		prepared, err := prepareDraft(a.opts.DataDir, *v.Resume)
		if err != nil {
			return nil, err
		}
		return struct {
			Draft *draft
			View  snapshot
		}{prepared, v}, nil
	})
}
func (a *App) draftFields(d *dialog) {
	cp := d.draft.Request.Checkpoint
	d.fields = []field{{"what changed", cp.Summary, 16384}, {"next", cp.NextAction, 8192}, {"on step", cp.CurrentStep, 64}, {"blockers (;)", strings.Join(cp.Blockers, "; "), 8192}}
	d.field = 0
	if d.draft.Locked {
		d.field = -1
		d.err = "Earlier submission may have committed. Enter retries the unchanged draft."
	}
	d.lines = []line{line{{"A retained draft with a fixed request ID; changes survive closing this view.", gray}}, plain(""), line{{"against     ", gray}, {"Original task revision, contract and checkpoint head", fg}}, line{{"draft       ", gray}, {d.draft.Path, fg}}}
	pins := cp.Artifacts
	if len(pins) == 0 {
		d.lines = append(d.lines, line{{"will pin    ", gray}, {"No artifact pins. Existing resource observations are captured on save.", gray}})
	} else {
		for _, p := range pins {
			d.lines = append(d.lines, line{{"will pin    ", gray}, {p.ArtifactID + " · " + p.VersionID, fg}})
		}
	}
	d.lines = append(d.lines, line{{"Changed or missing pinned versions refuse submission; no automatic repinning.", gray}})
}
func (a *App) persistDraft(d *dialog) error {
	if d.draft == nil || d.draft.Locked {
		return nil
	}
	cp := d.draft.Request.Checkpoint
	cp.Summary = d.fields[0].value
	cp.NextAction = d.fields[1].value
	cp.CurrentStep = d.fields[2].value
	cp.Blockers = []string{}
	for _, s := range strings.Split(d.fields[3].value, ";") {
		if s = strings.TrimSpace(s); s != "" {
			cp.Blockers = append(cp.Blockers, s)
		}
	}
	return writePrivate(d.draft.Path, d.draft.Request, false)
}
func (a *App) openFiles(target string) {
	if target == "" {
		return
	}
	d := a.newDialog("files", "files · "+target, target)
	d.busy = true
	a.async("files", target, d.generation, func(ctx context.Context) (any, error) { return fetch(ctx, a.call, target) })
}
func (a *App) openWorkspace(target string) {
	target = rootOf(target)
	if target == "" {
		return
	}
	d := a.newDialog("workspace", "desktop preview · "+target, target)
	d.busy = true
	a.async("workspace", target, d.generation, func(ctx context.Context) (any, error) { return a.workspaceView(ctx, target) })
}
func (a *App) openBind(target string) {
	target = rootOf(target)
	if target == "" {
		return
	}
	d := a.newDialog("bind", "bind terminal · "+target, target)
	d.busy = true
	a.async("bind", target, d.generation, func(ctx context.Context) (any, error) {
		raw, err := a.call(ctx, "GET", "/workspace/state?target="+url.QueryEscape(target), nil)
		if err != nil {
			return nil, err
		}
		var v workspace.View
		err = json.Unmarshal(raw, &v)
		return v, err
	})
}
func (a *App) openCapture(n need) {
	d := a.newDialog("capture", "file captured link", "")
	d.proposal = n.ID
	d.fields = []field{{"task ID", rootOf(a.selected), 64}}
	d.field = 0
	d.lines = []line{plain(n.Text), plain(""), line{{"Choose an explicit existing workstream. Enter files this capture there.", gray}}}
}
func (a *App) openHelp() {
	d := a.newDialog("help", "heimdall · keys", "")
	d.lines = []line{plain("tab / shift-tab   focus needs you, workstreams, selected context"), plain("j k / arrows      move selection or scroll"), plain("space             expand task steps and child workstreams"), plain("enter             inspect the selected need, task or step"), plain("/                 find tasks and steps; escape clears search"), plain("c                 save progress in a retained draft"), plain("f                 inspect bound files and pinned versions"), plain("p                 preview desired and observed workspace"), plain("b                 bind an explicit terminal surface to a Herdr pane"), plain("r                 refresh state and selected file observations"), plain("v                 toggle compact overview"), plain("q / ctrl-c        quit and restore your terminal"), plain(""), plain("Dialogs: tab edits fields; escape leaves a field, then closes."), plain("Ctrl-s saves/submits a form. Enter retries an uncertain request."), plain("Step checks: a accept, x reject, o reopen, e evaluator picker."), plain("Planning: tab writes a note, then escape and a / x / m to review."), plain("Drafts: e opens $EDITOR; n preserves a conflicted draft and starts fresh."), plain(""), line{{"Recorded agent bindings are not live working/blocked/idle telemetry.", gray}}, line{{"No desktop bar integration or automatic workspace recovery is installed.", gray}}}
}
func (a *App) applyDialog(r result) {
	d := a.modal
	switch r.kind {
	case "step":
		a.stepLines(d, r.value.(snapshot))
	case "progress":
		v := r.value.(struct {
			State model.State
			View  continuity.ProgressView
		})
		if v.View.Proposal.ID != d.proposal || v.View.Proposal.Target != d.target {
			d.err = "Unexpected proposal response"
			return
		}
		d.state = v.State
		d.progress = &v.View
		a.progressLines(d)
	case "draft":
		v := r.value.(struct {
			Draft *draft
			View  snapshot
		})
		d.draft = v.Draft
		d.state = v.View.State
		a.draftFields(d)
	case "files":
		v := r.value.(snapshot)
		d.lines = []line{line{{"Fresh read-only file checks · observations do not lock files", gray}}, plain("")}
		if v.Resume == nil {
			d.err = v.DetailError
			return
		}
		for _, c := range v.Resume.Resources {
			d.lines = append(d.lines, line{{filepath.Join(c.Resource.Root, c.Resource.Path) + " · ", fg}, {c.Status, checkColor(c.Status)}}, line{{c.Detail, gray}})
		}
		for _, c := range v.Resume.Artifacts {
			d.lines = append(d.lines, plain(""), line{{c.Artifact.Name + " · ", fg}, {c.Status, checkColor(c.Status)}}, plain("version "+c.Record.ID), plain("sha256  "+c.Record.Observation.Digest))
		}
		if len(v.Resume.Resources) == 0 {
			d.lines = append(d.lines, plain("No bound resources."))
		}
	case "workspace":
		v := r.value.(workspaceDialogView)
		d.workspace = &v.View
		d.snapshotStatus = &v.Snapshot
		d.workspacePreview = &v.Preview
		d.lines = v.Lines
	case "bind":
		v := r.value.(workspace.View)
		d.workspace = &v
		if v.Manifest == nil {
			d.err = "Accept a desired workspace with terminal surfaces first."
			return
		}
		d.lines = []line{line{{"Select a declared surface, then enter the actual local Herdr socket and pane.", gray}}, plain(""), line{{"The daemon independently verifies the pane, host and working directory.", gray}}}
		d.fields = []field{{"surface ID", "", 32}, {"socket path", "", 4096}, {"pane ID", "", 256}}
		for _, s := range v.Manifest.Surfaces {
			if s.Kind == "terminal" {
				if d.fields[0].value == "" {
					d.fields[0].value = s.ID
				}
				d.lines = append(d.lines, plain(s.Label+" · "+s.ID))
			}
		}
		d.field = 0
	case "mutation":
		message := d.title + " · saved"
		if d.draft != nil {
			if err := d.draft.archive("saved"); err != nil {
				message += "; draft retained: " + err.Error()
			}
		}
		if d.journal != "" {
			if err := os.Rename(d.journal, strings.TrimSuffix(d.journal, ".json")+".done.json"); err != nil {
				message += "; receipt request retained"
			}
		}
		a.message = message
		a.modal = nil
		a.refresh()
	}
}
func (a *App) editKey(e *tcell.EventKey) {
	d := a.modal
	if d.field < 0 || d.field >= len(d.fields) || d.busy || d.pending != nil || (d.draft != nil && d.draft.Locked) {
		return
	}
	f := &d.fields[d.field]
	switch e.Key() {
	case tcell.KeyRune:
		if len(f.value)+len(string(e.Rune())) <= f.limit {
			f.value += string(e.Rune())
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		f.value = backspace(f.value)
	case tcell.KeyCtrlU:
		f.value = ""
	default:
		return
	}
	if d.kind == "draft" {
		if err := a.persistDraft(d); err != nil {
			d.err = "Draft write failed: " + err.Error()
		}
	}
}
func (a *App) dialogKey(e *tcell.EventKey) {
	d := a.modal
	if e.Key() == tcell.KeyEscape {
		if d.field >= 0 {
			d.field = -1
		} else {
			if d.pending != nil {
				a.message = "Review request retained: " + d.journal
			}
			a.modal = nil
		}
		return
	}
	if d.busy {
		return
	}
	if e.Key() == tcell.KeyTab || e.Key() == tcell.KeyBacktab {
		if len(d.fields) > 0 && d.pending == nil {
			if e.Key() == tcell.KeyBacktab {
				d.field--
				if d.field < -1 {
					d.field = len(d.fields) - 1
				}
			} else {
				d.field++
				if d.field >= len(d.fields) {
					d.field = -1
				}
			}
		}
		return
	}
	if e.Key() == tcell.KeyCtrlS {
		a.submitForm(d)
		return
	}
	if d.pending != nil {
		if e.Key() == tcell.KeyEnter {
			a.sendPending(d)
		}
		return
	}
	if d.field >= 0 {
		if e.Key() == tcell.KeyEnter {
			a.submitForm(d)
		} else {
			a.editKey(e)
		}
		return
	}
	if e.Key() == tcell.KeyDown || e.Key() == tcell.KeyPgDn {
		d.offset += 1
		if e.Key() == tcell.KeyPgDn {
			d.offset += 7
		}
		return
	}
	if e.Key() == tcell.KeyUp || e.Key() == tcell.KeyPgUp {
		d.offset = max(0, d.offset-1)
		if e.Key() == tcell.KeyPgUp {
			d.offset = max(0, d.offset-7)
		}
		return
	}
	if e.Key() == tcell.KeyEnter {
		a.submitForm(d)
		return
	}
	if e.Key() != tcell.KeyRune {
		return
	}
	switch e.Rune() {
	case 'j':
		d.offset++
	case 'k':
		d.offset = max(0, d.offset-1)
	case 'r', 'f':
		if d.kind == "progress" {
			d.busy = true
			d.err = ""
			a.loadProgress(d)
		} else if d.kind == "step" {
			a.openStep(d.target, d.proposal)
		} else if d.kind == "files" {
			a.openFiles(d.target)
		} else if d.kind == "workspace" {
			a.openWorkspace(d.target)
		}
	case 'a', 'x', 'm':
		if d.kind == "progress" {
			status := map[rune]string{'a': "accepted", 'x': "rejected", 'm': "reviewed"}[e.Rune()]
			a.reviewProgress(d, status)
		} else if d.kind == "step" && (e.Rune() == 'a' || e.Rune() == 'x') {
			a.reviewCompletion(d, e.Rune() == 'a')
		}
	case 'o':
		if d.kind == "step" {
			a.confirmCore(d, "reopen", d.target)
		} else if d.kind == "workspace" {
			a.confirmWorkspaceOperation(d, "open")
		}
	case 'c':
		if d.kind == "workspace" {
			a.confirmWorkspaceOperation(d, "close")
		}
	case 'e':
		if d.kind == "draft" {
			a.editor(d)
		} else if d.kind == "step" {
			a.evaluators(d)
		}
	case 's':
		if d.kind == "workspace" {
			a.confirmSnapshot(d)
		}
	case 'b':
		if d.kind == "workspace" {
			a.openBind(d.target)
		}
	case 'n':
		if d.kind == "draft" && d.draft != nil {
			if err := d.draft.archive("retained"); err != nil {
				d.err = err.Error()
				return
			}
			a.openDraft(d.target)
		}
	}
}
func (a *App) reviewProgress(d *dialog, status string) {
	if d.progress == nil {
		return
	}
	v := d.progress
	if strings.TrimSpace(d.fields[0].value) == "" {
		d.err = "Add a review note (tab), then escape to return to actions."
		d.field = 0
		return
	}
	if v.Review != nil && (v.Review.Status == "accepted" || v.Review.Status == "rejected") {
		d.err = "This proposal already has a terminal review."
		return
	}
	if status != "rejected" && (v.Freshness == "stale" || v.Status == "superseded") {
		d.err = "Inputs changed. Recheck before reviewing this version."
		return
	}
	previous := v.ReviewHead
	if previous == "" {
		previous = "none"
	}
	task := d.state.Tasks[rootOf(d.target)]
	body := continuity.ProgressRequest{Version: 1, ID: model.NewID(), Target: d.target, ExpectedTaskRevision: task.Revision, Review: &continuity.ProgressReviewInput{ProposalID: d.proposal, Digest: v.Proposal.Digest, Previous: previous, Status: status, Note: d.fields[0].value}}
	a.mutate(d, "/progress/command", body)
}
func (a *App) reviewCompletion(d *dialog, accept bool) {
	if d.proposal == "" {
		d.err = "No pending completion proposal. This view does not create an attestation."
		return
	}
	action := "reject"
	if accept {
		action = "accept"
	}
	revision := d.state.Revision
	body := struct {
		Command core.Command `json:"command"`
	}{core.Command{ID: model.NewID(), Op: "ratify", Target: d.proposal, Action: action, ExpectedRevision: &revision}}
	a.mutate(d, "/commands", body)
}
func (a *App) confirmCore(d *dialog, op, target string) {
	revision := d.state.Revision
	c := core.Command{ID: model.NewID(), Op: op, Target: target, ExpectedRevision: &revision}
	confirm := a.newDialog("confirm", op+" · "+target, target)
	confirm.lines = []line{plain("Enter confirms this explicit task action. Escape cancels.")}
	raw, _ := json.Marshal(struct {
		Command core.Command `json:"command"`
	}{c})
	confirm.pending = &savedRequest{1, target, "/commands", raw}
}
func (a *App) evaluators(d *dialog) {
	defs := []model.Evaluator{}
	for _, e := range d.state.Evaluators {
		if e.Target == d.target && d.state.EvaluatorHeads[model.EvaluatorKey(e.Target, e.CheckID)] == e.ID {
			defs = append(defs, e)
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].CheckID < defs[j].CheckID })
	next := a.newDialog("evaluate", "run configured check · "+d.target, d.target)
	next.state = d.state
	next.fields = []field{{"evaluator ID", "", 32}}
	next.field = 0
	next.lines = []line{line{{"Enter explicitly runs this configured evaluator, which may execute a command.", gold}}, plain("")}
	for _, e := range defs {
		if next.fields[0].value == "" {
			next.fields[0].value = e.ID
		}
		next.lines = append(next.lines, plain(e.CheckID+" · "+e.Spec.Kind+" · "+e.ID))
	}
	if len(defs) == 0 {
		next.err = "No configured evaluators for this target."
	}
}
func (a *App) submitForm(d *dialog) {
	switch d.kind {
	case "draft":
		if d.draft == nil {
			return
		}
		if err := a.persistDraft(d); err != nil {
			d.err = err.Error()
			return
		}
		raw, _ := json.Marshal(d.draft.Request)
		if _, err := continuity.Decode(raw); err != nil {
			d.err = err.Error()
			return
		}
		if err := writePrivate(d.draft.Path+".attempt", map[string]string{"request_id": d.draft.Request.ID}, false); err != nil {
			d.err = err.Error()
			return
		}
		d.draft.Locked = true
		a.mutate(d, "/continuity/command", d.draft.Request)
	case "bind":
		if d.workspace == nil || len(d.fields) < 3 {
			return
		}
		surface := d.fields[0].value
		previous := "none"
		for _, b := range d.workspace.Bindings {
			if b.SurfaceID == surface && b.Head != "" {
				previous = b.Head
			}
		}
		body := workspace.HerdrBindRequest{Version: 1, ID: model.NewID(), Target: d.target, ExpectedTaskRevision: d.workspace.TaskRevision, ManifestID: d.workspace.ManifestHead, SurfaceID: surface, Previous: previous, Socket: d.fields[1].value, PaneID: d.fields[2].value}
		if err := body.Validate(); err != nil {
			d.err = err.Error()
			return
		}
		a.mutate(d, "/workspace/herdr/bind", body)
	case "capture":
		target := d.fields[0].value
		if _, ok := d.state.Tasks[target]; !ok {
			d.err = "Choose an existing task ID."
			return
		}
		revision := d.state.Revision
		body := struct {
			Command core.Command `json:"command"`
		}{core.Command{ID: model.NewID(), Op: "assign", Target: d.proposal, Targets: []string{target}, ExpectedRevision: &revision}}
		a.mutate(d, "/commands", body)
	case "evaluate":
		body := checks.Request{Version: 1, ID: model.NewID(), Target: d.target, ExpectedTaskRevision: d.state.Tasks[rootOf(d.target)].Revision, EvaluatorID: d.fields[0].value}
		a.mutate(d, "/evidence/evaluate", body)
	}
}
func (a *App) mutate(d *dialog, path string, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		d.err = err.Error()
		return
	}
	d.pending = &savedRequest{1, d.target, path, raw}
	a.sendPending(d)
}
func (a *App) sendPending(d *dialog) {
	if d.pending == nil {
		return
	}
	if d.journal == "" {
		d.journal = filepath.Join(a.opts.DataDir, "tui-requests", model.NewID()+".json")
		if err := writePrivate(d.journal, d.pending, true); err != nil {
			d.journal = ""
			d.err = "Cannot retain retry request: " + err.Error()
			return
		}
	}
	d.busy = true
	d.err = ""
	request := *d.pending
	a.async("mutation", d.target, d.generation, func(ctx context.Context) (any, error) {
		_, err := a.call(ctx, "POST", request.Path, request.Body)
		if err != nil {
			return nil, fmt.Errorf("%w · retry file: %s", err, d.journal)
		}
		return nil, nil
	})
}
func (a *App) editor(d *dialog) {
	if d.draft == nil || d.draft.Locked {
		d.err = "This draft has been submitted; retry it unchanged or start a new retained draft."
		return
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	if err := a.persistDraft(d); err != nil {
		d.err = err.Error()
		return
	}
	if err := a.screen.Suspend(); err != nil {
		d.err = err.Error()
		return
	}
	cmd := exec.CommandContext(a.ctx, editor, d.draft.Path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	resumeErr := a.screen.Resume()
	if err != nil {
		d.err = "Editor must name an executable: " + err.Error()
		return
	}
	if resumeErr != nil {
		d.err = resumeErr.Error()
		return
	}
	if err = d.draft.reload(); err != nil {
		d.err = err.Error()
		return
	}
	a.draftFields(d)
}

// OpenRequest resumes a retained mutation only after an explicit Enter in the TUI.
func (a *App) OpenRequest(path string) error {
	var r savedRequest
	if err := readBounded(path, &r); err != nil {
		return err
	}
	if r.Version != 1 || !model.Contains([]string{"/commands", "/progress/command", "/continuity/command", "/workspace/herdr/bind", "/workspace/snapshot/command", "/workspace/operation/queue", "/evidence/evaluate"}, r.Path) || !json.Valid(r.Body) {
		return errors.New("invalid retained TUI request")
	}
	d := a.newDialog("confirm", "retry retained request · "+r.Target, r.Target)
	d.pending = &r
	d.journal = path
	d.lines = []line{plain("Enter resends the original request. Escape cancels."), plain(path), plain(string(r.Body))}
	return nil
}
