package workspace

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sync"
	"time"
)

const PreviewMaxBytes = 256 << 10

type PreviewService struct {
	Store    *store.Store
	Observer *hyprland.Observer
	Herdr    HerdrAdapter
	once     sync.Once
	key      [32]byte
}

func (s *PreviewService) seal(v Preview) string {
	s.once.Do(func() { _, _ = rand.Read(s.key[:]) })
	v.Token = ""
	b, _ := json.Marshal(v)
	m := hmac.New(sha256.New, s.key[:])
	m.Write(b)
	return hex.EncodeToString(m.Sum(nil))
}

func previewDigest(v Preview) string {
	v.Token, v.Digest = "", ""
	v.AsOf, v.ExpiresAt = time.Time{}, time.Time{}
	v.SnapshotAgeSeconds = 0
	v.Boundary.StartedAt, v.Boundary.FinishedAt = time.Time{}, time.Time{}
	// A repaired event gap is diagnostic history, not a changed postcondition.
	v.Boundary.KnownEventGaps = 0
	b, _ := json.Marshal(v)
	return model.SnapshotHash(b)
}

func (s *PreviewService) Validate(ctx context.Context, v Preview, now time.Time) (PreviewValidation, error) {
	result := PreviewValidation{Version: 1}
	if v.Version != 1 || v.Request.Validate() != nil || !hmac.Equal([]byte(v.Token), []byte(s.seal(v))) {
		result.Issue = "preview_not_issued_by_this_daemon"
		return result, nil
	}
	if now.Before(v.AsOf) || !now.Before(v.ExpiresAt) {
		result.Issue = "preview_expired"
		return result, nil
	}
	fresh, err := s.Build(ctx, v.Request, now)
	if err != nil {
		result.Issue = "preview_inputs_unavailable_or_changed"
		return result, nil
	}
	result.Preview = &fresh
	result.Current = fresh.Fresh && fresh.Digest == v.Digest
	if !result.Current {
		result.Issue = "preview_changed"
	}
	return result, nil
}

// Diff resolves the current manifest/head once and returns those explicit IDs.
// It never captures a point, adds a pin, updates desired state or emits events.
func (s *PreviewService) Diff(ctx context.Context, target string, now time.Time) (Preview, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return Preview{}, err
	}
	if !model.ValidID(target) || st.Tasks[target].Revision == 0 {
		return Preview{}, fmt.Errorf("explicit existing task required")
	}
	r := PreviewRequest{Version: 1, Target: target, ManifestID: st.WorkspaceHeads[target], SnapshotID: st.SnapshotHeads[target].ID}
	return s.build(ctx, r, now, true)
}

func (s *PreviewService) Build(ctx context.Context, r PreviewRequest, now time.Time) (Preview, error) {
	if err := r.Validate(); err != nil {
		return Preview{}, err
	}
	return s.build(ctx, r, now, false)
}

func (s *PreviewService) List(ctx context.Context, target string, now time.Time) (WorkspaceList, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return WorkspaceList{}, err
	}
	if !model.ValidID(target) || st.Tasks[target].Revision == 0 {
		return WorkspaceList{}, fmt.Errorf("explicit existing task required")
	}
	v, err := s.build(ctx, PreviewRequest{Version: 1, Target: target, ManifestID: st.WorkspaceHeads[target]}, now, true)
	if err != nil {
		return WorkspaceList{}, err
	}
	out := WorkspaceList{Target: target, TaskRevision: v.TaskRevision, ManifestID: v.Request.ManifestID, Fresh: v.Fresh, Boundary: v.Boundary, Issues: []string{}, Surfaces: []ListedSurface{}}
	for _, issue := range v.Issues {
		if issue != "snapshot_missing" {
			out.Issues = append(out.Issues, issue)
		}
	}
	for _, row := range v.Surfaces {
		issues := []string{}
		for _, issue := range row.Issues {
			if issue != "saved_layout_missing" {
				issues = append(issues, issue)
			}
		}
		status := row.ObservationStatus
		if !v.Fresh {
			status = "unavailable"
		}
		out.Surfaces = append(out.Surfaces, ListedSurface{SurfaceID: row.SurfaceID, Kind: row.Kind, Label: row.Label, Status: status, Window: row.Window, Placement: row.Observed, SessionStatus: row.SessionStatus, Issues: issues})
	}
	return out, nil
}

func (s *PreviewService) build(ctx context.Context, r PreviewRequest, now time.Time, allowMissing bool) (Preview, error) {
	st, point, err := s.Store.WorkspacePreviewInputs(ctx, r.Target, r.SnapshotID)
	if err != nil {
		return Preview{}, err
	}
	if st.Tasks[r.Target].Revision == 0 || st.WorkspaceHeads[r.Target] != r.ManifestID {
		return Preview{}, fmt.Errorf("task or selected manifest changed: %w", store.ErrConflict)
	}
	if r.SnapshotID == "" && !allowMissing {
		return Preview{}, fmt.Errorf("snapshot required")
	}
	observed, _ := s.Observer.Read(ctx, true)
	checks := s.sessions(ctx, st, r.Target, now)
	v := planPreview(st, r, point, observed, checks, now)
	// IPC remains outside the writer. Do not publish a mixed observation if
	// current bindings, another owner's claim, a pin or a source changed.
	after, point, err := s.Store.WorkspacePreviewInputs(ctx, r.Target, r.SnapshotID)
	if err != nil {
		return Preview{}, err
	}
	check := planPreview(after, r, point, observed, checks, now)
	if previewDigest(v) != previewDigest(check) {
		return Preview{}, fmt.Errorf("workspace inputs changed during observation: %w", store.ErrConflict)
	}
	if observed.Fresh && observed.Snapshot != nil {
		if err := s.Observer.Check(observed.Snapshot.ID); err != nil {
			return Preview{}, fmt.Errorf("observation changed: %w", store.ErrConflict)
		}
	}
	v.Digest = previewDigest(v)
	v.Token = s.seal(v)
	b, _ := json.Marshal(v)
	if len(b) > PreviewMaxBytes {
		return Preview{}, fmt.Errorf("preview exceeds 256 KiB")
	}
	return v, nil
}

func (s *PreviewService) sessions(ctx context.Context, st model.State, target string, now time.Time) map[string]SessionCheck {
	checks := map[string]SessionCheck{}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, surface := range st.WorkspaceManifests[st.WorkspaceHeads[target]].Surfaces {
		b := st.SessionBindings[st.SessionHeads[surface.ID]]
		if !b.Active {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := SessionCheck{Status: "unavailable", Issues: []string{"session_adapter_unavailable"}}
			select {
			case sem <- struct{}{}:
				if s.Herdr != nil {
					c, _ = (HerdrService{Store: s.Store, Adapter: s.Herdr}).check(ctx, st, b, now)
				}
				<-sem
			case <-ctx.Done():
				c.Issues = []string{"session_observation_timeout"}
			}
			mu.Lock()
			checks[surface.ID] = c
			mu.Unlock()
		}()
	}
	wg.Wait()
	return checks
}
