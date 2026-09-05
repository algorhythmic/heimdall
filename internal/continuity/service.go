// Package continuity preserves explicit contracts, decisions and resumable checkpoints.
package continuity

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/authz"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"slices"
	"sort"
	"strings"
	"time"
)

const MaxRequest = 64 << 10

type ContractInput struct {
	Previous    string   `json:"previous"`
	Objective   string   `json:"objective"`
	Constraints []string `json:"constraints"`
	ResourceIDs []string `json:"resource_ids,omitempty"`
}
type DecisionInput struct {
	Text       string `json:"text"`
	Supersedes string `json:"supersedes,omitempty"`
}
type ResourceInput struct {
	Kind    string   `json:"kind"`
	Root    string   `json:"root"`
	Path    string   `json:"path"`
	Exclude []string `json:"exclude,omitempty"`
}
type CheckpointInput struct {
	Previous    string   `json:"previous"`
	ContractID  string   `json:"contract_id"`
	Summary     string   `json:"summary"`
	CurrentStep string   `json:"current_step,omitempty"`
	NextAction  string   `json:"next_action"`
	Blockers    []string `json:"blockers"`
}
type Request struct {
	Version              int              `json:"version"`
	ID                   string           `json:"id"`
	Op                   string           `json:"op"`
	Target               string           `json:"target"`
	ExpectedTaskRevision *int64           `json:"expected_task_revision"`
	Contract             *ContractInput   `json:"contract,omitempty"`
	Decision             *DecisionInput   `json:"decision,omitempty"`
	Resource             *ResourceInput   `json:"resource,omitempty"`
	Checkpoint           *CheckpointInput `json:"checkpoint,omitempty"`
	ResourceID           string           `json:"resource_id,omitempty"`
}

func Decode(body []byte) (Request, error) {
	var r Request
	if len(body) > MaxRequest {
		return r, fmt.Errorf("continuity request exceeds 64 KiB")
	}
	if err := model.StrictJSON(body, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}
func head(s string) (string, error) {
	if s == "none" {
		return "", nil
	}
	if !model.OpaqueID.MatchString(s) {
		return "", fmt.Errorf("previous must be an ID or the explicit sentinel none")
	}
	return s, nil
}
func textValid(s string, max int) bool { return strings.TrimSpace(s) != "" && len(s) <= max }
func linesValid(lines []string) bool {
	if len(lines) > 128 {
		return false
	}
	for _, s := range lines {
		if !textValid(s, 4096) {
			return false
		}
	}
	return true
}
func (r Request) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || r.ExpectedTaskRevision == nil || *r.ExpectedTaskRevision < 1 {
		return fmt.Errorf("version, request ID and expected task revision required")
	}
	n := 0
	for _, yes := range []bool{r.Contract != nil, r.Decision != nil, r.Resource != nil, r.Checkpoint != nil, r.ResourceID != ""} {
		if yes {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("exactly one operation payload required")
	}
	switch r.Op {
	case "contract.accept":
		if r.Contract == nil || !textValid(r.Contract.Objective, 8192) || !linesValid(r.Contract.Constraints) {
			return fmt.Errorf("invalid contract")
		}
		if len(r.Contract.ResourceIDs) > 16 {
			return fmt.Errorf("contract scope exceeds 16 resources")
		}
		for i, id := range r.Contract.ResourceIDs {
			if !model.OpaqueID.MatchString(id) || (i > 0 && r.Contract.ResourceIDs[i-1] >= id) {
				return fmt.Errorf("contract resource IDs must be unique and sorted")
			}
		}
		_, err := head(r.Contract.Previous)
		return err
	case "decision.accept":
		if r.Decision == nil || !textValid(r.Decision.Text, 8192) || (r.Decision.Supersedes != "" && !model.OpaqueID.MatchString(r.Decision.Supersedes)) {
			return fmt.Errorf("invalid decision")
		}
	case "resource.bind":
		if r.Resource == nil || r.Resource.Root == "" || !model.Contains([]string{"file", "tree"}, r.Resource.Kind) {
			return fmt.Errorf("resource root and kind required")
		}
	case "resource.unbind":
		if !model.OpaqueID.MatchString(r.ResourceID) {
			return fmt.Errorf("resource ID required")
		}
	case "checkpoint.record":
		if r.Checkpoint == nil || !model.OpaqueID.MatchString(r.Checkpoint.ContractID) || !textValid(r.Checkpoint.Summary, 16<<10) || !textValid(r.Checkpoint.NextAction, 8192) || !linesValid(r.Checkpoint.Blockers) {
			return fmt.Errorf("invalid checkpoint")
		}
		_, err := head(r.Checkpoint.Previous)
		return err
	default:
		return fmt.Errorf("unsupported continuity operation")
	}
	return nil
}

type Service struct{ Store *store.Store }

func (s Service) Execute(ctx context.Context, r Request, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("continuity mutation requires user CLI authority")
	}
	return s.execute(ctx, r, actor, now, "", nil)
}

// ExecuteClient derives identity from the credential and rechecks it inside the
// transaction before dedupe and commit. Checkpoint summaries are progress claims.
func (s Service) ExecuteClient(ctx context.Context, r Request, token string, clock func() time.Time) (json.RawMessage, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	g, err := authz.Authenticate(st, token, clock().UTC())
	if err != nil {
		return nil, err
	}
	guard := func(st model.State) error {
		current, err := authz.Authenticate(st, token, clock().UTC())
		if err != nil {
			return err
		}
		if current.ID != g.ID || r.Op != "checkpoint.record" || r.Checkpoint == nil || !current.PermitsCheckpoint(st, r.Target, r.Checkpoint.ContractID) {
			return authz.ErrDenied
		}
		if saved, ok := st.Checkpoints[r.ID]; ok {
			if saved.GrantID != g.ID || saved.Target != r.Target {
				return authz.ErrDenied
			}
			for _, v := range saved.Context {
				if !current.Contains(st, v.Target) {
					return authz.ErrDenied
				}
			}
			for _, v := range saved.Resources {
				if !model.Contains(current.ResourceIDs, v.ID) {
					return authz.ErrDenied
				}
			}
		}
		return nil
	}
	if err = guard(st); err != nil {
		return nil, err
	}
	return s.execute(ctx, r, "client:"+g.ID, clock().UTC(), g.ID, guard)
}

func (s Service) execute(ctx context.Context, r Request, actor string, now time.Time, grantID string, authorize func(model.State) error) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := r.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxRequest {
		return nil, fmt.Errorf("continuity request exceeds 64 KiB")
	}
	commandID := "continuity-" + r.ID
	if grantID != "" {
		commandID = "continuity-client-" + grantID + "-" + r.ID
	}
	return s.Store.TransactChecked(ctx, commandID, actor, raw, now, authorize, func(st model.State) (store.Change, error) {
		change := store.Change{Revision: st.Revision}
		task, step, err := model.ResolveTarget(st, r.Target)
		if err != nil {
			return change, err
		}
		if task.Revision != *r.ExpectedTaskRevision {
			return change, fmt.Errorf("task revision: %w", store.ErrConflict)
		}
		emit := func(subject, verb, id string, v any) (store.Change, error) {
			encoded, err := json.Marshal(v)
			if err != nil {
				return change, err
			}
			if len(encoded) > MaxRequest {
				return change, fmt.Errorf("record exceeds 64 KiB")
			}
			change.Events = []store.Pending{{Subject: subject, Verb: verb, EntityID: id, Payload: v}}
			change.Result = v
			return change, nil
		}
		switch r.Op {
		case "contract.accept":
			previous, _ := head(r.Contract.Previous)
			if st.ContractHeads[r.Target] != previous {
				return change, fmt.Errorf("contract head: %w", store.ErrConflict)
			}
			acceptance := task.Task.Done
			if step != nil {
				acceptance = step.Done
			}
			targets, err := lineage(st, r.Target)
			if err != nil {
				return change, err
			}
			scope := resourceIDs(st, targets)
			if !slices.Equal(r.Contract.ResourceIDs, scope) {
				return change, fmt.Errorf("resource scope changed; supply reviewed resource_ids: %w", store.ErrConflict)
			}
			if len(scope) > 16 {
				return change, fmt.Errorf("contract scope exceeds 16 resources")
			}
			v := model.Contract{Version: 2, ID: r.ID, Target: r.Target, TaskRevision: task.Revision, Previous: previous, Objective: r.Contract.Objective, Constraints: nonnull(r.Contract.Constraints), Acceptance: acceptance, ResourceIDs: scope, Actor: actor, At: now.UTC()}
			return emit("contract", "accepted", v.ID, v)
		case "decision.accept":
			if r.Decision.Supersedes != "" {
				old, ok := st.Decisions[r.Decision.Supersedes]
				if !ok || old.Target != r.Target {
					return change, fmt.Errorf("superseded decision must belong to target")
				}
				for _, d := range st.Decisions {
					if d.Supersedes == old.ID {
						return change, fmt.Errorf("decision already superseded: %w", store.ErrConflict)
					}
				}
			}
			v := model.Decision{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: task.Revision, Text: r.Decision.Text, Supersedes: r.Decision.Supersedes, Actor: actor, At: now.UTC()}
			return emit("decision", "accepted", v.ID, v)
		case "resource.bind":
			targets, err := lineage(st, r.Target)
			if err != nil {
				return change, err
			}
			if len(resources(st, targets)) >= 16 {
				return change, fmt.Errorf("target resource limit (16) exceeded")
			}
			spec := r.Resource
			v := model.Resource{Version: 1, ID: r.ID, Target: r.Target, Kind: spec.Kind, Root: spec.Root, Path: spec.Path, Exclude: append([]string{}, spec.Exclude...), Active: true, Actor: actor, At: now.UTC()}
			if err = normalizeResource(&v); err != nil {
				return change, err
			}
			v.Initial, err = Observe(ctx, v)
			if err != nil {
				return change, err
			}
			return emit("resource", "bound", v.ID, v)
		case "resource.unbind":
			v, ok := st.Resources[r.ResourceID]
			if !ok || v.Target != r.Target || !v.Active {
				return change, fmt.Errorf("resource is not an active binding of target")
			}
			v.Active = false
			return emit("resource", "unbound", v.ID, v)
		case "checkpoint.record":
			c := r.Checkpoint
			previous, _ := head(c.Previous)
			if st.CheckpointHeads[r.Target] != previous {
				return change, fmt.Errorf("checkpoint head: %w", store.ErrConflict)
			}
			contract, ok := st.Contracts[c.ContractID]
			if !ok || contract.Target != r.Target || st.ContractHeads[r.Target] != c.ContractID || contract.TaskRevision != task.Revision {
				return change, fmt.Errorf("current accepted contract required: %w", store.ErrConflict)
			}
			if c.CurrentStep != "" {
				_, _, err = model.ResolveTarget(st, task.Task.ID+"#"+c.CurrentStep)
				if err != nil {
					return change, err
				}
				if step != nil && c.CurrentStep != step.ID {
					return change, fmt.Errorf("checkpoint current step differs from target")
				}
			}
			targets, err := lineage(st, r.Target)
			if err != nil {
				return change, err
			}
			v := model.Checkpoint{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: task.Revision, ContractID: c.ContractID, Previous: previous, SourceEvent: st.LastEventID, Summary: c.Summary, CurrentStep: c.CurrentStep, NextAction: c.NextAction, Blockers: nonnull(c.Blockers), Resources: []model.ResourceVersion{}, Context: versions(st, targets), Decisions: decisionIDs(st, targets), Actor: actor, At: now.UTC()}
			if grantID != "" {
				v.Version = 2
				v.GrantID = grantID
			}
			bound := resources(st, targets)
			if contract.Version != 2 || !slices.Equal(contract.ResourceIDs, resourceIDs(st, targets)) {
				return change, fmt.Errorf("resource scope changed or unreviewed; reaccept contract: %w", store.ErrConflict)
			}
			if len(bound) > 16 {
				return change, fmt.Errorf("target lineage exceeds 16 resource bindings")
			}
			for _, resource := range bound {
				snapshot, err := Observe(ctx, resource)
				if err != nil {
					return change, fmt.Errorf("resource %s: %w", resource.ID, err)
				}
				v.Resources = append(v.Resources, model.ResourceVersion{ID: resource.ID, Snapshot: snapshot})
			}
			return emit("checkpoint", "recorded", v.ID, v)
		}
		return change, fmt.Errorf("unsupported operation")
	})
}
func nonnull(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
func lineage(st model.State, target string) ([]string, error) { return model.TargetLineage(st, target) }
func versions(st model.State, targets []string) []model.ContextVersion {
	v := []model.ContextVersion{}
	for _, t := range targets {
		r, _, _ := model.ResolveTarget(st, t)
		v = append(v, model.ContextVersion{Target: t, TaskRevision: r.Revision, ContractID: st.ContractHeads[t]})
	}
	return v
}
func decisionIDs(st model.State, targets []string) []string {
	superseded := map[string]bool{}
	for _, d := range st.Decisions {
		if d.Supersedes != "" {
			superseded[d.Supersedes] = true
		}
	}
	ids := []string{}
	for id, d := range st.Decisions {
		if model.Contains(targets, d.Target) && !superseded[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
func resources(st model.State, targets []string) []model.Resource {
	ids := []string{}
	for id, r := range st.Resources {
		if r.Active && model.Contains(targets, r.Target) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	result := []model.Resource{}
	for _, id := range ids {
		result = append(result, st.Resources[id])
	}
	return result
}

func resourceIDs(st model.State, targets []string) []string {
	ids := []string{}
	for _, r := range resources(st, targets) {
		ids = append(ids, r.ID)
	}
	return ids
}
