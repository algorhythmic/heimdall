package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const ContinuityVersion = 1

var OpaqueID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Contract struct {
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	Target       string    `json:"target"`
	TaskRevision int64     `json:"task_revision"`
	Previous     string    `json:"previous"`
	Objective    string    `json:"objective"`
	Constraints  []string  `json:"constraints"`
	Acceptance   Done      `json:"acceptance"`
	ResourceIDs  []string  `json:"resource_ids,omitempty"`
	Actor        string    `json:"actor"`
	At           time.Time `json:"at"`
}

// Contract v2 freezes binding identities. An empty list is a reviewed empty scope;
// v1 has no reviewed scope, even when no bindings were present.
func ValidContract(c Contract) error {
	if c.Version != 1 && c.Version != 2 {
		return fmt.Errorf("unsupported contract version")
	}
	if c.Version == 1 && len(c.ResourceIDs) != 0 {
		return fmt.Errorf("v1 contract cannot declare scope")
	}
	for i, id := range c.ResourceIDs {
		if !OpaqueID.MatchString(id) || (i > 0 && c.ResourceIDs[i-1] >= id) {
			return fmt.Errorf("invalid contract scope")
		}
	}
	return ValidRecord(1, c.ID, c.Target, c.Actor, c.At)
}

type Decision struct {
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	Target       string    `json:"target"`
	TaskRevision int64     `json:"task_revision"`
	Text         string    `json:"text"`
	Supersedes   string    `json:"supersedes,omitempty"`
	Actor        string    `json:"actor"`
	At           time.Time `json:"at"`
}
type Resource struct {
	Version int       `json:"version"`
	ID      string    `json:"id"`
	Target  string    `json:"target"`
	Kind    string    `json:"kind"`
	Root    string    `json:"root"`
	Path    string    `json:"path"`
	Exclude []string  `json:"exclude"`
	Active  bool      `json:"active"`
	Initial Snapshot  `json:"initial"`
	Actor   string    `json:"actor"`
	At      time.Time `json:"at"`
}
type Snapshot struct {
	Digest string `json:"digest"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}
type ResourceVersion struct {
	ID       string   `json:"id"`
	Snapshot Snapshot `json:"snapshot"`
}
type ContextVersion struct {
	Target       string `json:"target"`
	TaskRevision int64  `json:"task_revision"`
	ContractID   string `json:"contract_id,omitempty"`
}
type Checkpoint struct {
	GrantID      string            `json:"grant_id,omitempty"`
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Target       string            `json:"target"`
	TaskRevision int64             `json:"task_revision"`
	ContractID   string            `json:"contract_id"`
	Previous     string            `json:"previous"`
	SourceEvent  int64             `json:"source_event"`
	Summary      string            `json:"summary"`
	CurrentStep  string            `json:"current_step,omitempty"`
	NextAction   string            `json:"next_action"`
	Blockers     []string          `json:"blockers"`
	Resources    []ResourceVersion `json:"resources"`
	Context      []ContextVersion  `json:"context"`
	Decisions    []string          `json:"decisions"`
	Actor        string            `json:"actor"`
	At           time.Time         `json:"at"`
}

func ValidCheckpoint(c Checkpoint) error {
	if c.Version == 1 {
		if c.GrantID != "" {
			return fmt.Errorf("legacy checkpoint cannot declare grant")
		}
		return ValidRecord(1, c.ID, c.Target, c.Actor, c.At)
	}
	if c.Version != 2 || !OpaqueID.MatchString(c.GrantID) || c.Actor != "client:"+c.GrantID {
		return fmt.Errorf("invalid client checkpoint provenance")
	}
	return ValidRecord(1, c.ID, c.Target, "cli", c.At)
}

// StrictJSON is shared by wire and versioned event payload readers.
func StrictJSON(b []byte, out any) error {
	check := json.NewDecoder(bytes.NewReader(b))
	if err := uniqueJSON(check); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

// encoding/json otherwise accepts duplicate keys and silently chooses the last.
func uniqueJSON(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid JSON key: %v", key)
			}
			seen[name] = true
			if err := uniqueJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(d); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}
func ResolveTarget(st State, target string) (TaskRecord, *Step, error) {
	parts := strings.Split(target, "#")
	if len(parts) > 2 || !ValidID(parts[0]) {
		return TaskRecord{}, nil, fmt.Errorf("invalid task target")
	}
	r, ok := st.Tasks[parts[0]]
	if !ok {
		return r, nil, fmt.Errorf("unknown task")
	}
	if len(parts) == 1 {
		return r, nil, nil
	}
	for i := range r.Task.Subtasks {
		if r.Task.Subtasks[i].ID == parts[1] {
			v := r.Task.Subtasks[i]
			return r, &v, nil
		}
	}
	return r, nil, fmt.Errorf("unknown step")
}
func ValidRecord(version int, id, target, actor string, at time.Time) error {
	if version != ContinuityVersion || !OpaqueID.MatchString(id) || actor != "cli" || at.IsZero() {
		return fmt.Errorf("invalid continuity record envelope")
	}
	p := strings.Split(target, "#")
	if len(p) > 2 || !ValidID(p[0]) || (len(p) == 2 && p[1] == "") {
		return fmt.Errorf("invalid continuity target")
	}
	return nil
}
