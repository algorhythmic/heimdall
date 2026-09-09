package model

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// DesiredSurface is a stable task-owned identity, never a compositor address or
// application runtime ID. Version 1 only supports manual restoration.
type DesiredSurface struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Label         string `json:"label"`
	Required      bool   `json:"required"`
	RestorePolicy string `json:"restore_policy"`
}

type WorkspaceManifest struct {
	Version      int              `json:"version"`
	ID           string           `json:"id"`
	Target       string           `json:"target"`
	TaskRevision int64            `json:"task_revision"`
	Previous     string           `json:"previous"`
	Name         string           `json:"name"`
	Surfaces     []DesiredSurface `json:"surfaces"`
	Actor        string           `json:"actor"`
	At           time.Time        `json:"at"`
}

// SurfaceIdentity survives removal from a manifest, preventing another task
// from adopting a retired ID or changing its application kind.
type SurfaceIdentity struct {
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

// SessionLocator is an explicit declaration, not a live observation. Even an
// exact locator confers no filesystem, process, or desktop authority. Epochs
// scope runtime IDs; a future adapter must independently verify them.
type SessionLocator struct {
	Adapter        string `json:"adapter"`
	Environment    string `json:"environment"`
	Host           string `json:"host"`
	SourceEpoch    string `json:"source_epoch"`
	SessionID      string `json:"session_id"`
	WorkspaceID    string `json:"workspace_id"`
	PaneID         string `json:"pane_id"`
	Platform       string `json:"platform"`
	Cwd            string `json:"cwd"`
	Repository     string `json:"repository,omitempty"`
	Worktree       string `json:"worktree,omitempty"`
	AgentSessionID string `json:"agent_session_id,omitempty"`
}

type SessionBinding struct {
	Version      int             `json:"version"`
	ID           string          `json:"id"`
	Target       string          `json:"target"`
	TaskRevision int64           `json:"task_revision"`
	ManifestID   string          `json:"manifest_id"`
	SurfaceID    string          `json:"surface_id"`
	Previous     string          `json:"previous"`
	Active       bool            `json:"active"`
	Locator      *SessionLocator `json:"locator,omitempty"`
	Herdr        *HerdrIdentity  `json:"herdr,omitempty"`
	Actor        string          `json:"actor"`
	At           time.Time       `json:"at"`
}

func workspaceText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && len(s) <= max && !strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.In(r, unicode.Cf) || r == '\u2028' || r == '\u2029'
	})
}

func ValidDesiredWorkspace(name string, surfaces []DesiredSurface) error {
	if !workspaceText(name, 256) || surfaces == nil || len(surfaces) > 32 {
		return fmt.Errorf("workspace requires a name and an explicit list of at most 32 surfaces")
	}
	seen := map[string]bool{}
	for _, s := range surfaces {
		if !OpaqueID.MatchString(s.ID) || seen[s.ID] || !Contains([]string{"terminal", "editor", "browser", "native"}, s.Kind) || !workspaceText(s.Label, 256) || s.RestorePolicy != "manual" {
			return fmt.Errorf("invalid or duplicate desired surface; v1 requires manual restore policy")
		}
		seen[s.ID] = true
	}
	return nil
}

// Validate path syntax independently of this daemon's OS so replay and backup
// inspection never depend on the original host or the continued existence of cwd.
func declaredPath(platform, s string) bool {
	if !workspaceText(s, 4096) {
		return false
	}
	if platform == "windows" {
		if len(s) >= 3 && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
			return true
		}
		parts := strings.Split(strings.ReplaceAll(s, "\\", "/"), "/")
		return len(parts) >= 4 && parts[0] == "" && parts[1] == "" && parts[2] != "" && parts[2] != "." && parts[2] != "?" && parts[3] != ""
	}
	return strings.HasPrefix(s, "/")
}

func (l SessionLocator) Validate() error {
	if l.Adapter != "generic" {
		return fmt.Errorf("v1 requires the generic adapter and an explicit supported platform")
	}
	return l.validateFields()
}

func (l SessionLocator) validateFields() error {
	if !Contains([]string{"linux", "darwin", "windows"}, l.Platform) {
		return fmt.Errorf("unsupported session platform")
	}
	for _, s := range []string{l.Environment, l.Host, l.SourceEpoch, l.SessionID, l.WorkspaceID, l.PaneID} {
		if !workspaceText(s, 256) {
			return fmt.Errorf("environment, host, source epoch and complete session/workspace/pane IDs required")
		}
	}
	if !declaredPath(l.Platform, l.Cwd) || (l.Repository == "") != (l.Worktree == "") || (l.Repository != "" && (!declaredPath(l.Platform, l.Repository) || !declaredPath(l.Platform, l.Worktree))) || (l.AgentSessionID != "" && !workspaceText(l.AgentSessionID, 256)) {
		return fmt.Errorf("absolute declared paths and a valid optional agent session ID required; repository and worktree must be supplied together")
	}
	return nil
}

// HerdrIdentity records a bounded observation made by the local adapter when a
// v2 binding was accepted. It is evidence at that time, never a live status flag.
type HerdrIdentity struct {
	Protocol      int    `json:"protocol"`
	ServerVersion string `json:"server_version"`
	TerminalID    string `json:"terminal_id"`
	TabID         string `json:"tab_id"`
	ShellPID      int    `json:"shell_pid"`
	ShellStart    string `json:"shell_start"`
	AgentSource   string `json:"agent_source,omitempty"`
	AgentKind     string `json:"agent_kind,omitempty"`
}

func ValidHerdrIdentity(l SessionLocator, h HerdrIdentity) error {
	if err := l.validateFields(); err != nil {
		return err
	}
	if l.Adapter != "herdr" || l.Environment != "local" || l.Platform != "linux" || !TokenHashPattern.MatchString(l.Host) || !TokenHashPattern.MatchString(l.SourceEpoch) || !strings.HasPrefix(l.SessionID, "/") || h.Protocol != 20 || h.ServerVersion != "0.8.2" || h.ShellPID < 1 || !workspaceText(h.TerminalID, 256) || !workspaceText(h.TabID, 256) || h.ShellStart == "" {
		return fmt.Errorf("invalid Herdr identity")
	}
	for _, c := range h.ShellStart {
		if c < '0' || c > '9' {
			return fmt.Errorf("invalid process start identity")
		}
	}
	if len(h.ShellStart) > 20 || h.ShellStart == "0" {
		return fmt.Errorf("invalid process start identity")
	}
	if l.AgentSessionID != "" {
		if !workspaceText(h.AgentSource, 256) || !workspaceText(h.AgentKind, 256) {
			return fmt.Errorf("agent session provenance required")
		}
	} else if h.AgentSource != "" || h.AgentKind != "" {
		return fmt.Errorf("unexpected agent session provenance")
	}
	return nil
}

func ValidSessionBinding(v SessionBinding) error {
	if err := ValidWorkspaceRecord(1, v.ID, v.Target, v.Previous, v.Actor, v.At, v.TaskRevision); err != nil {
		return err
	}
	switch v.Version {
	case 1:
		if v.Herdr != nil {
			return fmt.Errorf("v1 binding cannot contain Herdr observation")
		}
		if v.Locator != nil {
			return v.Locator.Validate()
		}
	case 2:
		if !v.Active || v.Herdr == nil || v.Locator == nil {
			return fmt.Errorf("v2 binding requires an observed active Herdr session")
		}
		return ValidHerdrIdentity(*v.Locator, *v.Herdr)
	default:
		return fmt.Errorf("unsupported session binding version")
	}
	return nil
}

// SameSessionPane deliberately excludes cwd, display names and workspace ID:
// moving a pane must not let a second task claim it in the same source epoch.
func (l SessionLocator) SameSessionPane(other SessionLocator) bool {
	return l.Adapter == other.Adapter && l.Environment == other.Environment && l.Host == other.Host && l.SourceEpoch == other.SourceEpoch && l.SessionID == other.SessionID && l.PaneID == other.PaneID
}

func ValidWorkspaceRecord(version int, id, target, previous, actor string, at time.Time, revision int64) error {
	if version != 1 || !OpaqueID.MatchString(id) || !ValidID(target) || (previous != "" && !OpaqueID.MatchString(previous)) || actor != "cli" || at.IsZero() || revision < 1 {
		return fmt.Errorf("invalid workspace record version, identity or provenance")
	}
	return nil
}
