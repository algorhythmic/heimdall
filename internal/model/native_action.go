package model

import "time"

type NativeDispatchReport struct {
	Process      *ApplicationProcess `json:"process,omitempty"`
	Submitted    bool                `json:"submitted"`
	Acknowledged bool                `json:"acknowledged"`
}
type NativeReadback struct {
	SessionBindingID string              `json:"session_binding_id,omitempty"`
	SessionDigest    string              `json:"session_digest,omitempty"`
	Process          *ApplicationProcess `json:"process,omitempty"`
	LaunchCandidates int                 `json:"launch_candidates,omitempty"`
	Version          int                 `json:"version"`
	ActionID         string              `json:"action_id"`
	AttemptID        string              `json:"attempt_id"`
	SourceID         string              `json:"source_id"`
	SourceEpoch      string              `json:"source_epoch"`
	SnapshotID       string              `json:"snapshot_id"`
	AfterEventID     int64               `json:"after_event_id"`
	StartedAt        time.Time           `json:"started_at"`
	CapturedAt       time.Time           `json:"captured_at"`
	ReceivedAt       time.Time           `json:"received_at"`
	Complete         bool                `json:"complete"`
	Window           *DesktopWindow      `json:"window,omitempty"`
	Workspace        string              `json:"workspace,omitempty"`
	FocusKnown       bool                `json:"focus_known"`
	FocusedWindow    *WindowIdentity     `json:"focused_window,omitempty"`
}

func NativeOutcomeInState(st State, a ActionRecord, r NativeReadback) (string, string) {
	status, detail := NativeOutcome(a, r)
	if status == "matched" && a.Intent.Native != nil && a.Intent.Native.Launch != nil && !ApplicationAssociationCurrent(st, a.Intent) {
		return "unknown", "Launch scope changed; observed process cannot acquire current ownership"
	}
	return status, detail
}

func NativeOutcome(a ActionRecord, r NativeReadback) (string, string) {
	n := a.Intent.Native
	if n == nil || a.Execution == "queued" || a.Execution == "refused" || a.Execution == "cancelled" {
		return "unsupported", "No dispatched native attempt"
	}
	if !r.Complete || r.SourceEpoch != n.SourceEpoch || r.SourceID != n.SourceID || r.AfterEventID < a.LastEventID {
		return "unknown", "Fresh complete native readback after the latest transition required"
	}
	if n.SessionBindingID != "" && (r.SessionBindingID != n.SessionBindingID || r.SessionDigest == "") {
		return "unknown", "Original Herdr session survival not independently observed"
	}
	if n.Kind == "open" {
		if n.Launch == nil {
			return "unsupported", "Reviewed application launch and instance association required"
		}
		if a.Report == nil || a.Report.Native == nil || a.Report.Native.Process == nil {
			return "unknown", "Launch receipt missing; possible process is never relaunched"
		}
		if r.Process == nil || *r.Process != *a.Report.Native.Process || r.LaunchCandidates != 1 || r.Window == nil {
			return "unknown", "Unique window and original launched process identity not yet verified"
		}
		if n.Launch.SessionBindingID != "" {
			if r.SessionBindingID != n.Launch.SessionBindingID || r.SessionDigest == "" {
				return "unknown", "Original Herdr session survival not independently observed"
			}
			return "matched", "New direct-attach view and surviving Herdr session observed; attachment rendering remains unverified"
		}
		if n.Launch.Editor {
			return "matched", "New editor view observed; saved files and cursor requested, unsaved buffers and editor state not verified"
		}
		return "matched", "New terminal independently associated; previous processes were not resumed"
	}
	if r.Window == nil && n.Kind == "close" {
		if n.SessionBindingID != "" {
			return "matched", "Exact attached view is absent and original Herdr pane process survives"
		}
		return "matched", "Exact owned native window is absent; application data preservation is not asserted"
	}
	if a.Report == nil {
		return "unknown", "Native dispatch acknowledgment was lost; attempt remains uncertain"
	}
	if r.Window == nil {
		return "not_matched", "Exact owned native window is absent"
	}
	if n.Kind == "close" {
		return "not_matched", "Exact owned native window remains open"
	}
	if n.Kind == "focus" {
		if !r.FocusKnown {
			return "unknown", "Independent active-window observation unavailable"
		}
		if r.FocusedWindow == nil || *r.FocusedWindow != *n.Window {
			return "not_matched", "Exact owned native window is not focused"
		}
	}
	if n.Kind == "move" && r.Workspace != n.Workspace {
		return "not_matched", "Native window is outside the requested named workspace"
	}
	return "matched", "Requested native postcondition independently observed"
}
