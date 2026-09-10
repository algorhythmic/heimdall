package model

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// BrowserFocusSpan is an extension's sampled attention interval. It is not an
// input audit, a continuous OS focus guarantee, task binding or action proof.
type BrowserFocusSpan struct {
	Version         int       `json:"version"`
	TabID           int       `json:"tab_id"`
	WindowID        int       `json:"window_id"`
	Pointer         string    `json:"pointer"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds float64   `json:"duration_s"`
}

func (f BrowserFocusSpan) Validate() error {
	if f.Version != 1 || f.TabID < 1 || f.WindowID < 1 || !BrowserURL(f.Pointer) || f.StartedAt.IsZero() || f.EndedAt.IsZero() ||
		math.IsNaN(f.DurationSeconds) || math.IsInf(f.DurationSeconds, 0) || f.DurationSeconds < 2 || f.DurationSeconds > 86400 ||
		math.Abs(f.EndedAt.Sub(f.StartedAt).Seconds()-f.DurationSeconds) > 0.000001 {
		return fmt.Errorf("invalid sampled browser focus span")
	}
	return nil
}

type SurfaceFocusSpan struct {
	BrowserFocusSpan
	Profile    string `json:"profile"`
	Epoch      string `json:"epoch"`
	Connection string `json:"connection"`
	Sequence   int64  `json:"sequence"`
	SurfaceID  string `json:"surface_id"`
}

type CompositorSurfaceFocusSpan struct {
	Version                 int            `json:"version"`
	SourceID                string         `json:"source_id"`
	SourceEpoch             string         `json:"source_epoch"`
	Sequence                int64          `json:"sequence"`
	Window                  WindowIdentity `json:"window"`
	Class                   string         `json:"class"`
	CompositorWorkspaceID   int            `json:"compositor_workspace_id"`
	CompositorWorkspaceName string         `json:"compositor_workspace_name"`
	Title                   string         `json:"title"`
	StartedAt               time.Time      `json:"started_at"`
	EndedAt                 time.Time      `json:"ended_at"`
	DurationSeconds         float64        `json:"duration_s"`
	Gaps                    []string       `json:"gaps"`
}

func (f CompositorSurfaceFocusSpan) Validate() error {
	if f.Version != 1 || !OpaqueID.MatchString(f.SourceID) || !TokenHashPattern.MatchString(f.SourceEpoch) || f.Sequence < 1 || f.Window.SourceEpoch != f.SourceEpoch || f.Window.Validate() != nil || f.StartedAt.IsZero() || f.EndedAt.IsZero() || math.IsNaN(f.DurationSeconds) || math.IsInf(f.DurationSeconds, 0) || f.DurationSeconds < 2 || f.DurationSeconds > 86400 || math.Abs(f.EndedAt.Sub(f.StartedAt).Seconds()-f.DurationSeconds) > 0.000001 || !focusText(f.Class) || !focusText(f.Title) || !focusGaps(f.Gaps) {
		return fmt.Errorf("invalid sampled compositor surface focus span")
	}
	if (f.Class == "" && !containsFocusGap(f.Gaps, "class_unknown")) || (f.CompositorWorkspaceName == "" && !containsFocusGap(f.Gaps, "compositor_workspace_unknown")) {
		return fmt.Errorf("compositor focus fields require explicit gaps")
	}
	return nil
}

type CompositorWorkspaceFocusSpan struct {
	Version                 int       `json:"version"`
	SourceID                string    `json:"source_id"`
	SourceEpoch             string    `json:"source_epoch"`
	Sequence                int64     `json:"sequence"`
	CompositorWorkspaceID   int       `json:"compositor_workspace_id"`
	CompositorWorkspaceName string    `json:"compositor_workspace_name"`
	Monitor                 string    `json:"monitor"`
	StartedAt               time.Time `json:"started_at"`
	EndedAt                 time.Time `json:"ended_at"`
	DurationSeconds         float64   `json:"duration_s"`
	Gaps                    []string  `json:"gaps"`
}

func (f CompositorWorkspaceFocusSpan) Validate() error {
	if f.Version != 1 || !OpaqueID.MatchString(f.SourceID) || !TokenHashPattern.MatchString(f.SourceEpoch) || f.Sequence < 1 || f.CompositorWorkspaceName == "" || !focusText(f.Monitor) || f.StartedAt.IsZero() || f.EndedAt.IsZero() || math.IsNaN(f.DurationSeconds) || math.IsInf(f.DurationSeconds, 0) || f.DurationSeconds < 2 || f.DurationSeconds > 86400 || math.Abs(f.EndedAt.Sub(f.StartedAt).Seconds()-f.DurationSeconds) > 0.000001 || !focusGaps(f.Gaps) {
		return fmt.Errorf("invalid sampled compositor workspace focus span")
	}
	if f.Monitor == "" && !containsFocusGap(f.Gaps, "compositor_monitor_unknown") {
		return fmt.Errorf("compositor workspace monitor requires explicit gap")
	}
	return nil
}

func focusText(v string) bool { return len(v) <= 512 }
func containsFocusGap(gaps []string, want string) bool {
	for _, gap := range gaps {
		if gap == want {
			return true
		}
	}
	return false
}
func focusGaps(gaps []string) bool {
	if gaps == nil || len(gaps) > 8 || !sort.StringsAreSorted(gaps) {
		return false
	}
	for i, gap := range gaps {
		if gap == "" || len(gap) > 64 || (i > 0 && gaps[i-1] == gap) {
			return false
		}
	}
	return true
}
