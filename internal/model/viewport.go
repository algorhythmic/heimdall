package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// DesktopSource is explicitly selected by the local operator. Its epoch binds
// host, boot, peer process start and both socket generations, never a title.
type DesktopSource struct {
	Version           int       `json:"version"`
	ID                string    `json:"id"`
	Previous          string    `json:"previous"`
	Active            bool      `json:"active"`
	SocketDir         string    `json:"socket_dir"`
	Host              string    `json:"host"`
	Epoch             string    `json:"epoch"`
	CompositorVersion string    `json:"compositor_version"`
	Actor             string    `json:"actor"`
	At                time.Time `json:"at"`
}

type WindowIdentity struct {
	SourceEpoch string `json:"source_epoch"`
	StableID    string `json:"stable_id"`
}

var nativeWindowID = regexp.MustCompile(`^[1-9a-f][0-9a-f]{0,15}$`)

func (w WindowIdentity) Validate() error {
	if !TokenHashPattern.MatchString(w.SourceEpoch) || !nativeWindowID.MatchString(w.StableID) {
		return fmt.Errorf("invalid compositor window identity")
	}
	return nil
}

type ViewportBinding struct {
	ApplicationActionID  string          `json:"application_action_id,omitempty"`
	BrowserAssociationID string          `json:"browser_association_id,omitempty"`
	Version              int             `json:"version"`
	ID                   string          `json:"id"`
	Target               string          `json:"target"`
	TaskRevision         int64           `json:"task_revision"`
	ManifestID           string          `json:"manifest_id"`
	SurfaceID            string          `json:"surface_id"`
	Previous             string          `json:"previous"`
	Active               bool            `json:"active"`
	SourceID             string          `json:"source_id"`
	SnapshotID           string          `json:"snapshot_id"`
	Window               *WindowIdentity `json:"window,omitempty"`
	// This joins two explicit declarations; it does not prove that a terminal
	// window currently displays the bound pane. Herdr refresh supplies pane facts.
	SessionBindingID string    `json:"session_binding_id,omitempty"`
	Actor            string    `json:"actor"`
	At               time.Time `json:"at"`
}

func (s DesktopSource) Validate() error {
	if s.Version != 1 || !OpaqueID.MatchString(s.ID) || (s.Previous != "" && !OpaqueID.MatchString(s.Previous)) || s.Actor != "cli" || s.At.IsZero() {
		return fmt.Errorf("invalid desktop source envelope")
	}
	if s.Active {
		if !strings.HasPrefix(s.SocketDir, "/") || !workspaceText(s.SocketDir, 4096) || !TokenHashPattern.MatchString(s.Host) || !TokenHashPattern.MatchString(s.Epoch) || s.CompositorVersion != "0.56.2" {
			return fmt.Errorf("invalid supported desktop source")
		}
	} else if s.SocketDir != "" || s.Host != "" || s.Epoch != "" || s.CompositorVersion != "" {
		return fmt.Errorf("inactive source cannot contain a locator")
	}
	return nil
}
func (b ViewportBinding) Validate() error {

	version, actor := b.Version, b.Actor
	if b.Version == 3 {
		if !b.Active || b.Actor != "coordinator" || !OpaqueID.MatchString(b.ApplicationActionID) || b.BrowserAssociationID != "" {
			return fmt.Errorf("application viewport requires observed launch association")
		}
		version, actor = 1, "cli"
	} else if b.ApplicationActionID != "" {
		return fmt.Errorf("application association requires viewport version 3")
	} else if b.Version == 2 {
		if !b.Active || b.Actor != "coordinator" || !OpaqueID.MatchString(b.BrowserAssociationID) {
			return fmt.Errorf("browser viewport requires a coordinator association")
		}
		version, actor = 1, "cli"
	} else if b.BrowserAssociationID != "" {
		return fmt.Errorf("association requires viewport version 2")
	}
	if err := ValidWorkspaceRecord(version, b.ID, b.Target, b.Previous, actor, b.At, b.TaskRevision); err != nil {
		return err
	}
	if !OpaqueID.MatchString(b.ManifestID) || !OpaqueID.MatchString(b.SurfaceID) {
		return fmt.Errorf("viewport manifest and surface required")
	}
	if b.Active {
		if b.Window == nil || !OpaqueID.MatchString(b.SourceID) || !TokenHashPattern.MatchString(b.SnapshotID) || (b.SessionBindingID != "" && !OpaqueID.MatchString(b.SessionBindingID)) {
			return fmt.Errorf("viewport observation identity required")
		}
		return b.Window.Validate()
	}
	if b.Window != nil || b.SourceID != "" || b.SnapshotID != "" || b.SessionBindingID != "" {
		return fmt.Errorf("inactive viewport cannot contain observation")
	}
	return nil
}

type DesktopMonitor struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	X                int     `json:"x"`
	Y                int     `json:"y"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	Scale            float64 `json:"scale"`
	Transform        int     `json:"transform"`
	ActiveWorkspace  int     `json:"active_workspace"`
	SpecialWorkspace int     `json:"special_workspace"`
}
type DesktopWorkspace struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	MonitorID   int    `json:"monitor_id"`
	MonitorName string `json:"monitor_name"`
}
type DesktopWindow struct {
	Identity    WindowIdentity `json:"identity"`
	Address     string         `json:"address"`
	PID         int            `json:"pid"`
	Class       string         `json:"class"`
	Title       string         `json:"title"`
	WorkspaceID int            `json:"workspace_id"`
	MonitorID   int            `json:"monitor_id"`
	At          [2]int         `json:"at"`
	Size        [2]int         `json:"size"`
	Floating    bool           `json:"floating"`
	Pinned      bool           `json:"pinned"`
	Hidden      bool           `json:"hidden"`
	Fullscreen  int            `json:"fullscreen"`
}

// Live observations are bounded memory in W02; they are not restoration points.
// Coordinates are Hyprland global logical window coordinates. Monitor width and
// height are physical pixels with scale/transform separately recorded.
type DesktopSnapshot struct {
	StartedAt         time.Time          `json:"started_at"`
	Version           int                `json:"version"`
	ID                string             `json:"id"`
	SourceEpoch       string             `json:"source_epoch"`
	Host              string             `json:"host"`
	CompositorVersion string             `json:"compositor_version"`
	CapturedAt        time.Time          `json:"captured_at"`
	Monitors          []DesktopMonitor   `json:"monitors"`
	Workspaces        []DesktopWorkspace `json:"workspaces"`
	Windows           []DesktopWindow    `json:"windows"`
}
