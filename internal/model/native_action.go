package model

import "time"

type NativeDispatchReport struct {
	Submitted    bool `json:"submitted"`
	Acknowledged bool `json:"acknowledged"`
}
type NativeReadback struct {
	Version       int             `json:"version"`
	ActionID      string          `json:"action_id"`
	AttemptID     string          `json:"attempt_id"`
	SourceID      string          `json:"source_id"`
	SourceEpoch   string          `json:"source_epoch"`
	SnapshotID    string          `json:"snapshot_id"`
	AfterEventID  int64           `json:"after_event_id"`
	StartedAt     time.Time       `json:"started_at"`
	CapturedAt    time.Time       `json:"captured_at"`
	ReceivedAt    time.Time       `json:"received_at"`
	Complete      bool            `json:"complete"`
	Window        *DesktopWindow  `json:"window,omitempty"`
	Workspace     string          `json:"workspace,omitempty"`
	FocusKnown    bool            `json:"focus_known"`
	FocusedWindow *WindowIdentity `json:"focused_window,omitempty"`
}

func NativeOutcome(a ActionRecord, r NativeReadback) (string, string) {
	n := a.Intent.Native
	if n == nil || a.Execution == "queued" || a.Execution == "refused" || a.Execution == "cancelled" {
		return "unsupported", "No dispatched native attempt"
	}
	if !r.Complete || r.SourceEpoch != n.SourceEpoch || r.SourceID != n.SourceID || r.AfterEventID < a.LastEventID {
		return "unknown", "Fresh complete native readback after the latest transition required"
	}
	if n.Kind == "open" {
		return "unsupported", "Reviewed application launch and instance association required"
	}
	if r.Window == nil && n.Kind == "close" {
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
