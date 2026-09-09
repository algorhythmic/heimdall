package store

import (
	"fmt"
	"heimdall/internal/model"
)

func applySnapshot(st *model.State, e Event) error {
	if e.CommandID != "snapshot-"+e.EntityID && e.Verb != "captured" {
		return fmt.Errorf("invalid snapshot command identity")
	}
	switch e.Verb {
	case "captured":
		var v model.WorkspacePoint
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.Kind == "operation" {
			op := st.WorkspaceOperations[v.OperationID]
			target := op.Intent.Target
			if op.Intent.Swap != nil {
				target = op.Intent.Swap.Target
			}
			if e.CommandID != "workspace-operation-"+v.OperationID || op.Revision != 1 || op.CloseSnapshotID != "" || op.DiffID == "" || target != v.Target || (op.Intent.Kind != "close" && op.Intent.Swap == nil) || !v.At.Equal(op.Intent.At) {
				return fmt.Errorf("close capture lacks its reviewed operation")
			}
		} else if e.CommandID != "snapshot-"+e.EntityID {
			return fmt.Errorf("invalid snapshot command identity")
		}
		if v.ID != e.EntityID || v.Actor != e.Actor || !v.At.Equal(e.TS) || st.Tasks[v.Target].Revision != v.TaskRevision || st.WorkspaceHeads[v.Target] != v.ManifestID || st.WorkspaceManifests[v.ManifestID].TaskRevision != v.TaskRevision || st.DesktopSourceHead != v.SourceID || !st.DesktopSources[v.SourceID].Active || st.DesktopSources[v.SourceID].Epoch != v.SourceEpoch || st.SnapshotHeads[v.Target].ID != v.PreviousHead || model.SnapshotInputDigest(*st, v.Target) != v.InputDigest {
			return fmt.Errorf("snapshot capture input changed")
		}
		if v.Kind == "automatic" {
			policy := st.SnapshotPolicies[v.Target]
			if policy.ID != v.PolicyID || !policy.Enabled || policy.ManifestID != v.ManifestID || policy.TaskRevision != v.TaskRevision || policy.SourceID != v.SourceID {
				return fmt.Errorf("snapshot capture policy changed")
			}
		}
		if v.Kind == "manual" {
			count := 0
			for _, p := range st.SnapshotPins {
				if p.Target == v.Target {
					count++
				}
			}
			if count >= 64 {
				return fmt.Errorf("manual pin limit (64) reached; release a pin first")
			}
			st.SnapshotPins[v.ID] = model.SnapshotPin{Version: 1, ID: v.ID, Target: v.Target, SnapshotID: v.ID, Active: true, Reason: "manual restore point", Actor: "cli", At: v.At}
		}
		if v.Published {
			st.SnapshotHeads[v.Target] = v
		}
		if v.Kind == "operation" {
			op := st.WorkspaceOperations[v.OperationID]
			op.CloseSnapshotID = v.ID
			st.WorkspaceOperations[v.OperationID] = op
		}
	case "policy":
		var v model.SnapshotPolicy
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != e.EntityID || v.Actor != e.Actor || !v.At.Equal(e.TS) || st.SnapshotPolicies[v.Target].ID != v.Previous || st.Tasks[v.Target].Revision != v.TaskRevision {
			return fmt.Errorf("snapshot policy envelope or revision")
		}
		if v.Enabled {
			if st.WorkspaceHeads[v.Target] != v.ManifestID || st.WorkspaceManifests[v.ManifestID].TaskRevision != v.TaskRevision || st.DesktopSourceHead != v.SourceID || !st.DesktopSources[v.SourceID].Active {
				return fmt.Errorf("snapshot policy requires current reviewed manifest and source")
			}
			count := 0
			for target, p := range st.SnapshotPolicies {
				if target != v.Target && p.Enabled {
					count++
				}
			}
			if count >= 32 {
				return fmt.Errorf("automatic capture policy limit (32) reached")
			}
		}
		st.SnapshotPolicies[v.Target] = v
	case "pin":
		var v model.SnapshotPin
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := v.Validate(); err != nil {
			return err
		}
		old := st.SnapshotPins[v.SnapshotID]
		if v.ID != e.EntityID || v.Actor != e.Actor || !v.At.Equal(e.TS) || old.ID != v.Previous || old.Active == v.Active || (old.ID != "" && old.Target != v.Target) {
			return fmt.Errorf("snapshot pin envelope or head")
		}
		if v.Active {
			count := 0
			for _, p := range st.SnapshotPins {
				if p.Target == v.Target {
					count++
				}
			}
			if count >= 64 {
				return fmt.Errorf("manual pin limit (64) reached")
			}
			st.SnapshotPins[v.SnapshotID] = v
		} else {
			delete(st.SnapshotPins, v.SnapshotID)
		}
	case "pruned":
		var v model.SnapshotPrune
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != e.EntityID || v.Actor != e.Actor || !v.At.Equal(e.TS) {
			return fmt.Errorf("snapshot retention envelope")
		}
		if v.Actor == "snapshotter" && (st.SnapshotHeads[v.Target].ID != v.ID || st.SnapshotHeads[v.Target].Kind != "automatic") {
			return fmt.Errorf("automatic pruning requires its just-published snapshot")
		}
		for _, id := range v.SnapshotIDs {
			if model.SnapshotProtected(*st, id) {
				return fmt.Errorf("snapshot has a protected head or durable pin")
			}
		}
	default:
		return fmt.Errorf("unknown snapshot event")
	}
	return nil
}
