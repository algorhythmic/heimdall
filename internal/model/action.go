package model

import (
	"fmt"
	"net/url"
	"slices"
	"time"
)

func ValidActionExecution(s string) bool {
	return Contains([]string{"queued", "dispatching", "api_reported", "refused", "uncertain", "cancelled"}, s)
}
func ValidActionVerification(s string) bool {
	return Contains([]string{"pending", "matched", "not_matched", "unknown", "unsupported"}, s)
}

type BrowserIntent struct {
	Profile     string `json:"profile"`
	Epoch       string `json:"epoch"`
	Action      string `json:"action"`
	TabID       int    `json:"tab_id,omitempty"`
	WindowID    int    `json:"window_id,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	ExpectedURL string `json:"expected_url,omitempty"`
	URL         string `json:"url,omitempty"`
}
type ActionPostcondition struct {
	Kind           string `json:"kind"`
	URL            string `json:"url,omitempty"`
	RedirectPolicy string `json:"redirect_policy"`
	LoadCondition  string `json:"load_condition"`
}
type ActionIntent struct {
	Version       int                 `json:"version"`
	ID            string              `json:"id"`
	Target        string              `json:"target"`
	TaskRevision  int64               `json:"task_revision"`
	ManifestID    string              `json:"manifest_id"`
	SurfaceID     string              `json:"surface_id"`
	ContextDigest string              `json:"context_digest"`
	SnapshotID    string              `json:"snapshot_id,omitempty"`
	Adapter       string              `json:"adapter"`
	AttemptID     string              `json:"attempt_id"`
	Authority     string              `json:"authority"`
	AuthorityRef  string              `json:"authority_ref"`
	Browser       *BrowserIntent      `json:"browser,omitempty"`
	Expected      ActionPostcondition `json:"expected"`
	At            time.Time           `json:"at"`
	ExpiresAt     time.Time           `json:"expires_at"`
}
type ActionReport struct {
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	TabID    int    `json:"tab_id,omitempty"`
	WindowID int    `json:"window_id,omitempty"`
	URL      string `json:"url,omitempty"`
}
type ActionObservation struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	SourceEpoch string    `json:"source_epoch"`
	Digest      string    `json:"digest"`
	ObservedAt  time.Time `json:"observed_at"`
	Detail      string    `json:"detail"`
}
type ActionRecord struct {
	Intent              ActionIntent       `json:"intent"`
	IntentDigest        string             `json:"intent_digest"`
	Revision            int64              `json:"revision"`
	Execution           string             `json:"execution"`
	Verification        string             `json:"verification"`
	CancelRequested     bool               `json:"cancel_requested"`
	DeliveryID          string             `json:"delivery_id,omitempty"`
	UncertainSince      time.Time          `json:"uncertain_since"`
	Report              *ActionReport      `json:"report,omitempty"`
	Observation         *ActionObservation `json:"observation,omitempty"`
	DispatchObservation *ActionObservation `json:"dispatch_observation,omitempty"`
	UpdatedAt           time.Time          `json:"updated_at"`
	LastReason          string             `json:"last_reason"`
	LastEventID         int64              `json:"last_event_id"`
}
type ActionTransition struct {
	Version          int                `json:"version"`
	ID               string             `json:"id"`
	ActionID         string             `json:"action_id"`
	AttemptID        string             `json:"attempt_id"`
	PreviousRevision int64              `json:"previous_revision"`
	Kind             string             `json:"kind"`
	Reason           string             `json:"reason"`
	DeliveryID       string             `json:"delivery_id,omitempty"`
	Report           *ActionReport      `json:"report,omitempty"`
	Observation      *ActionObservation `json:"observation,omitempty"`
	Actor            string             `json:"actor"`
	At               time.Time          `json:"at"`
}
type BrowserActionRef struct {
	Version      int    `json:"version"`
	ID           string `json:"id"`
	AttemptID    string `json:"attempt_id"`
	IntentDigest string `json:"intent_digest"`
	Target       string `json:"target"`
	ManifestID   string `json:"manifest_id"`
	SurfaceID    string `json:"surface_id"`
}

func (a ActionRecord) BrowserRef() *BrowserActionRef {
	return &BrowserActionRef{1, a.Intent.ID, a.Intent.AttemptID, a.IntentDigest, a.Intent.Target, a.Intent.ManifestID, a.Intent.SurfaceID}
}
func (r BrowserActionRef) Validate() error {
	if r.Version != 1 || !ValidID(r.Target) || !TokenHashPattern.MatchString(r.IntentDigest) {
		return fmt.Errorf("invalid action reference")
	}
	for _, id := range []string{r.ID, r.AttemptID, r.ManifestID, r.SurfaceID} {
		if !OpaqueID.MatchString(id) {
			return fmt.Errorf("invalid action reference identity")
		}
	}
	return nil
}
func BrowserURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && len(s) <= 8192
}
func (b BrowserIntent) Validate() error {
	if !OpaqueID.MatchString(b.Profile) || !OpaqueID.MatchString(b.Epoch) || !Contains([]string{"open", "navigate", "focus", "move", "close"}, b.Action) {
		return fmt.Errorf("invalid browser intent identity/action")
	}
	if b.Action == "open" {
		if !BrowserURL(b.URL) || b.TabID != 0 || b.WindowID != 0 || b.OwnerID != "" || b.ExpectedURL != "" {
			return fmt.Errorf("open requires only a URL")
		}
	} else {
		if b.TabID < 1 || !OpaqueID.MatchString(b.OwnerID) || !BrowserURL(b.ExpectedURL) {
			return fmt.Errorf("owned tab and expected URL required")
		}
		if (b.Action == "navigate") != (b.URL != "") || (b.URL != "" && !BrowserURL(b.URL)) {
			return fmt.Errorf("only navigation accepts a destination URL")
		}
		if (b.Action == "move" && b.WindowID < 1) || (b.Action != "move" && b.WindowID != 0) {
			return fmt.Errorf("only move requires a destination window")
		}
	}
	return nil
}
func BrowserPostcondition(b BrowserIntent) ActionPostcondition {
	p := ActionPostcondition{RedirectPolicy: "exact", LoadCondition: "not_requested"}
	switch b.Action {
	case "open":
		p.Kind = "owned_instance_exists"
		p.URL = b.URL
	case "navigate":
		p.Kind = "url_matches"
		p.URL = b.URL
	case "focus":
		p.Kind = "active_in_focused_window"
	case "move":
		p.Kind = "window_membership"
	case "close":
		p.Kind = "owned_instance_absent"
	}
	return p
}
func (v ActionIntent) Validate() error {
	if v.Version != 1 || !ValidID(v.Target) || v.TaskRevision < 1 || !TokenHashPattern.MatchString(v.ContextDigest) || v.Adapter != "browser" || v.Authority != "cli" || v.AuthorityRef != "action-"+v.ID || v.At.IsZero() || v.ExpiresAt.Sub(v.At) != 30*time.Second {
		return fmt.Errorf("invalid action intent envelope")
	}
	for _, id := range []string{v.ID, v.ManifestID, v.SurfaceID, v.AttemptID} {
		if !OpaqueID.MatchString(id) {
			return fmt.Errorf("invalid action identity")
		}
	}
	if v.SnapshotID != "" && !OpaqueID.MatchString(v.SnapshotID) {
		return fmt.Errorf("invalid action snapshot reference")
	}
	if v.Browser == nil {
		return fmt.Errorf("browser intent required")
	}
	if err := v.Browser.Validate(); err != nil {
		return err
	}
	if v.Expected != BrowserPostcondition(*v.Browser) {
		return fmt.Errorf("postcondition must match typed intent")
	}
	return nil
}

// The context pin contains accepted record identity, never resource contents or
// permission to execute a checkpoint/recipe. Browser actions gain no file access.
func ActionContextDigest(st State, target string) string {
	type pin struct {
		Target   string
		Revision int64
		Contract string
	}
	pins := []pin{}
	lineage, err := TargetLineage(st, target)
	if err != nil {
		return ""
	}
	for _, target := range lineage {
		task, _, err := ResolveTarget(st, target)
		if err != nil {
			return ""
		}
		pins = append(pins, pin{target, task.Revision, st.ContractHeads[target]})
	}
	ids, err := ResourceScope(st, target)
	if err != nil {
		return ""
	}
	slices.Sort(ids)
	resources := []Resource{}
	for _, id := range ids {
		resources = append(resources, st.Resources[id])
	}
	return ContentDigest(struct {
		Pins      []pin
		Resources []Resource
		Decisions string
	}{pins, resources, EvidenceDecisionDigest(st, target)})
}

func ActionHolds(a ActionRecord) bool {
	if a.Execution == "cancelled" || a.Execution == "refused" {
		return false
	}
	return a.Verification != "matched" && a.Verification != "not_matched"
}
func ActionConflict(st State, v ActionIntent) string {
	ids := []string{}
	for id, a := range st.Actions {
		if !ActionHolds(a) {
			continue
		}
		if a.Intent.SurfaceID == v.SurfaceID || (a.Intent.Browser != nil && v.Browser != nil && a.Intent.Browser.Profile == v.Browser.Profile && a.Intent.Browser.Epoch == v.Browser.Epoch && v.Browser.OwnerID != "" && (a.Intent.ID == v.Browser.OwnerID || a.Intent.Browser.OwnerID == v.Browser.OwnerID)) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}
func ActionInputsCurrent(st State, v ActionIntent) bool {
	return st.Tasks[v.Target].Revision == v.TaskRevision && st.WorkspaceHeads[v.Target] == v.ManifestID && ActionContextDigest(st, v.Target) == v.ContextDigest
}

func ActionBrowserCurrent(st State, v ActionIntent) bool {
	if v.Browser == nil {
		return false
	}
	b := v.Browser
	p := st.Browsers[b.Profile]
	if !p.Paired || p.Epoch != b.Epoch || p.ActionProtocol != 1 {
		return false
	}
	if b.Action == "open" {
		return true
	}
	for _, tab := range p.Tabs {
		if tab.ID == b.TabID && tab.OwnerID == b.OwnerID && tab.URL == b.ExpectedURL {
			return true
		}
	}
	return false
}
