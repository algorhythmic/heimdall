package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sync"
	"time"
)

type SourceInput struct {
	SocketDir string `json:"socket_dir"`
	Host      string `json:"host"`
	Epoch     string `json:"epoch"`
}
type ViewportInput struct {
	ManifestID       string                `json:"manifest_id"`
	SurfaceID        string                `json:"surface_id"`
	SourceID         string                `json:"source_id,omitempty"`
	SnapshotID       string                `json:"snapshot_id,omitempty"`
	Window           *model.WindowIdentity `json:"window,omitempty"`
	SessionBindingID string                `json:"session_binding_id,omitempty"`
}
type ViewportRequest struct {
	Version              int            `json:"version"`
	ID                   string         `json:"id"`
	Op                   string         `json:"op"`
	Previous             string         `json:"previous"`
	Target               string         `json:"target,omitempty"`
	ExpectedTaskRevision int64          `json:"expected_task_revision,omitempty"`
	Source               *SourceInput   `json:"source,omitempty"`
	Binding              *ViewportInput `json:"binding,omitempty"`
}

func DecodeViewport(b []byte) (ViewportRequest, error) {
	var r ViewportRequest
	if len(b) > MaxRequest {
		return r, fmt.Errorf("viewport request exceeds 64 KiB")
	}
	if err := model.StrictJSON(b, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}
func (r ViewportRequest) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || (r.Previous != "none" && !model.OpaqueID.MatchString(r.Previous)) {
		return fmt.Errorf("version, request ID and explicit previous head required")
	}
	switch r.Op {
	case "select", "stop":
		if r.Target != "" || r.ExpectedTaskRevision != 0 || r.Binding != nil || (r.Source != nil) != (r.Op == "select") {
			return fmt.Errorf("select requires only a source; stop requires no payload")
		}
		if r.Source != nil {
			s := r.Source
			return (model.DesktopSource{Version: 1, ID: r.ID, Active: true, SocketDir: s.SocketDir, Host: s.Host, Epoch: s.Epoch, CompositorVersion: "0.56.2", Actor: "cli", At: time.Unix(1, 0)}).Validate()
		}
	case "bind", "unbind":
		if r.Source != nil || r.Binding == nil {
			return fmt.Errorf("viewport binding payload required")
		}
		b := r.record(time.Unix(1, 0))
		return b.Validate()
	default:
		return fmt.Errorf("unknown viewport operation")
	}
	return nil
}
func (r ViewportRequest) record(now time.Time) model.ViewportBinding {
	b := r.Binding
	return model.ViewportBinding{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, ManifestID: b.ManifestID, SurfaceID: b.SurfaceID, Previous: previous(r.Previous), Active: r.Op == "bind", SourceID: b.SourceID, SnapshotID: b.SnapshotID, Window: b.Window, SessionBindingID: b.SessionBindingID, Actor: "cli", At: now.UTC()}
}

// This service serializes selection changes with explicit binding commands.
// Reads never infer ownership from title, PID, class, cwd or a similar pane.
type ViewportService struct {
	Store    *store.Store
	Observer *hyprland.Observer
	mu       sync.Mutex
}

func (s *ViewportService) Restore(ctx context.Context) error {
	st, err := s.Store.State(ctx)
	if err == nil {
		s.Observer.Configure(st.DesktopSources[st.DesktopSourceHead])
	}
	return err
}
func (s *ViewportService) Execute(ctx context.Context, r ViewportRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("viewport mutation requires local CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(r)
	checkedSnapshot := ""
	check := func(model.State) error {
		if checkedSnapshot != "" {
			if err := s.Observer.Check(checkedSnapshot); err != nil {
				return fmt.Errorf("%s: %w", err, store.ErrConflict)
			}
		}
		return nil
	}
	result, err := s.Store.TransactChecked(ctx, "viewport-"+r.ID, actor, raw, now, check, func(st model.State) (store.Change, error) {
		change := store.Change{Revision: st.Revision}
		var payload any
		subject, verb := "desktop", "selected"
		if r.Op == "select" || r.Op == "stop" {
			if st.DesktopSourceHead != previous(r.Previous) {
				return change, fmt.Errorf("source head: %w", store.ErrConflict)
			}
			v := model.DesktopSource{Version: 1, ID: r.ID, Previous: previous(r.Previous), Active: r.Op == "select", Actor: actor, At: now.UTC()}
			if v.Active {
				observed, err := hyprland.Probe(ctx, r.Source.SocketDir, s.Observer.Connector)
				if err != nil {
					return change, err
				}
				snapshot := observed.Snapshot
				if snapshot.SourceEpoch != r.Source.Epoch || snapshot.Host != r.Source.Host {
					return change, fmt.Errorf("source changed since probe: %w", store.ErrConflict)
				}
				v.SocketDir = r.Source.SocketDir
				v.Host = snapshot.Host
				v.Epoch = snapshot.SourceEpoch
				v.CompositorVersion = snapshot.CompositorVersion
			}
			payload = v
		} else {
			v := r.record(now)
			if st.Tasks[v.Target].Revision != v.TaskRevision || st.WorkspaceHeads[v.Target] != v.ManifestID || st.ViewportHeads[v.SurfaceID] != v.Previous {
				return change, fmt.Errorf("task, manifest or viewport head: %w", store.ErrConflict)
			}
			if v.Active {
				if st.DesktopSourceHead != v.SourceID {
					return change, fmt.Errorf("source selection: %w", store.ErrConflict)
				}
				observed, err := s.Observer.Read(ctx, true)
				if err != nil {
					return change, err
				}
				if !observed.Fresh || observed.Snapshot.ID != v.SnapshotID {
					return change, fmt.Errorf("snapshot changed; inspect fresh inventory: %w", store.ErrConflict)
				}
				found := false
				for _, w := range observed.Snapshot.Windows {
					if w.Identity == *v.Window {
						found = true
					}
				}
				if !found {
					return change, fmt.Errorf("selected window is absent from fresh inventory: %w", store.ErrConflict)
				}
				checkedSnapshot = v.SnapshotID
			}
			payload = v
			subject, verb = "viewport", "bound"
			if !v.Active {
				verb = "unbound"
			}
		}
		change.Events = []store.Pending{{Subject: subject, Verb: verb, EntityID: r.ID, Payload: payload}}
		change.Result = payload
		return change, nil
	})
	if err == nil && (r.Op == "select" || r.Op == "stop") {
		err = s.Restore(ctx)
	}
	return result, err
}

type ViewportView struct {
	Target     string            `json:"target"`
	ManifestID string            `json:"manifest_id"`
	Fresh      bool              `json:"fresh"`
	Issue      string            `json:"issue"`
	Surfaces   []ViewportSurface `json:"surfaces"`
}
type ViewportSurface struct {
	SurfaceID string                 `json:"surface_id"`
	Status    string                 `json:"status"`
	Binding   *model.ViewportBinding `json:"binding,omitempty"`
	Window    *model.DesktopWindow   `json:"window,omitempty"`
	Session   *model.SessionBinding  `json:"session,omitempty"`
	Issues    []string               `json:"issues"`
}

func (s *ViewportService) View(ctx context.Context, target string, fresh bool) (ViewportView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.Store.State(ctx)
	if err != nil {
		return ViewportView{}, err
	}
	task, ok := st.Tasks[target]
	if !ok {
		return ViewportView{}, fmt.Errorf("task not found")
	}
	observed, _ := s.Observer.Read(ctx, fresh)
	v := ViewportView{Target: target, ManifestID: st.WorkspaceHeads[target], Fresh: observed.Fresh, Issue: observed.Issue, Surfaces: []ViewportSurface{}}
	m := st.WorkspaceManifests[v.ManifestID]
	if v.ManifestID == "" {
		v.Issue = "manifest_missing"
	}
	for _, surface := range m.Surfaces {
		row := ViewportSurface{SurfaceID: surface.ID, Status: "unowned", Issues: []string{}}
		b, ok := st.ViewportBindings[st.ViewportHeads[surface.ID]]
		if ok {
			row.Binding = &b
		}
		if b.Active {
			row.Status = "unavailable"
			if observed.Fresh && observed.Snapshot != nil && observed.Snapshot.SourceEpoch == b.Window.SourceEpoch {
				row.Status = "missing"
				for _, w := range observed.Snapshot.Windows {
					if w.Identity == *b.Window {
						row.Status = "observed"
						copy := w
						row.Window = &copy
						break
					}
				}
			}
			if b.ManifestID != m.ID {
				row.Issues = append(row.Issues, "binding_manifest_changed")
			}
			if b.TaskRevision != task.Revision {
				row.Issues = append(row.Issues, "binding_task_changed")
			}
			if b.SessionBindingID != "" {
				if st.SessionHeads[surface.ID] != b.SessionBindingID {
					row.Issues = append(row.Issues, "session_binding_changed")
				} else {
					session := st.SessionBindings[b.SessionBindingID]
					row.Session = &session
					row.Issues = append(row.Issues, "terminal_pane_attachment_not_verified", "live_session_not_checked")
				}
			}
		}
		v.Surfaces = append(v.Surfaces, row)
	}
	return boundRead(v)
}
