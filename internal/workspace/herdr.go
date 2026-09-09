package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/adapters/herdr"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"strings"
	"time"
	"unicode"
)

type HerdrAdapter interface {
	Observe(context.Context, string, string, string) (herdr.Observation, error)
	Report(context.Context, herdr.Observation, string, int64, string, map[string]string, int, bool) error
}

type HerdrService struct {
	Store   *store.Store
	Adapter HerdrAdapter
}

type HerdrBindRequest struct {
	Version              int    `json:"version"`
	ID                   string `json:"id"`
	Target               string `json:"target"`
	SurfaceID            string `json:"surface_id"`
	ManifestID           string `json:"manifest_id"`
	Previous             string `json:"previous"`
	ExpectedTaskRevision int64  `json:"expected_task_revision"`
	Socket               string `json:"socket"`
	PaneID               string `json:"pane_id"`
}

func (r HerdrBindRequest) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.ValidID(r.Target) || !model.OpaqueID.MatchString(r.SurfaceID) || !model.OpaqueID.MatchString(r.ManifestID) || (r.Previous != "none" && !model.OpaqueID.MatchString(r.Previous)) || r.ExpectedTaskRevision < 1 || !strings.HasPrefix(r.Socket, "/") || len(r.Socket) > 256 || strings.TrimSpace(r.PaneID) == "" || len(r.PaneID) > 256 {
		return fmt.Errorf("invalid explicit Herdr binding request")
	}
	return nil
}

func (s HerdrService) Bind(ctx context.Context, r HerdrBindRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("Herdr binding requires CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return s.Store.Transact(ctx, "workspace-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		m, ok := st.WorkspaceManifests[r.ManifestID]
		if !ok || m.Target != r.Target || st.WorkspaceHeads[r.Target] != m.ID || st.Tasks[r.Target].Revision != r.ExpectedTaskRevision || m.TaskRevision != r.ExpectedTaskRevision || st.SessionHeads[r.SurfaceID] != previous(r.Previous) {
			return c, fmt.Errorf("task, manifest or binding changed: %w", store.ErrConflict)
		}
		owned := false
		for _, surface := range m.Surfaces {
			if surface.ID == r.SurfaceID && surface.Kind == "terminal" {
				owned = true
			}
		}
		if !owned {
			return c, fmt.Errorf("task-owned terminal surface required")
		}
		o, err := s.Adapter.Observe(ctx, r.Socket, r.PaneID, "")
		if err != nil {
			return c, err
		}
		v := model.SessionBinding{Version: 2, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, ManifestID: r.ManifestID, SurfaceID: r.SurfaceID, Previous: previous(r.Previous), Active: true, Locator: &o.Locator, Herdr: &o.Herdr, Actor: actor, At: now.UTC()}
		if err := model.ValidSessionBinding(v); err != nil {
			return c, err
		}
		c.Events = []store.Pending{{Subject: "session", Verb: "bound", EntityID: v.ID, Payload: v}}
		c.Result = v
		return c, nil
	})
}

type SessionCheck struct {
	Version   int       `json:"version"`
	Target    string    `json:"target"`
	SurfaceID string    `json:"surface_id"`
	BindingID string    `json:"binding_id"`
	Status    string    `json:"status"`
	Issues    []string  `json:"issues"`
	CheckedAt time.Time `json:"checked_at"`
}

func selectedBinding(st model.State, target, surface, id string) (model.SessionBinding, error) {
	b, ok := st.SessionBindings[st.SessionHeads[surface]]
	if !ok || b.Target != target || b.SurfaceID != surface || (id != "" && b.ID != id) {
		return b, fmt.Errorf("binding not current for task and surface: %w", store.ErrConflict)
	}
	return b, nil
}

func bindingIssues(st model.State, b model.SessionBinding) []string {
	issues := []string{}
	if !b.Active {
		return []string{"session_unbound"}
	}
	if b.Version != 2 || b.Herdr == nil {
		return []string{"unverified_generic_binding"}
	}
	if st.Tasks[b.Target].Revision != b.TaskRevision {
		issues = append(issues, "binding_task_changed")
	}
	if st.WorkspaceHeads[b.Target] != b.ManifestID {
		issues = append(issues, "binding_manifest_changed")
	}
	return issues
}

func (s HerdrService) check(ctx context.Context, st model.State, b model.SessionBinding, now time.Time) (SessionCheck, herdr.Observation) {
	c := SessionCheck{Version: 1, Target: b.Target, SurfaceID: b.SurfaceID, BindingID: b.ID, Status: "stale", Issues: bindingIssues(st, b), CheckedAt: now.UTC()}
	if len(c.Issues) != 0 {
		return c, herdr.Observation{}
	}
	o, err := s.Adapter.Observe(ctx, b.Locator.SessionID, b.Locator.PaneID, b.Locator.SourceEpoch)
	if err != nil {
		code := "observation_unavailable"
		var ae *herdr.Error
		if errors.As(err, &ae) {
			code = ae.Code
		}
		if code == "disconnected" {
			c.Status = "disconnected"
		}
		c.Issues = append(c.Issues, code)
		return c, herdr.Observation{}
	}
	if !reflect.DeepEqual(o.Locator, *b.Locator) || !reflect.DeepEqual(o.Herdr, *b.Herdr) {
		c.Issues = append(c.Issues, "session_identity_changed")
		return c, o
	}
	c.Status = "current"
	return c, o
}

// Refresh is a fresh read. Stored records remain observations at their original
// time; a failed refresh cannot silently reassign ownership or replace a head.
func (s HerdrService) Refresh(ctx context.Context, target, surface, id string, now time.Time) (SessionCheck, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return SessionCheck{}, err
	}
	b, err := selectedBinding(st, target, surface, id)
	if err != nil {
		return SessionCheck{}, err
	}
	c, _ := s.check(ctx, st, b, now)
	after, err := s.Store.State(ctx)
	if err != nil {
		return c, err
	}
	if _, err := selectedBinding(after, target, surface, b.ID); err != nil {
		c.Status = "stale"
		c.Issues = append(c.Issues, "binding_head_changed")
	}
	if !reflect.DeepEqual(bindingIssues(st, b), bindingIssues(after, b)) {
		c.Status = "stale"
		c.Issues = append(c.Issues, "task_or_manifest_changed_during_check")
	}
	return c, nil
}

type HerdrPublishRequest struct {
	Version          int    `json:"version"`
	ID               string `json:"id"`
	Target           string `json:"target"`
	SurfaceID        string `json:"surface_id"`
	BindingID        string `json:"binding_id"`
	WorkspaceSummary bool   `json:"workspace_summary,omitempty"`
}

func (r HerdrPublishRequest) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.ValidID(r.Target) || !model.OpaqueID.MatchString(r.SurfaceID) || !model.OpaqueID.MatchString(r.BindingID) {
		return fmt.Errorf("explicit metadata request, task, surface and binding IDs required")
	}
	return nil
}

type MetadataResult struct {
	Version          int               `json:"version"`
	Target           string            `json:"target"`
	BindingID        string            `json:"binding_id"`
	Status           string            `json:"status"`
	Issue            string            `json:"issue,omitempty"`
	At               time.Time         `json:"at"`
	TTLMillis        int               `json:"ttl_ms"`
	Tokens           map[string]string `json:"tokens"`
	WorkspaceSummary bool              `json:"workspace_summary"`
}

func displayText(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) || r == '\u2028' || r == '\u2029' {
			r = ' '
		}
		if b.Len()+len(string(r)) > max {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// Publish writes only an expiring display projection. It holds the existing
// writer while checking the binding and sending metadata, so a concurrent
// unbind/rebind cannot publish stale task text. There is no automatic refresh,
// agent state mutation or replayed output. An uncertain result stays uncertain.
func (s HerdrService) Publish(ctx context.Context, r HerdrPublishRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("Herdr metadata requires CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return s.Store.Transact(ctx, "herdr-metadata-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		b, err := selectedBinding(st, r.Target, r.SurfaceID, r.BindingID)
		if err != nil {
			return c, err
		}
		ioctx, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		check, o := s.check(ioctx, st, b, now)
		if check.Status != "current" {
			return c, fmt.Errorf("metadata requires a current live binding (%s): %w", strings.Join(check.Issues, ", "), store.ErrConflict)
		}
		task := st.Tasks[r.Target].Task
		next, direction := continuity.RecordedNextAction(st, r.Target)
		reviews := continuity.RecordedReviews(st, r.Target)
		review := fmt.Sprintf("%d proposals; %d failed; %d stale; %d unknown", reviews.PendingProposals, reviews.FailedEvidence, reviews.StaleEvidence, reviews.UnknownEvidence)
		tokens := map[string]string{"heimdall_task": r.Target, "heimdall_title": displayText(task.Title, 256), "heimdall_next": displayText(next, 512), "heimdall_review": review, "heimdall_binding": b.ID, "heimdall_state": "checked", "heimdall_direction": direction}
		tokens["heimdall_checked_at"] = now.UTC().Format(time.RFC3339)
		title := displayText(task.Title+" | "+next, 180)
		result := MetadataResult{Version: 1, Target: r.Target, BindingID: b.ID, Status: "published", At: now.UTC(), TTLMillis: 30000, Tokens: tokens, WorkspaceSummary: r.WorkspaceSummary}
		if err := s.Adapter.Report(ioctx, o, "heimdall-"+b.ID, st.LastEventID+1, title, tokens, result.TTLMillis, r.WorkspaceSummary); err != nil {
			// The write may have reached Herdr. Preserve the receipt; the same
			// committed request never resends. A crash or failed commit can
			// reapply these idempotent display values; TTL bounds their lifetime.
			result.Status = "unconfirmed"
			result.Issue = "metadata_readback_unconfirmed"
		}
		c.Result = result
		return c, nil
	})
}
