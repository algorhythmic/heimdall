package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

type SnapshotStatus struct {
	Target         string                `json:"target"`
	TaskRevision   int64                 `json:"task_revision"`
	ManifestID     string                `json:"manifest_id"`
	SourceID       string                `json:"source_id"`
	InputDigest    string                `json:"input_digest"`
	Head           *model.WorkspacePoint `json:"head,omitempty"`
	HeadAgeSeconds int64                 `json:"head_age_seconds"`
	HeadAvailable  bool                  `json:"head_available"`
	Policy         *model.SnapshotPolicy `json:"policy,omitempty"`
	Pins           []model.SnapshotPin   `json:"pins"`
	Capture        CaptureDiagnostic     `json:"capture"`
}

func (s *SnapshotService) diagnostic(target string, update func(*CaptureDiagnostic)) CaptureDiagnostic {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.diagnostics == nil {
		s.diagnostics = map[string]CaptureDiagnostic{}
	}
	d := s.diagnostics[target]
	if update != nil {
		update(&d)
		s.diagnostics[target] = d
	}
	return d
}
func (s *SnapshotService) Status(ctx context.Context, target string, now time.Time) (SnapshotStatus, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return SnapshotStatus{}, err
	}
	task, ok := st.Tasks[target]
	if !ok {
		return SnapshotStatus{}, fmt.Errorf("task not found")
	}
	v := SnapshotStatus{Target: target, TaskRevision: task.Revision, ManifestID: st.WorkspaceHeads[target], SourceID: st.DesktopSourceHead, InputDigest: model.SnapshotInputDigest(st, target), Pins: []model.SnapshotPin{}, Capture: s.diagnostic(target, nil)}
	if head, ok := st.SnapshotHeads[target]; ok {
		v.Head = &head
		v.HeadAgeSeconds = max(0, int64(now.Sub(head.ObservedAt).Seconds()))
		_, err = s.Store.WorkspacePoint(ctx, target, head.ID)
		v.HeadAvailable = err == nil
		if err != nil {
			v.Capture.Issue = "retained_head_payload_unavailable"
		}
	}
	if policy, ok := st.SnapshotPolicies[target]; ok {
		v.Policy = &policy
		if policy.Enabled && (policy.TaskRevision != task.Revision || policy.ManifestID != v.ManifestID || policy.SourceID != v.SourceID) {
			v.Capture.Issue = "capture_policy_stale"
		}
	}
	for _, pin := range st.SnapshotPins {
		if pin.Target == target {
			v.Pins = append(v.Pins, pin)
		}
	}
	sort.Slice(v.Pins, func(i, j int) bool { return v.Pins[i].SnapshotID < v.Pins[j].SnapshotID })
	return v, nil
}
func (s *SnapshotService) Run(ctx context.Context, clock func() time.Time) {
	if clock == nil {
		clock = time.Now
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.Scan(ctx, clock().UTC())
		}
	}
}

// Scan uses the observer's latest bounded cache. It never waits for IPC while
// holding the task writer. Source reads are shared by all configured targets.
// The injected time controls debounce/deadline behavior in deterministic tests.
func (s *SnapshotService) Scan(ctx context.Context, now time.Time) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return
	}
	observed, _ := s.Observer.Read(ctx, false)
	targets := []string{}
	for target, p := range st.SnapshotPolicies {
		if p.Enabled {
			targets = append(targets, target)
		}
	}
	sort.Strings(targets)
	for _, target := range targets {
		if ctx.Err() != nil {
			return
		}
		policy := st.SnapshotPolicies[target]
		issue := ""
		if policy.TaskRevision != st.Tasks[target].Revision || policy.ManifestID != st.WorkspaceHeads[target] || policy.SourceID != st.DesktopSourceHead {
			issue = "capture_policy_stale"
		}
		payload, err := pointPayload(st, target, observed)
		if err != nil {
			issue = "fresh_source_unavailable"
		} else if payload.Coverage != "complete" {
			issue = "partial_coverage_last_good_preserved"
		}
		if issue != "" {
			s.diagnostic(target, func(d *CaptureDiagnostic) { d.LastAttempt = now; d.Issue = issue })
			continue
		}
		content := model.SnapshotContentDigest(payload)
		if st.SnapshotHeads[target].ContentDigest == content {
			s.diagnostic(target, func(d *CaptureDiagnostic) {
				d.LastObserved = payload.ObservedAt
				d.Issue = ""
				d.DirtySince = time.Time{}
				d.LastChanged = time.Time{}
				d.ContentDigest = content
			})
			continue
		}
		d := s.diagnostic(target, func(d *CaptureDiagnostic) {
			d.LastObserved = payload.ObservedAt
			d.Issue = ""
			if d.DirtySince.IsZero() {
				d.DirtySince = now
			}
			if d.ContentDigest != content {
				d.LastChanged = now
				d.ContentDigest = content
			}
		})
		if now.Sub(d.LastChanged) < time.Duration(policy.DebounceSeconds)*time.Second && now.Sub(d.DirtySince) < time.Duration(policy.MaxDirtySeconds)*time.Second {
			continue
		}
		previousHead := st.SnapshotHeads[target].ID
		if previousHead == "" {
			previousHead = "none"
		}
		r := SnapshotRequest{Version: 1, ID: model.NewID(), Op: "capture", Target: target, Previous: previousHead, ExpectedTaskRevision: policy.TaskRevision, ManifestID: policy.ManifestID, SourceID: policy.SourceID, InputDigest: model.SnapshotInputDigest(st, target)}
		raw, _ := json.Marshal(struct {
			Request  SnapshotRequest `json:"request"`
			PolicyID string          `json:"policy_id"`
		}{r, policy.ID})
		_, err = s.capture(ctx, r, st, observed, "snapshotter", policy.ID, raw, now)
		s.diagnostic(target, func(d *CaptureDiagnostic) {
			d.LastAttempt = now
			if err != nil {
				d.Issue = err.Error()
			} else {
				d.Issue = ""
				d.DirtySince = time.Time{}
				d.LastChanged = time.Time{}
			}
		})
		// A concurrent command may have changed policy or scope. Next scan obtains
		// fresh state; this scan never extends authority from the stale copy.
	}
}
func (s *SnapshotService) List(ctx context.Context, target string, before int64, limit int) ([]store.PointView, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := st.Tasks[target]; !ok {
		return nil, fmt.Errorf("task not found")
	}
	return s.Store.WorkspacePoints(ctx, target, before, limit)
}
