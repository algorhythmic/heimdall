// Package workspace records explicit desired membership and session declarations.
// It performs no filesystem inspection, application discovery or desktop actions.
package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"time"
)

const MaxRequest = 64 << 10
const MaxRead = 512 << 10

type ManifestInput struct {
	Previous string                 `json:"previous"`
	Name     string                 `json:"name"`
	Surfaces []model.DesiredSurface `json:"surfaces"`
}

type SessionInput struct {
	Previous   string                `json:"previous"`
	ManifestID string                `json:"manifest_id"`
	SurfaceID  string                `json:"surface_id"`
	Locator    *model.SessionLocator `json:"locator,omitempty"`
}

type Request struct {
	Version              int            `json:"version"`
	ID                   string         `json:"id"`
	Op                   string         `json:"op"`
	Target               string         `json:"target"`
	ExpectedTaskRevision int64          `json:"expected_task_revision"`
	Manifest             *ManifestInput `json:"manifest,omitempty"`
	Session              *SessionInput  `json:"session,omitempty"`
}

func Decode(b []byte) (Request, error) {
	var r Request
	if len(b) > MaxRequest {
		return r, fmt.Errorf("workspace request exceeds 64 KiB")
	}
	if err := model.StrictJSON(b, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}

func previous(s string) string {
	if s == "none" {
		return ""
	}
	return s
}

func (r Request) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.ValidID(r.Target) || r.ExpectedTaskRevision < 1 || (r.Manifest == nil) == (r.Session == nil) {
		return fmt.Errorf("workspace version, request ID, task target, revision and exactly one payload required")
	}
	head := ""
	switch r.Op {
	case "workspace.accept":
		if r.Manifest == nil {
			return fmt.Errorf("manifest required")
		}
		if err := model.ValidDesiredWorkspace(r.Manifest.Name, r.Manifest.Surfaces); err != nil {
			return err
		}
		head = r.Manifest.Previous
	case "session.bind", "session.unbind":
		if r.Session == nil || !model.OpaqueID.MatchString(r.Session.ManifestID) || !model.OpaqueID.MatchString(r.Session.SurfaceID) || (r.Session.Locator != nil) != (r.Op == "session.bind") {
			return fmt.Errorf("session requires manifest and surface IDs, and a locator only for bind")
		}
		if r.Session.Locator != nil {
			if err := r.Session.Locator.Validate(); err != nil {
				return err
			}
		}
		head = r.Session.Previous
	default:
		return fmt.Errorf("unknown workspace operation")
	}
	if head != "none" && !model.OpaqueID.MatchString(head) {
		return fmt.Errorf("previous must be an ID or the explicit sentinel none")
	}
	return nil
}

type Service struct{ Store *store.Store }

func (s Service) Execute(ctx context.Context, r Request, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("workspace mutation requires user CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxRequest {
		return nil, fmt.Errorf("workspace request exceeds 64 KiB")
	}
	return s.Store.Transact(ctx, "workspace-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		change := store.Change{Revision: st.Revision}
		task, ok := st.Tasks[r.Target]
		if !ok {
			return change, fmt.Errorf("task not found")
		}
		if task.Revision != r.ExpectedTaskRevision {
			return change, fmt.Errorf("task revision: %w", store.ErrConflict)
		}
		var payload any
		var subject, verb string
		if r.Manifest != nil {
			m := r.Manifest
			if st.WorkspaceHeads[r.Target] != previous(m.Previous) {
				return change, fmt.Errorf("manifest head: %w", store.ErrConflict)
			}
			payload = model.WorkspaceManifest{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, Previous: previous(m.Previous), Name: m.Name, Surfaces: m.Surfaces, Actor: actor, At: now.UTC()}
			subject, verb = "workspace", "accepted"
		} else {
			b := r.Session
			if st.WorkspaceHeads[r.Target] != b.ManifestID || st.SessionHeads[b.SurfaceID] != previous(b.Previous) {
				return change, fmt.Errorf("manifest or binding head: %w", store.ErrConflict)
			}
			if b.Locator != nil && st.WorkspaceManifests[b.ManifestID].TaskRevision != task.Revision {
				return change, fmt.Errorf("stale workspace task revision: %w", store.ErrConflict)
			}
			payload = model.SessionBinding{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, ManifestID: b.ManifestID, SurfaceID: b.SurfaceID, Previous: previous(b.Previous), Active: b.Locator != nil, Locator: b.Locator, Actor: actor, At: now.UTC()}
			subject, verb = "session", "bound"
			if b.Locator == nil {
				verb = "unbound"
			}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return change, err
		}
		if len(encoded) > MaxRequest {
			return change, fmt.Errorf("workspace record exceeds 64 KiB")
		}
		// The reducer is the shared semantic validator for live writes and replay.
		change.Events = []store.Pending{{Subject: subject, Verb: verb, EntityID: r.ID, Payload: payload}}
		change.Result = payload
		return change, nil
	})
}

type BindingView struct {
	SurfaceID string                `json:"surface_id"`
	Head      string                `json:"head"`
	Binding   *model.SessionBinding `json:"binding,omitempty"`
	Status    string                `json:"status"`
	Issues    []string              `json:"issues"`
}

type View struct {
	Version      int                      `json:"version"`
	Target       string                   `json:"target"`
	TaskRevision int64                    `json:"task_revision"`
	ManifestHead string                   `json:"manifest_head"`
	Manifest     *model.WorkspaceManifest `json:"manifest,omitempty"`
	Issues       []string                 `json:"issues"`
	Bindings     []BindingView            `json:"bindings"`
}

func boundRead[T any](v T) (T, error) {
	b, err := json.Marshal(v)
	if err == nil && len(b) > MaxRead {
		err = fmt.Errorf("workspace response exceeds 512 KiB")
	}
	return v, err
}

func (s Service) View(ctx context.Context, target string) (View, error) {
	v := View{Version: 1, Target: target, Issues: []string{}, Bindings: []BindingView{}}
	st, err := s.Store.State(ctx)
	if err != nil {
		return v, err
	}
	task, ok := st.Tasks[target]
	if !ok {
		return v, fmt.Errorf("task not found")
	}
	v.TaskRevision, v.ManifestHead = task.Revision, st.WorkspaceHeads[target]
	if v.ManifestHead == "" {
		v.Issues = append(v.Issues, "manifest_missing")
		return v, nil
	}
	m := st.WorkspaceManifests[v.ManifestHead]
	v.Manifest = &m
	if m.TaskRevision != task.Revision {
		v.Issues = append(v.Issues, "manifest_task_changed")
	}
	for _, surface := range m.Surfaces {
		if surface.Kind != "terminal" {
			continue
		}
		b := BindingView{SurfaceID: surface.ID, Head: st.SessionHeads[surface.ID], Status: "unbound", Issues: []string{}}
		if b.Head != "" {
			record := st.SessionBindings[b.Head]
			b.Binding = &record
			if record.Active {
				b.Status = "unverified"
				if record.Version == 2 {
					b.Status = "recorded"
				}
				b.Issues = append(b.Issues, "live_session_not_checked")
				if record.ManifestID != m.ID {
					b.Issues = append(b.Issues, "binding_manifest_changed")
				}
				if record.TaskRevision != task.Revision {
					b.Issues = append(b.Issues, "binding_task_changed")
				}
			}
		}
		v.Bindings = append(v.Bindings, b)
	}
	return boundRead(v)
}

// Lookups require both the task and record identity. There is no global-ID
// fallback, inherited ownership, or new capability for scoped read credentials.
func (s Service) Manifest(ctx context.Context, target, id string) (model.WorkspaceManifest, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return model.WorkspaceManifest{}, err
	}
	m, ok := st.WorkspaceManifests[id]
	if !ok || m.Target != target {
		return model.WorkspaceManifest{}, fmt.Errorf("manifest not found for task")
	}
	return boundRead(m)
}

func (s Service) Binding(ctx context.Context, target, surface, id string) (model.SessionBinding, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return model.SessionBinding{}, err
	}
	if id == "" {
		id = st.SessionHeads[surface]
	}
	b, ok := st.SessionBindings[id]
	if !ok || b.Target != target || b.SurfaceID != surface {
		return model.SessionBinding{}, fmt.Errorf("binding not found for task and surface")
	}
	return boundRead(b)
}
