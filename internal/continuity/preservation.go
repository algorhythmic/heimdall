package continuity

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/adapters/dotprivate"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

type PreservationObserveInput struct {
	PlanID          string `json:"plan_id"`
	Previous        string `json:"previous"`
	CheckRemote     bool   `json:"check_remote"`
	ReportedOutcome string `json:"reported_outcome"`
	Note            string `json:"note"`
}
type PreservationRequest struct {
	Version              int                       `json:"version"`
	ID                   string                    `json:"id"`
	Target               string                    `json:"target"`
	ExpectedTaskRevision int64                     `json:"expected_task_revision"`
	Plan                 *model.PreservationInput  `json:"plan,omitempty"`
	PreviewDigest        string                    `json:"preview_digest,omitempty"`
	Observe              *PreservationObserveInput `json:"observe,omitempty"`
}

func (r PreservationRequest) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || r.ExpectedTaskRevision < 1 || (r.Plan == nil) == (r.Observe == nil) {
		return fmt.Errorf("preservation requires version 1, stable ID, task revision and exactly one payload")
	}
	if r.Plan != nil {
		if r.PreviewDigest != "" && !model.ValidPreservationDigest(r.PreviewDigest) {
			return fmt.Errorf("invalid preview digest")
		}
		return r.Plan.Validate()
	}
	o := r.Observe
	if _, err := head(o.Previous); err != nil {
		return err
	}
	if r.PreviewDigest != "" || !model.OpaqueID.MatchString(o.PlanID) || !model.ValidPreservationReport(o.ReportedOutcome, o.Note) {
		return fmt.Errorf("invalid observation request")
	}
	return nil
}
func DecodePreservation(raw []byte) (PreservationRequest, error) {
	var r PreservationRequest
	if len(raw) > MaxRequest {
		return r, fmt.Errorf("preservation request exceeds 64 KiB")
	}
	if err := model.StrictJSON(raw, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}

type PreservationHandoff struct {
	Mode    string   `json:"mode"`
	Sources []string `json:"sources"`
	Issues  []string `json:"issues"`
}
type PreservationPreview struct {
	Target       string                     `json:"target"`
	TaskRevision int64                      `json:"task_revision"`
	Host         string                     `json:"host"`
	Input        model.PreservationInput    `json:"input"`
	Snapshot     model.PreservationSnapshot `json:"snapshot"`
	Digest       string                     `json:"digest"`
	Handoff      PreservationHandoff        `json:"handoff"`
}

func preservationRead(ctx context.Context, root, path string, expected model.ArtifactObservation) model.PreservedFile {
	f := model.PreservedFile{Status: "refused"}
	if dotprivate.Refused(path) {
		return f
	}
	r := model.Resource{Active: true, Kind: "tree", Root: root, Path: "."}
	o, err := ObserveArtifact(ctx, r, path, false)
	if err != nil {
		if os.IsNotExist(err) {
			f.Status = "missing"
		} else if strings.Contains(err.Error(), "changed") || strings.Contains(err.Error(), "replaced") {
			f.Status = "changed_during_observation"
		} else if !strings.Contains(err.Error(), "symlink") && !strings.Contains(err.Error(), "regular file") {
			f.Status = "unavailable"
		}
		return f
	}
	f = model.PreservedFile{Status: "content_changed", Digest: o.Digest, Bytes: o.Bytes}
	if o.Digest == expected.Digest && o.Bytes == expected.Bytes {
		f.Status = "matched"
	}
	return f
}
func preservationSource(ctx context.Context, st model.State, target string, sel model.PreservationSelection) (model.PreservedFile, string) {
	v := st.ArtifactVersions[sel.VersionID]
	a := st.Artifacts[sel.ArtifactID]
	f := model.PreservedFile{Status: "scope_changed"}
	host, err := artifactHost()
	if err != nil || host != a.Host {
		f.Status = "host_changed"
		return f, ""
	}
	scope, err := model.ResourceScope(st, target)
	if err != nil || !model.Contains(scope, v.ResourceID) {
		return f, ""
	}
	r, err := model.ArtifactResource(st.Resources[v.ResourceID], v.Path)
	if err != nil {
		return f, ""
	}
	return preservationRead(ctx, r.Root, r.Path, v.Observation), filepath.Join(r.Root, r.Path)
}
func preservationSnapshot(ctx context.Context, st model.State, target string, in model.PreservationInput, remote bool, fingerprint string) (model.PreservationSnapshot, []string) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	items := []model.PreservationItem{}
	sources := []string{}
	expected := map[string]model.ArtifactObservation{}
	for _, sel := range in.Selections {
		v := st.ArtifactVersions[sel.VersionID]
		source, path := preservationSource(ctx, st, target, sel)
		sources = append(sources, path)
		items = append(items, model.PreservationItem{ArtifactID: sel.ArtifactID, VersionID: sel.VersionID, ExpectedDigest: v.Observation.Digest, Source: source})
		expected[sel.MirrorPath] = v.Observation
	}
	read := func(ctx context.Context, root, path string) model.PreservedFile {
		return preservationRead(ctx, root, path, expected[path])
	}
	s := dotprivate.Observe(ctx, in, items, read, remote, fingerprint)
	for i, sel := range in.Selections {
		source, _ := preservationSource(ctx, st, target, sel)
		mirror := read(ctx, in.PrivateRoot, sel.MirrorPath)
		if !reflect.DeepEqual(source, s.Items[i].Source) {
			s.Items[i].Source = model.PreservedFile{Status: "changed_during_observation"}
		}
		if !reflect.DeepEqual(mirror, s.Items[i].Mirror) {
			s.Items[i].Mirror = model.PreservedFile{Status: "changed_during_observation"}
		}
	}
	s.Stage = s.DeriveStage()
	return s, sources
}
func preservationPreview(ctx context.Context, st model.State, r PreservationRequest) (PreservationPreview, error) {
	p := PreservationPreview{}
	if r.Plan == nil || r.Observe != nil {
		return p, fmt.Errorf("preview requires plan payload")
	}
	t, _, err := model.ResolveTarget(st, r.Target)
	if err != nil {
		return p, err
	}
	if t.Revision != r.ExpectedTaskRevision {
		return p, store.ErrConflict
	}
	if err := model.PreservationPins(st, r.Target, *r.Plan); err != nil {
		return p, err
	}
	host, err := artifactHost()
	if err != nil {
		return p, err
	}
	snapshot, sources := preservationSnapshot(ctx, st, r.Target, *r.Plan, false, "")
	p = PreservationPreview{Target: r.Target, TaskRevision: t.Revision, Host: host, Input: *r.Plan, Snapshot: snapshot, Handoff: PreservationHandoff{Mode: "manual_only", Sources: sources, Issues: []string{"dotprivate ingest may stage every change in the private clone; inspect its dry run and complete the operation manually"}}}
	if snapshot.UnrelatedCount > 0 {
		p.Handoff.Issues = append(p.Handoff.Issues, "unrelated dirty/staged paths: selected-file handoff refused until reconciled")
	}
	if snapshot.RepositoryStatus != "available" {
		p.Handoff.Issues = append(p.Handoff.Issues, "private repository is "+snapshot.RepositoryStatus)
	}
	if snapshot.Ref != r.Plan.Ref {
		p.Handoff.Issues = append(p.Handoff.Issues, "private checkout branch differs from selected publication ref")
	}
	for _, i := range snapshot.Items {
		if i.Source.Status != "matched" {
			p.Handoff.Issues = append(p.Handoff.Issues, "source "+i.ArtifactID+" is "+i.Source.Status)
		}
	}
	p.Digest = model.PreservationDigest([]any{p.Target, p.TaskRevision, p.Host, p.Input, p.Snapshot})
	return p, nil
}
func (s Service) PreservationPreview(ctx context.Context, r PreservationRequest) (PreservationPreview, error) {
	if err := r.Validate(); err != nil {
		return PreservationPreview{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return PreservationPreview{}, err
	}
	p, err := preservationPreview(ctx, st, r)
	if err != nil {
		return p, err
	}
	after, err := s.Store.State(ctx)
	if err == nil && st.LastEventID != after.LastEventID {
		err = store.ErrConflict
	}
	return p, err
}
func (s Service) Preservation(ctx context.Context, r PreservationRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("preservation requires CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(r)
	if len(raw) > MaxRequest {
		return nil, fmt.Errorf("preservation request exceeds 64 KiB")
	}
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	// An exact retry returns its recorded observation even if the clone vanished.
	if st.PreservationPlans[r.ID].ID != "" || st.PreservationReceipts[r.ID].ID != "" {
		return s.Store.Transact(ctx, "preservation-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) { return store.Change{}, store.ErrConflict })
	}
	var event store.Pending
	if r.Plan != nil {
		if r.PreviewDigest == "" {
			return nil, fmt.Errorf("request requires the reviewed preview_digest")
		}
		p, err := preservationPreview(ctx, st, r)
		if err != nil {
			return nil, err
		}
		if p.Digest != r.PreviewDigest {
			return nil, fmt.Errorf("preservation preview changed: %w", store.ErrConflict)
		}
		plan := model.PreservationPlan{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, Host: p.Host, Input: *r.Plan, PreviewDigest: p.Digest, Snapshot: p.Snapshot, Actor: actor, At: now.UTC()}
		event = store.Pending{Subject: "preservation", Verb: "requested", EntityID: r.ID, Payload: plan}
	} else {
		p, ok := st.PreservationPlans[r.Observe.PlanID]
		if !ok || p.Target != r.Target {
			return nil, fmt.Errorf("preservation plan not found for target")
		}
		host, err := artifactHost()
		if err != nil || host != p.Host {
			return nil, fmt.Errorf("preservation requires the recorded host")
		}
		prev, _ := head(r.Observe.Previous)
		if st.PreservationHeads[p.ID] != prev {
			return nil, store.ErrConflict
		}
		snapshot, _ := preservationSnapshot(ctx, st, r.Target, p.Input, r.Observe.CheckRemote, p.Snapshot.RemoteFingerprint)
		receipt := model.PreservationReceipt{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, PlanID: p.ID, Previous: prev, ReportedOutcome: r.Observe.ReportedOutcome, Note: r.Observe.Note, Snapshot: snapshot, Actor: actor, At: now.UTC()}
		event = store.Pending{Subject: "preservation", Verb: "observed", EntityID: r.ID, Payload: receipt}
	}
	return s.Store.Transact(ctx, "preservation-"+r.ID, actor, raw, now, func(current model.State) (store.Change, error) {
		c := store.Change{Revision: current.Revision}
		t, _, err := model.ResolveTarget(current, r.Target)
		if err != nil || t.Revision != r.ExpectedTaskRevision || current.LastEventID != st.LastEventID {
			return c, fmt.Errorf("state changed during preservation observation: %w", store.ErrConflict)
		}
		c.Events = []store.Pending{event}
		c.Result = event.Payload
		return c, nil
	})
}

type PreservationView struct {
	Plan    model.PreservationPlan     `json:"plan"`
	Head    string                     `json:"head"`
	Receipt *model.PreservationReceipt `json:"receipt,omitempty"`
}

func (s Service) PreservationShow(ctx context.Context, target, id string) (PreservationView, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return PreservationView{}, err
	}
	p, ok := st.PreservationPlans[id]
	if !ok || p.Target != target {
		return PreservationView{}, fmt.Errorf("preservation plan not found for target")
	}
	v := PreservationView{Plan: p, Head: st.PreservationHeads[id]}
	if v.Head != "" {
		r := st.PreservationReceipts[v.Head]
		v.Receipt = &r
	}
	return v, nil
}
func (s Service) PreservationList(ctx context.Context, target, after string, limit int) ([]PreservationView, error) {
	if limit < 1 || limit > 50 || (after != "" && !model.OpaqueID.MatchString(after)) {
		return nil, fmt.Errorf("limit 1..50 and optional after ID required")
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if _, _, err = model.ResolveTarget(st, target); err != nil {
		return nil, err
	}
	ids := []string{}
	for id, p := range st.PreservationPlans {
		if p.Target == target && id > after {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := []PreservationView{}
	for _, id := range ids {
		v := PreservationView{Plan: st.PreservationPlans[id], Head: st.PreservationHeads[id]}
		if v.Head != "" {
			r := st.PreservationReceipts[v.Head]
			v.Receipt = &r
		}
		out = append(out, v)
	}
	return out, nil
}

// Explicit allowlist: portable progress exports have no resource roots, live
// credentials, database, sessions, host identity or private remote configuration.
type PortableProgress struct {
	Version      int                `json:"version"`
	Target       string             `json:"target"`
	CheckpointID string             `json:"checkpoint_id"`
	SavedAt      time.Time          `json:"saved_at"`
	Summary      string             `json:"summary"`
	NextAction   string             `json:"next_action"`
	Blockers     []string           `json:"blockers"`
	Decisions    []PortableDecision `json:"decisions"`
	Artifacts    []PortableArtifact `json:"artifacts"`
}
type PortableDecision struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}
type PortableArtifact struct {
	ArtifactID string `json:"artifact_id"`
	VersionID  string `json:"version_id"`
	Name       string `json:"name"`
	Digest     string `json:"digest"`
	Bytes      int64  `json:"bytes"`
}

func (s Service) PreservationExport(ctx context.Context, target, checkpoint string) (PortableProgress, error) {
	out := PortableProgress{}
	st, err := s.Store.State(ctx)
	if err != nil {
		return out, err
	}
	c, ok := st.Checkpoints[checkpoint]
	if !ok || c.Target != target {
		return out, fmt.Errorf("checkpoint not found for target")
	}
	out = PortableProgress{Version: 1, Target: target, CheckpointID: c.ID, SavedAt: c.At, Summary: c.Summary, NextAction: c.NextAction, Blockers: c.Blockers, Decisions: []PortableDecision{}, Artifacts: []PortableArtifact{}}
	for _, id := range c.Decisions {
		d := st.Decisions[id]
		out.Decisions = append(out.Decisions, PortableDecision{d.ID, d.Text})
	}
	for _, ref := range c.Artifacts {
		a := st.Artifacts[ref.ArtifactID]
		v := st.ArtifactVersions[ref.VersionID]
		out.Artifacts = append(out.Artifacts, PortableArtifact{a.ID, v.ID, a.Name, v.Observation.Digest, v.Observation.Bytes})
	}
	return out, nil
}
