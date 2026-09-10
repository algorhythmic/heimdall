package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// OwnedWindow is a declaration of exact ownership, not a fresh existence claim.
// Runtime address/title are returned only after registration reads the observer.
type OwnedWindowRuntime struct {
	Address    string    `json:"address"`
	Title      string    `json:"title"`
	Workspace  string    `json:"workspace"`
	CapturedAt time.Time `json:"captured_at"`
}
type OwnedWindow struct {
	Browser   bool                `json:"browser,omitempty"`
	Runtime   *OwnedWindowRuntime `json:"runtime,omitempty"`
	Target    string              `json:"target"`
	SurfaceID string              `json:"surface_id"`
	BindingID string              `json:"binding_id"`
	SourceID  string              `json:"source_id"`
	Window    WindowIdentity      `json:"window"`
	Label     string              `json:"label"`
}
type ExternalIntent struct {
	Browser *ExternalBrowser `json:"browser,omitempty"`
	Owned   OwnedWindow      `json:"owned"`
	Purpose string           `json:"purpose"`
	Steps   int              `json:"steps"`
}
type ExternalObservation struct {
	TraceCount      int    `json:"trace_count,omitempty"`
	TraceDigest     string `json:"trace_digest,omitempty"`
	TraceDurationMS int64  `json:"trace_duration_ms,omitempty"`
	TraceSubmitted  int    `json:"trace_submitted,omitempty"`
	TargetMismatch  bool   `json:"target_mismatch,omitempty"`
	ReportsDigest   string `json:"reports_digest,omitempty"`
	ReportCount     int    `json:"report_count,omitempty"`
	Source          string `json:"source"`
	Revision        string `json:"revision"`
	Freshness       bool   `json:"freshness"`
	Partial         bool   `json:"partial"`
	Digest          string `json:"digest"`
}

func OwnedWindows(st State, target string) []OwnedWindow {
	result := []OwnedWindow{}
	task, _, err := ResolveTarget(st, target)
	if err != nil {
		return result
	}
	manifest := st.WorkspaceManifests[st.WorkspaceHeads[task.Task.ID]]
	if manifest.TaskRevision != task.Revision {
		return result
	}
	for _, surface := range manifest.Surfaces {
		b := st.ViewportBindings[st.ViewportHeads[surface.ID]]
		source := st.DesktopSources[b.SourceID]
		if !b.Active || b.Window == nil || b.Target != task.Task.ID || b.ManifestID != manifest.ID || b.TaskRevision != task.Revision || !source.Active || source.ID != st.DesktopSourceHead || b.Window.SourceEpoch != source.Epoch {
			continue
		}
		result = append(result, OwnedWindow{Browser: surface.Kind == "browser", Target: target, SurfaceID: surface.ID, BindingID: b.ID, SourceID: b.SourceID, Window: *b.Window, Label: surface.Label})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SurfaceID < result[j].SurfaceID })
	return result
}
func ExternalInputsCurrent(st State, v ActionIntent) bool {
	r, _, err := ResolveTarget(st, v.Target)
	if err != nil || v.External == nil || r.Revision != v.TaskRevision || st.WorkspaceHeads[r.Task.ID] != v.ManifestID || ActionContextDigest(st, v.Target) != v.ContextDigest {
		return false
	}
	for _, w := range OwnedWindows(st, v.Target) {
		if w == v.External.Owned {
			return (!w.Browser && v.External.Browser == nil) || (w.Browser && v.External.Browser != nil && ExternalBrowserCurrent(st, *v.External.Browser, w))
		}
	}
	return false
}
func (v ActionIntent) ValidateExternal() error {
	x := v.External
	parts := strings.Split(v.Target, "#")
	if len(parts) > 2 || !ValidID(parts[0]) || (len(parts) == 2 && !localID.MatchString(parts[1])) || v.Version != 6 || x == nil || v.Adapter != "wcu" || v.Authority != "grant" || !OpaqueID.MatchString(v.AuthorityRef) || v.Browser != nil || v.Native != nil || v.Workspace != nil || v.SnapshotID != "" || v.TaskRevision < 1 || !TokenHashPattern.MatchString(v.ContextDigest) || v.At.IsZero() || v.ExpiresAt.Sub(v.At) < time.Second || v.ExpiresAt.Sub(v.At) > 120*time.Second {
		return fmt.Errorf("invalid external intent envelope")
	}
	for _, id := range []string{v.ID, v.ManifestID, v.SurfaceID, v.AttemptID, x.Owned.BindingID, x.Owned.SourceID} {
		if !OpaqueID.MatchString(id) {
			return fmt.Errorf("invalid external identity")
		}
	}
	if x.Owned.Runtime != nil || x.Owned.Target != v.Target || x.Owned.SurfaceID != v.SurfaceID || x.Owned.Window.Validate() != nil || strings.TrimSpace(x.Purpose) == "" || len(x.Purpose) > 512 || x.Steps < 1 || x.Steps > 64 {
		return fmt.Errorf("bounded purpose, steps and exact owned window required")
	}
	if x.Owned.Browser != (x.Browser != nil) || (!x.Owned.Browser && strings.HasPrefix(v.Expected.Kind, "owned_tab_")) {
		return fmt.Errorf("browser postcondition requires an exact associated browser")
	}
	if x.Browser != nil {
		b := x.Browser
		if !OpaqueID.MatchString(b.AssociationID) || !OpaqueID.MatchString(b.Profile) || !OpaqueID.MatchString(b.Epoch) || !OpaqueID.MatchString(b.OwnerID) || b.TabID < 1 || b.WindowID < 1 || b.AfterEventID < 0 {
			return fmt.Errorf("invalid external browser identity")
		}
	}
	if err := ValidateExternalExpected(v.Expected); err != nil {
		return err
	}
	return nil
}
func ValidateExternalExpected(p ActionPostcondition) error {
	if strings.HasPrefix(p.Kind, "owned_tab_") {
		if p.Workspace != "" {
			return fmt.Errorf("foreign browser parameter")
		}
		if p.Kind == "owned_tab_url" && BrowserURL(p.URL) && p.RedirectPolicy == "exact" && Contains([]string{"", "committed", "complete"}, p.LoadCondition) {
			return nil
		}
		if Contains([]string{"owned_tab_absent", "owned_tab_focused"}, p.Kind) && p.URL == "" && p.RedirectPolicy == "" && p.LoadCondition == "" {
			return nil
		}
		return fmt.Errorf("invalid external browser postcondition")
	}
	if !Contains([]string{"window_exists", "window_absent", "window_focused", "window_workspace"}, p.Kind) || p.URL != "" || p.RedirectPolicy != "" || p.LoadCondition != "" || (p.Kind == "window_workspace") != (p.Workspace != "") || len(p.Workspace) > 128 {
		return fmt.Errorf("unsupported external postcondition")
	}
	return nil
}
func ExternalOutcome(st State, a ActionRecord, o *ActionObservation, now time.Time) (string, string) {
	if o == nil || o.Native == nil {
		return "unknown", "Independent desktop observation unavailable"
	}
	r := o.Native
	x := a.Intent.External
	if x == nil || r.Version != 1 || !TokenHashPattern.MatchString(r.SnapshotID) || r.StartedAt.IsZero() || r.ReceivedAt.Before(r.CapturedAt) || now.Before(r.ReceivedAt) || o.SourceEpoch != r.SourceEpoch || !o.ObservedAt.Equal(r.CapturedAt) || !ExternalInputsCurrent(st, a.Intent) || r.ActionID != a.Intent.ID || r.AttemptID != a.Intent.AttemptID || r.SourceID != x.Owned.SourceID || r.SourceEpoch != x.Owned.Window.SourceEpoch || !r.Complete || r.AfterEventID < a.LastEventID || r.StartedAt.Before(a.UpdatedAt) || r.CapturedAt.Before(r.StartedAt) || now.Before(r.CapturedAt) || now.Sub(r.CapturedAt) > 2*time.Second || o.Digest != ContentDigest(r) {
		return "unknown", "Fresh exact-source observation after the latest transition required"
	}
	if r.Window != nil && r.Window.Identity != x.Owned.Window {
		return "unknown", "Observed window identity differs"
	}
	if x.Browser != nil {
		status, detail := ExternalBrowserOutcome(st, a, o, now)
		if status != "" {
			return status, detail
		}
	}
	switch a.Intent.Expected.Kind {
	case "window_absent":
		if r.Window == nil {
			return "matched", "Exact owned window absent; saved data not asserted"
		}
		return "not_matched", "Owned window remains present"
	case "window_exists":
		if r.Window != nil {
			return "matched", "Exact owned window observed"
		}
	case "window_focused", "owned_tab_focused":
		if !r.FocusKnown {
			return "unknown", "Focus coverage unavailable"
		}
		if r.Window != nil && r.FocusedWindow != nil && *r.FocusedWindow == x.Owned.Window {
			return "matched", "Exact owned window focused"
		}
	case "window_workspace":
		if r.Window != nil && r.Workspace == a.Intent.Expected.Workspace {
			return "matched", "Exact owned window in requested workspace"
		}
	}
	return "not_matched", "Independent observation does not match postcondition"
}

func ValidActionTarget(target string) bool {
	parts := strings.Split(target, "#")
	return len(parts) <= 2 && ValidID(parts[0]) && (len(parts) == 1 || localID.MatchString(parts[1]))
}

// A completion check cites an immutable observation, never a submitted report.
func VerifiedAction(st State, target, id string) (ActionRecord, bool) {
	a, ok := st.Actions[id]
	return a, ok && a.Intent.External != nil && a.Intent.Target == target && a.Execution != "external" && a.Verification == "matched" && a.Observation != nil && a.Observation.Status == "matched" && ExternalInputsCurrent(st, a.Intent)
}
func ActionChecks(st State, target string) []string {
	r, step, err := ResolveTarget(st, target)
	if err != nil {
		return nil
	}
	done := r.Task.Done
	if step != nil {
		done = step.Done
	}
	ids := []string{}
	for _, c := range done.Checks {
		if c.Kind == "action.verified" {
			ids = append(ids, c.ActionID)
		}
	}
	return ids
}
