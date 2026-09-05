// Package model defines the versioned, provider-independent Heimdall core.
package model

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Check struct {
	ID            string `json:"id" yaml:"id"`
	Kind          string `json:"kind" yaml:"kind"`
	Days          int    `json:"days,omitempty" yaml:"days,omitempty"`
	After         string `json:"after,omitempty" yaml:"after,omitempty"`
	ResponseCheck string `json:"response_check,omitempty" yaml:"response_check,omitempty"`
	Account       string `json:"account,omitempty" yaml:"account,omitempty"`
	FromDomain    string `json:"from_domain,omitempty" yaml:"from_domain,omitempty"`
	ToDomain      string `json:"to_domain,omitempty" yaml:"to_domain,omitempty"`
	From          string `json:"from,omitempty" yaml:"from,omitempty"`
	To            string `json:"to,omitempty" yaml:"to,omitempty"`
	Correlation   string `json:"correlation,omitempty" yaml:"correlation,omitempty"`
	Since         string `json:"since,omitempty" yaml:"since,omitempty"`
	Repo          string `json:"repo,omitempty" yaml:"repo,omitempty"`
	Ref           string `json:"ref,omitempty" yaml:"ref,omitempty"`
	Session       string `json:"session,omitempty" yaml:"session,omitempty"`
	URL           string `json:"url,omitempty" yaml:"url,omitempty"`
}
type Done struct {
	Text   string  `json:"text" yaml:"text"`
	Mode   string  `json:"mode,omitempty" yaml:"mode,omitempty"`
	Checks []Check `json:"checks,omitempty" yaml:"checks,omitempty"`
}
type Step struct {
	ID       string   `json:"id" yaml:"id"`
	Title    string   `json:"title" yaml:"title"`
	Status   string   `json:"status" yaml:"status"`
	After    []string `json:"after,omitempty" yaml:"after,omitempty"`
	Due      string   `json:"due,omitempty" yaml:"due,omitempty"`
	Estimate *int     `json:"estimate_minutes,omitempty" yaml:"estimate_minutes,omitempty"`
	Done     Done     `json:"done" yaml:"done"`
}
type Task struct {
	ID         string         `json:"id" yaml:"id"`
	Parent     string         `json:"parent,omitempty" yaml:"parent,omitempty"`
	Title      string         `json:"title" yaml:"title"`
	Type       string         `json:"type" yaml:"type"`
	Status     string         `json:"status" yaml:"status"`
	Importance *int           `json:"importance,omitempty" yaml:"importance,omitempty"`
	ResumeBy   string         `json:"resume_by,omitempty" yaml:"resume_by,omitempty"`
	Estimate   *int           `json:"estimate_minutes,omitempty" yaml:"estimate_minutes,omitempty"`
	NextAction string         `json:"next_action,omitempty" yaml:"next_action,omitempty"`
	Impact     map[string]int `json:"impact,omitempty" yaml:"impact,omitempty"`
	Done       Done           `json:"done" yaml:"done"`
	Subtasks   []Step         `json:"subtasks,omitempty" yaml:"subtasks,omitempty"`
}
type Document struct {
	Version  int    `json:"version" yaml:"version"`
	Revision int64  `json:"revision" yaml:"revision"`
	Tasks    []Task `json:"tasks" yaml:"tasks"`
}
type Mapping struct {
	AfterSubtask string `json:"after_subtask" yaml:"after_subtask"`
	Status       string `json:"status" yaml:"status"`
}
type Workflow struct {
	Version  int       `json:"version" yaml:"version"`
	Statuses []string  `json:"statuses" yaml:"statuses"`
	Initial  string    `json:"initial_status" yaml:"initial_status"`
	Success  []string  `json:"success_statuses" yaml:"success_statuses"`
	Dropped  []string  `json:"dropped_statuses" yaml:"dropped_statuses"`
	Subtasks []Step    `json:"subtasks,omitempty" yaml:"subtasks,omitempty"`
	Mappings []Mapping `json:"proposed_status_mappings,omitempty" yaml:"proposed_status_mappings,omitempty"`
}
type Catalog struct {
	Version  int                 `json:"version" yaml:"version"`
	Revision int64               `json:"revision" yaml:"revision"`
	Types    map[string]Workflow `json:"types" yaml:"types"`
}
type Stamp struct {
	At    time.Time `json:"at"`
	Token string    `json:"token"`
}
type TaskRecord struct {
	Task      Task             `json:"task"`
	Revision  int64            `json:"revision"`
	Workflow  Workflow         `json:"workflow"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Completed map[string]Stamp `json:"completed"`
}
type Capture struct {
	ID        string     `json:"id"`
	Client    string     `json:"client"`
	Pointer   string     `json:"pointer"`
	Title     string     `json:"title"`
	Targets   []string   `json:"targets"`
	Kind      string     `json:"kind"`
	Why       string     `json:"why"`
	Origin    string     `json:"origin,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Expired   bool       `json:"expired"`
}
type Proposal struct {
	ID             string    `json:"id"`
	Target         string    `json:"target"`
	TargetRevision int64     `json:"target_revision"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	Evidence       []string  `json:"evidence"`
	CreatedAt      time.Time `json:"created_at"`
}
type Timer struct {
	ID      string    `json:"id"`
	Target  string    `json:"target"`
	Kind    string    `json:"kind"`
	Anchor  string    `json:"anchor,omitempty"`
	DueAt   time.Time `json:"due_at"`
	Status  string    `json:"status"`
	Outcome string    `json:"outcome,omitempty"`
}
type State struct {
	Grants            map[string]Grant            `json:"grants"`
	Revision          int64                       `json:"revision"`
	LastEventID       int64                       `json:"last_event_id"`
	Tasks             map[string]TaskRecord       `json:"tasks"`
	Captures          map[string]Capture          `json:"captures"`
	Proposals         map[string]Proposal         `json:"proposals"`
	Timers            map[string]Timer            `json:"timers"`
	Browsers          map[string]BrowserProfile   `json:"browsers"`
	BrowserOperations map[string]BrowserOperation `json:"browser_operations"`
	Contracts         map[string]Contract         `json:"contracts"`
	ContractHeads     map[string]string           `json:"contract_heads"`
	Decisions         map[string]Decision         `json:"decisions"`
	Resources         map[string]Resource         `json:"resources"`
	Checkpoints       map[string]Checkpoint       `json:"checkpoints"`
	CheckpointHeads   map[string]string           `json:"checkpoint_heads"`
}

func Empty() State {
	s := State{}
	s.Normalize()
	return s
}
func (s State) Document() Document {
	d := Document{Version: 1, Revision: s.Revision, Tasks: []Task{}}
	for _, r := range s.Tasks {
		d.Tasks = append(d.Tasks, r.Task)
	}
	sort.Slice(d.Tasks, func(i, j int) bool { return d.Tasks[i].ID < d.Tasks[j].ID })
	return d
}
func Clone[T any](x T) T {
	b, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	var y T
	if err = json.Unmarshal(b, &y); err != nil {
		panic(err)
	}
	return y
}
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func Int(n int) *int { return &n }
func Contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

var taskID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,22}[a-z0-9]$`)
var localID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
var statusID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func ValidID(s string) bool { return taskID.MatchString(s) && s != "unassigned" && s != "radiator" }
func StrictYAML(b []byte, out any) error {
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one YAML document")
	}
	return nil
}
func ParseDocument(b []byte) (Document, error) {
	var d Document
	err := StrictYAML(b, &d)
	return d, err
}
func ParseCatalog(b []byte) (Catalog, error) {
	var c Catalog
	if err := StrictYAML(b, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}
func Date(s string) error {
	if s == "" {
		return nil
	}
	_, err := time.Parse("2006-01-02", s)
	return err
}
func Defaults(t *Task, w Workflow) {
	if t.Importance == nil {
		t.Importance = Int(3)
	}
	if t.Status == "" {
		t.Status = w.Initial
	}
	for i := range t.Subtasks {
		if t.Subtasks[i].Status == "" {
			t.Subtasks[i].Status = "open"
		}
	}
}
func (c Catalog) Validate() error {
	if c.Version != 1 || c.Revision < 0 || len(c.Types) == 0 {
		return fmt.Errorf("invalid workflow catalog version/revision/types")
	}
	for name, w := range c.Types {
		if !localID.MatchString(name) || w.Version < 1 || len(w.Statuses) == 0 || len(w.Success) == 0 || len(w.Dropped) == 0 {
			return fmt.Errorf("invalid workflow %s", name)
		}
		seen := map[string]bool{}
		for _, s := range w.Statuses {
			if !statusID.MatchString(s) || seen[s] {
				return fmt.Errorf("duplicate/invalid workflow status")
			}
			seen[s] = true
		}
		if !seen[w.Initial] {
			return fmt.Errorf("unknown initial status")
		}
		for _, s := range append(append([]string{}, w.Success...), w.Dropped...) {
			if !seen[s] {
				return fmt.Errorf("unknown terminal status")
			}
		}
		for _, s := range w.Success {
			if Contains(w.Dropped, s) {
				return fmt.Errorf("success and dropped overlap")
			}
		}
		t := Task{ID: "template-task", Title: name, Type: name, Status: w.Initial, Subtasks: Clone(w.Subtasks)}
		Defaults(&t, w)
		if err := validateSteps(t); err != nil {
			return fmt.Errorf("template %s: %w", name, err)
		}
		for _, m := range w.Mappings {
			found := false
			for _, s := range w.Subtasks {
				if s.ID == m.AfterSubtask {
					found = true
				}
			}
			if !found || !seen[m.Status] {
				return fmt.Errorf("invalid status mapping in %s", name)
			}
		}
	}
	return nil
}
func Validate(d Document, c Catalog) error {
	if d.Version != 1 || d.Revision < 0 {
		return fmt.Errorf("invalid document version/revision")
	}
	byID := map[string]Task{}
	children := map[string]int{}
	for _, t := range d.Tasks {
		if !ValidID(t.ID) || strings.TrimSpace(t.Title) == "" {
			return fmt.Errorf("invalid task id/title: %q", t.ID)
		}
		if _, ok := byID[t.ID]; ok {
			return fmt.Errorf("duplicate task %s", t.ID)
		}
		byID[t.ID] = t
		children[t.Parent]++
		w, ok := c.Types[t.Type]
		if !ok || !Contains(w.Statuses, t.Status) {
			return fmt.Errorf("invalid type/status for %s", t.ID)
		}
		if t.Importance == nil || *t.Importance < 1 || *t.Importance > 5 {
			return fmt.Errorf("importance must be 1..5")
		}
		if err := Date(t.ResumeBy); err != nil {
			return fmt.Errorf("%s resume_by: %w", t.ID, err)
		}
		if t.Estimate != nil && *t.Estimate <= 0 {
			return fmt.Errorf("estimate must be positive")
		}
		for k, v := range t.Impact {
			if !Contains([]string{"income", "skill", "portfolio", "relationships"}, k) || v < 0 || v > 5 {
				return fmt.Errorf("invalid impact")
			}
		}
		if err := validateSteps(t); err != nil {
			return fmt.Errorf("%s: %w", t.ID, err)
		}
		if err := validateDone(t.Done, t); err != nil {
			return fmt.Errorf("%s done: %w", t.ID, err)
		}
	}
	for id, t := range byID {
		if children[id] > 0 && len(t.Subtasks) > 0 {
			return fmt.Errorf("%s has both children and subtasks", id)
		}
		seen := map[string]bool{id: true}
		for t.Parent != "" {
			p, ok := byID[t.Parent]
			if !ok || seen[t.Parent] {
				return fmt.Errorf("invalid parent or cycle at %s", id)
			}
			seen[t.Parent] = true
			t = p
		}
	}
	return nil
}
func validateSteps(t Task) error {
	byID := map[string]Step{}
	for _, s := range t.Subtasks {
		if !localID.MatchString(s.ID) || strings.TrimSpace(s.Title) == "" || !Contains([]string{"open", "blocked", "done", "dropped"}, s.Status) {
			return fmt.Errorf("invalid subtask %s", s.ID)
		}
		if _, ok := byID[s.ID]; ok {
			return fmt.Errorf("duplicate subtask")
		}
		byID[s.ID] = s
		if s.Estimate != nil && *s.Estimate <= 0 {
			return fmt.Errorf("estimate must be positive")
		}
		if err := Date(s.Due); err != nil {
			return err
		}
		if err := validateDone(s.Done, t); err != nil {
			return err
		}
	}
	visiting := map[string]bool{}
	done := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if done[id] {
			return nil
		}
		s, ok := byID[id]
		if !ok || visiting[id] {
			return fmt.Errorf("invalid prerequisite/cycle %s", id)
		}
		visiting[id] = true
		seen := map[string]bool{}
		for _, dep := range s.After {
			if seen[dep] {
				return fmt.Errorf("duplicate prerequisite")
			}
			seen[dep] = true
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[id] = false
		done[id] = true
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
func validateDone(d Done, t Task) error {
	if d.Checks != nil && len(d.Checks) == 0 {
		return fmt.Errorf("empty explicit automatic check list; omit checks for manual completion")
	}
	if d.Mode != "" && d.Mode != "any" && d.Mode != "all" {
		return fmt.Errorf("done.mode must be any/all")
	}
	if len(d.Checks) > 1 && d.Mode == "" {
		return fmt.Errorf("multiple checks require explicit mode")
	}
	seen := map[string]Check{}
	for _, c := range d.Checks {
		if !localID.MatchString(c.ID) {
			return fmt.Errorf("invalid check id")
		}
		if _, ok := seen[c.ID]; ok {
			return fmt.Errorf("duplicate check id")
		}
		seen[c.ID] = c
		allowed := map[string]string{"manual": "", "children_done": "", "subtasks_done": "", "silence": "days after response_check", "mail.sent": "account to to_domain since after correlation", "mail.received": "account from from_domain since after correlation", "agent.released": "session", "repo.commit": "repo ref", "gh.pr_merged": "url"}
		fields, ok := allowed[c.Kind]
		if !ok {
			return fmt.Errorf("unknown check kind %q", c.Kind)
		}
		b, _ := json.Marshal(c)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		for k := range m {
			if k != "id" && k != "kind" && !Contains(strings.Fields(fields), k) {
				return fmt.Errorf("parameter %s not valid for %s", k, c.Kind)
			}
		}
		if c.After != "" {
			parts := strings.Split(c.After, "#")
			if len(parts) != 2 || parts[0] != t.ID {
				return fmt.Errorf("check anchor must refer to a subtask in this task")
			}
			found := false
			for _, s := range t.Subtasks {
				if s.ID == parts[1] {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unknown check anchor")
			}
		}
		if c.Kind == "silence" && (c.Days < 1 || c.Days > 36500 || c.After == "" || c.ResponseCheck == "") {
			return fmt.Errorf("silence requires days, anchor and response_check")
		}
		if strings.HasPrefix(c.Kind, "mail.") {
			if c.Account == "" {
				return fmt.Errorf("mail check requires account")
			}
			if c.Since != "" && c.Since != "step_opened" {
				if _, err := time.Parse(time.RFC3339, c.Since); err != nil {
					return fmt.Errorf("invalid check since")
				}
			}
			if c.Correlation != "" && c.Correlation != "reply_to_anchor" {
				return fmt.Errorf("unknown mail correlation")
			}
			if c.Correlation != "" && c.After == "" {
				return fmt.Errorf("reply correlation requires anchor")
			}
			if c.Kind == "mail.sent" && c.To == "" && c.ToDomain == "" {
				return fmt.Errorf("mail.sent needs recipient")
			}
			if c.Kind == "mail.received" && c.From == "" && c.FromDomain == "" {
				return fmt.Errorf("mail.received needs sender")
			}
		}
		if c.Kind == "repo.commit" && c.Repo == "" {
			return fmt.Errorf("repo.commit requires repo")
		}
		if c.Kind == "gh.pr_merged" && c.URL == "" {
			return fmt.Errorf("gh.pr_merged requires url")
		}
	}
	for _, c := range d.Checks {
		if c.Kind == "silence" {
			r, ok := seen[c.ResponseCheck]
			if !ok || r.Kind != "mail.received" || r.After != c.After {
				return fmt.Errorf("silence response must be an anchored mail.received check")
			}
		}
	}
	return nil
}
func ChangedFields(a, b Task) []string {
	out := []string{}
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	typ := av.Type()
	for i := 0; i < av.NumField(); i++ {
		if !reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			out = append(out, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
		}
	}
	return out
}
