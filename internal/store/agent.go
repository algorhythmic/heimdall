package store

import (
	"fmt"
	"heimdall/internal/model"
	"strings"
)

// applyAgent applies epoch-scoped Herdr agent observations. Records are keyed
// by container (host|epoch|socket|pane); a regressed Herdr state sequence or a
// re-sent identical record is refused, never silently folded.
func applyAgent(st *model.State, e Event) error {
	var r model.AgentRecord
	if err := model.StrictJSON(e.Payload, &r); err != nil {
		return err
	}
	key := r.ContainerKey()
	if err := model.ValidAgentRecord(r); err != nil {
		return err
	}
	if e.Actor != "observer:herdr" || e.EntityID != key || !strings.HasPrefix(e.CommandID, "agent-") {
		return fmt.Errorf("invalid agent observation envelope")
	}
	old, exists := st.AgentHeads[key]
	if exists {
		if old.StateSeq > r.StateSeq {
			return fmt.Errorf("agent observation sequence regressed")
		}
		if old.StateSeq == r.StateSeq && old.Status == r.Status && old.AgentSessionID == r.AgentSessionID &&
			old.WorkspaceID == r.WorkspaceID && old.TabID == r.TabID && old.TerminalID == r.TerminalID &&
			old.Cwd == r.Cwd && old.Agent == r.Agent && old.Title == r.Title && old.Active == r.Active {
			return fmt.Errorf("duplicate agent observation")
		}
	}
	if e.Verb == "detached" && (!exists || !old.Active || r.Active) {
		return fmt.Errorf("agent detach requires a previous active observation")
	}
	if e.Verb == "observed" && !r.Active {
		return fmt.Errorf("agent observation requires an active record")
	}
	st.AgentHeads[key] = r
	return nil
}
