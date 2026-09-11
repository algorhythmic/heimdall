package model

import (
	"fmt"
	"time"
)

// AgentStatus values are Herdr-reported detection states. They describe the
// agent's own terminal status; they never attest task ownership.
var AgentStatuses = map[string]bool{"idle": true, "working": true, "blocked": true, "done": true, "unknown": true}

// AgentRecord is the last observed agent in one epoch-scoped Herdr pane
// container. Active reports the latest recorded observation, not current
// coverage; consumers must check the source epoch and binding freshness.
type AgentRecord struct {
	Version        int       `json:"version"`
	Adapter        string    `json:"adapter"`
	Host           string    `json:"host"`
	SourceEpoch    string    `json:"source_epoch"`
	SessionID      string    `json:"session_id"`
	WorkspaceID    string    `json:"workspace_id"`
	PaneID         string    `json:"pane_id"`
	TabID          string    `json:"tab_id"`
	TerminalID     string    `json:"terminal_id"`
	Cwd            string    `json:"cwd"`
	Agent          string    `json:"agent"`
	AgentSessionID string    `json:"agent_session_id"`
	Status         string    `json:"status"`
	Title          string    `json:"title,omitempty"`
	StateSeq       int64     `json:"state_seq"`
	Active         bool      `json:"active"`
	At             time.Time `json:"at"`
}

// ContainerKey scopes the record to one host/epoch/session/pane container, so
// a Herdr restart or re-bound socket cannot alias old observations.
func (r AgentRecord) ContainerKey() string {
	return r.Host + "|" + r.SourceEpoch + "|" + r.SessionID + "|" + r.PaneID
}

func ValidAgentRecord(r AgentRecord) error {
	if r.Version != 1 || r.Adapter != "herdr" || !AgentStatuses[r.Status] {
		return fmt.Errorf("invalid agent record envelope")
	}
	for _, s := range []string{r.Host, r.SourceEpoch, r.SessionID, r.WorkspaceID, r.PaneID, r.TabID, r.TerminalID, r.Agent, r.AgentSessionID} {
		if s == "" {
			return fmt.Errorf("agent record requires explicit container and agent identity")
		}
	}
	if r.StateSeq < 1 || r.At.IsZero() || (r.Active && r.Cwd == "") {
		return fmt.Errorf("agent record requires a sequence, time and live cwd")
	}
	return nil
}
