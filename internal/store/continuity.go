package store

import (
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"slices"
)

func applyContinuity(st *model.State, e Event) error {
	if e.Actor != "cli" && !(e.Subject == "checkpoint" && e.Verb == "recorded") {
		return fmt.Errorf("invalid continuity actor")
	}
	switch e.Subject + "." + e.Verb {
	case "contract.accepted":
		var v model.Contract
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := model.ValidContract(v); err != nil {
			return err
		}
		if e.EntityID != v.ID || st.ContractHeads[v.Target] != v.Previous {
			return fmt.Errorf("invalid contract chain")
		}
		if _, ok := st.Contracts[v.ID]; ok {
			return fmt.Errorf("duplicate contract")
		}
		r, step, err := model.ResolveTarget(*st, v.Target)
		if err != nil || r.Revision != v.TaskRevision {
			return fmt.Errorf("invalid contract task revision")
		}
		done := r.Task.Done
		if step != nil {
			done = step.Done
		}
		if !reflect.DeepEqual(v.Acceptance, done) {
			return fmt.Errorf("contract acceptance differs from task")
		}
		for _, id := range v.ResourceIDs {
			if r, ok := st.Resources[id]; !ok || !r.Active {
				return fmt.Errorf("unknown contract resource")
			}
		}
		if v.Version == 2 {
			ids, err := model.ResourceScope(*st, v.Target)
			if err != nil || !slices.Equal(ids, v.ResourceIDs) {
				return fmt.Errorf("contract resource scope differs from task lineage")
			}
		}
		st.Contracts[v.ID] = v
		st.ContractHeads[v.Target] = v.ID
	case "decision.accepted":
		var v model.Decision
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := model.ValidRecord(v.Version, v.ID, v.Target, v.Actor, v.At); err != nil {
			return err
		}
		if e.EntityID != v.ID {
			return fmt.Errorf("invalid decision identity")
		}
		if _, ok := st.Decisions[v.ID]; ok {
			return fmt.Errorf("duplicate decision")
		}
		r, _, err := model.ResolveTarget(*st, v.Target)
		if err != nil || r.Revision != v.TaskRevision {
			return fmt.Errorf("invalid decision task revision")
		}
		if v.Supersedes != "" {
			old, ok := st.Decisions[v.Supersedes]
			if !ok || old.Target != v.Target {
				return fmt.Errorf("invalid superseded decision")
			}
			for _, d := range st.Decisions {
				if d.Supersedes == old.ID {
					return fmt.Errorf("decision already superseded")
				}
			}
		}
		st.Decisions[v.ID] = v
	case "resource.bound", "resource.unbound":
		var v model.Resource
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := model.ValidRecord(v.Version, v.ID, v.Target, v.Actor, v.At); err != nil {
			return err
		}
		if e.EntityID != v.ID {
			return fmt.Errorf("invalid resource identity")
		}
		old, exists := st.Resources[v.ID]
		if (e.Verb == "bound" && (exists || !v.Active)) || (e.Verb == "unbound" && (!exists || v.Active)) {
			return fmt.Errorf("invalid resource lifecycle")
		}
		if _, _, err := model.ResolveTarget(*st, v.Target); err != nil {
			return err
		}
		if e.Verb == "unbound" {
			if !old.Active {
				return fmt.Errorf("resource already unbound")
			}
			old.Active = false
			if !reflect.DeepEqual(old, v) {
				return fmt.Errorf("unbind changed immutable resource")
			}
		}
		st.Resources[v.ID] = v
	case "checkpoint.recorded":
		var v model.Checkpoint
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := model.ValidCheckpoint(v); err != nil {
			return err
		}
		if v.Actor != e.Actor || !v.At.Equal(e.TS) {
			return fmt.Errorf("checkpoint author/time differs from envelope")
		}
		if v.Version == 2 {
			g, ok := st.Grants[v.GrantID]
			if !ok || g.RevokedAt != nil || e.TS.Before(g.At) || !e.TS.Before(g.ExpiresAt) || !g.PermitsCheckpoint(*st, v.Target, v.ContractID) {
				return fmt.Errorf("checkpoint outside recorded grant authority")
			}
		}
		if e.EntityID != v.ID || st.CheckpointHeads[v.Target] != v.Previous {
			return fmt.Errorf("invalid checkpoint chain")
		}
		if _, ok := st.Checkpoints[v.ID]; ok {
			return fmt.Errorf("duplicate checkpoint")
		}
		c, ok := st.Contracts[v.ContractID]
		if !ok || c.Target != v.Target || st.ContractHeads[v.Target] != c.ID || c.TaskRevision != v.TaskRevision {
			return fmt.Errorf("invalid checkpoint contract")
		}
		r, _, err := model.ResolveTarget(*st, v.Target)
		if err != nil || r.Revision != v.TaskRevision || v.SourceEvent < 0 || v.SourceEvent >= e.ID {
			return fmt.Errorf("invalid checkpoint source revision")
		}
		for _, id := range v.Decisions {
			if _, ok := st.Decisions[id]; !ok {
				return fmt.Errorf("unknown checkpoint decision")
			}
		}
		for _, ref := range v.Resources {
			if r, ok := st.Resources[ref.ID]; !ok || !r.Active {
				return fmt.Errorf("unknown checkpoint resource")
			}
		}
		if c.Version == 2 {
			ids := []string{}
			for _, ref := range v.Resources {
				ids = append(ids, ref.ID)
			}
			current, err := model.ResourceScope(*st, v.Target)
			if err != nil || !slices.Equal(ids, c.ResourceIDs) || !slices.Equal(ids, current) {
				return fmt.Errorf("checkpoint resource scope differs from contract")
			}
		}
		st.Checkpoints[v.ID] = v
		st.CheckpointHeads[v.Target] = v.ID
	default:
		return fmt.Errorf("unknown continuity event")
	}
	return nil
}
