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
	Pairing       *BrowserPairingIntent `json:"pairing,omitempty"`
	LoadCondition string                `json:"load_condition,omitempty"`
	Profile       string                `json:"profile"`
	Epoch         string                `json:"epoch"`
	Action        string                `json:"action"`
	TabID         int                   `json:"tab_id,omitempty"`
	WindowID      int                   `json:"window_id,omitempty"`
	OwnerID       string                `json:"owner_id,omitempty"`
	ExpectedURL   string                `json:"expected_url,omitempty"`
	URL           string                `json:"url,omitempty"`
}
type ActionPostcondition struct {
	Kind           string `json:"kind"`
	URL            string `json:"url,omitempty"`
	RedirectPolicy string `json:"redirect_policy"`
	LoadCondition  string `json:"load_condition"`
}
type ActionIntent struct {
	Workspace     *BrowserWorkspaceAction `json:"workspace,omitempty"`
	Version       int                     `json:"version"`
	ID            string                  `json:"id"`
	Target        string                  `json:"target"`
	TaskRevision  int64                   `json:"task_revision"`
	ManifestID    string                  `json:"manifest_id"`
	SurfaceID     string                  `json:"surface_id"`
	ContextDigest string                  `json:"context_digest"`
	SnapshotID    string                  `json:"snapshot_id,omitempty"`
	Adapter       string                  `json:"adapter"`
	AttemptID     string                  `json:"attempt_id"`
	Authority     string                  `json:"authority"`
	AuthorityRef  string                  `json:"authority_ref"`
	Browser       *BrowserIntent          `json:"browser,omitempty"`
	Native        *NativeIntent           `json:"native,omitempty"`
	Expected      ActionPostcondition     `json:"expected"`
	At            time.Time               `json:"at"`
	ExpiresAt     time.Time               `json:"expires_at"`
}
type ActionReport struct {
	Native   *NativeDispatchReport `json:"native,omitempty"`
	Status   string                `json:"status"`
	Detail   string                `json:"detail"`
	TabID    int                   `json:"tab_id,omitempty"`
	WindowID int                   `json:"window_id,omitempty"`
	URL      string                `json:"url,omitempty"`
}
type ActionObservation struct {
	Native      *NativeReadback `json:"native,omitempty"`
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	SourceEpoch string          `json:"source_epoch"`
	Digest      string          `json:"digest"`
	ObservedAt  time.Time       `json:"observed_at"`
	Detail      string          `json:"detail"`
}
type ActionRecord struct {
	Pairing              *BrowserPairingState `json:"pairing,omitempty"`
	VerificationAttempts int                  `json:"verification_attempts,omitempty"`
	Intent               ActionIntent         `json:"intent"`
	IntentDigest         string               `json:"intent_digest"`
	Revision             int64                `json:"revision"`
	Execution            string               `json:"execution"`
	Verification         string               `json:"verification"`
	CancelRequested      bool                 `json:"cancel_requested"`
	DeliveryID           string               `json:"delivery_id,omitempty"`
	UncertainSince       time.Time            `json:"uncertain_since"`
	Report               *ActionReport        `json:"report,omitempty"`
	Observation          *ActionObservation   `json:"observation,omitempty"`
	DispatchObservation  *ActionObservation   `json:"dispatch_observation,omitempty"`
	UpdatedAt            time.Time            `json:"updated_at"`
	LastReason           string               `json:"last_reason"`
	LastEventID          int64                `json:"last_event_id"`
}
type ActionTransition struct {
	PairProbe        *BrowserPairProbe  `json:"pair_probe,omitempty"`
	PairReady        *BrowserPairReady  `json:"pair_ready,omitempty"`
	AssociationID    string             `json:"association_id,omitempty"`
	ContinuationID   string             `json:"continuation_id,omitempty"`
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
	if a.Intent.Native != nil {
		return nil
	}
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
	if b.Pairing != nil {
		if err := b.Pairing.Validate(); err != nil {
			return err
		}
		if b.Action != "open" && b.Action != "associate" {
			return fmt.Errorf("pairing is supported only for open or explicit association")
		}
	}
	if b.Action == "associate" && b.Pairing == nil {
		return fmt.Errorf("association requires explicit pairing authority")
	}
	if b.LoadCondition != "" && (b.LoadCondition != "complete" || (b.Action != "open" && b.Action != "navigate")) {
		return fmt.Errorf("only open/navigation accept complete load condition")
	}
	if !OpaqueID.MatchString(b.Profile) || !OpaqueID.MatchString(b.Epoch) || !Contains([]string{"open", "navigate", "focus", "move", "close", "associate"}, b.Action) {
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
		if ((b.Action == "move" || b.Action == "associate") && b.WindowID < 1) || (b.Action != "move" && b.Action != "associate" && b.WindowID != 0) {
			return fmt.Errorf("only move requires a destination window")
		}
	}
	return nil
}
func BrowserPostcondition(b BrowserIntent) ActionPostcondition {
	p := ActionPostcondition{RedirectPolicy: "exact", LoadCondition: "not_requested"}
	if b.LoadCondition != "" {
		p.LoadCondition = b.LoadCondition
	}
	switch b.Action {
	case "associate":
		p.Kind = "native_window_association"
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
	if (v.Version < 1 || v.Version > 5) || !ValidID(v.Target) || v.TaskRevision < 1 || !TokenHashPattern.MatchString(v.ContextDigest) || v.Authority != "cli" || v.At.IsZero() || v.ExpiresAt.Sub(v.At) != 30*time.Second {
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
	if (v.Version == 5) != (v.Workspace != nil) {
		return fmt.Errorf("workspace browser action requires version 5")
	}
	if v.Workspace != nil {
		w := v.Workspace
		if v.Native != nil || v.Browser == nil || v.Adapter != "browser" || !OpaqueID.MatchString(w.OperationID) || !OpaqueID.MatchString(w.RecipeID) || (w.PreviousViewport != "" && !OpaqueID.MatchString(w.PreviousViewport)) || v.AuthorityRef != "workspace-operation-"+w.OperationID || !Contains([]string{"open", "focus", "close"}, v.Browser.Action) || v.Browser.Validate() != nil || v.Expected != BrowserPostcondition(*v.Browser) || (v.Browser.Action == "open" && v.Browser.Pairing == nil) {
			return fmt.Errorf("invalid workspace browser authority")
		}
		return nil
	}
	if v.Version == 3 || v.Version == 4 {
		if v.Version == 3 && v.Native != nil && (v.Native.Launch != nil || v.Native.RecipeID != "") {
			return fmt.Errorf("application inputs require action version 4")
		}
		if v.Version == 4 && (v.Native == nil || (v.Native.Launch == nil && v.Native.RecipeID == "")) {
			return fmt.Errorf("application action lacks recipe")
		}
		if v.Native == nil || v.Browser != nil || v.Adapter != "hyprland" || v.AuthorityRef != "workspace-operation-"+v.Native.OperationID || v.Native.Validate() != nil || v.Expected != NativePostcondition(*v.Native) {
			return fmt.Errorf("invalid workspace-owned native action")
		}
		return nil
	}
	if v.Native != nil || v.Adapter != "browser" || v.AuthorityRef != "action-"+v.ID {
		return fmt.Errorf("invalid browser action authority")
	}
	if v.Browser == nil {
		return fmt.Errorf("browser intent required")
	}
	if (v.Version == 2) != (v.Browser.Pairing != nil) {
		return fmt.Errorf("pairing requires intent version 2")
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
	if a.Execution == "cancelled" || a.Execution == "refused" || (a.Pairing != nil && a.Pairing.Abandoned) {
		return false
	}
	// Native applications can process a graceful request after the IPC ACK.
	// Give the bounded observation budget time to settle without repeating input.
	if a.Intent.Native != nil && a.Execution == "api_reported" && a.Verification == "not_matched" && a.VerificationAttempts < 8 {
		return true
	}
	// An uncertain failed postcondition is not permission to duplicate a possible effect.
	return a.Verification != "matched" && !(a.Verification == "not_matched" && a.Execution == "api_reported")
}
func ActionConflict(st State, v ActionIntent) string {
	ids := []string{}
	for id, a := range st.Actions {
		if !ActionHolds(a) {
			continue
		}
		if a.Intent.Browser != nil && v.Browser != nil && ((a.Intent.Browser.Pairing != nil && v.Browser.Pairing != nil) || ((a.Intent.Browser.Pairing != nil || v.Browser.Pairing != nil) && a.Intent.Browser.Profile == v.Browser.Profile)) {
			ids = append(ids, id)
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
	if v.Workspace != nil {
		w := v.Workspace
		op := st.WorkspaceOperations[w.OperationID]
		if st.DesktopSourceHead != op.Intent.SourceID || !st.DesktopSources[op.Intent.SourceID].Active || st.DesktopSources[op.Intent.SourceID].Epoch != op.Intent.SourceEpoch {
			return false
		}
		inputs := op.Intent.InputDigest
		if op.Intent.Swap != nil && v.Target == op.Intent.Swap.Target {
			inputs = op.Intent.Swap.InputDigest
		}
		if !WorkspaceOperationHolds(op) || op.CancelRequested || !WorkspaceActionScope(op, v) || !Contains(op.ActionIDs, v.ID) || !ApplicationRecipeCurrent(st, w.RecipeID, v.Target, v.SurfaceID) || OperationInputDigest(st, op, v.Target) != inputs {
			return false
		}
	}
	if v.Native != nil {
		n := v.Native
		source := st.DesktopSources[n.SourceID]
		operation := st.WorkspaceOperations[n.OperationID]
		inputs := operation.Intent.InputDigest
		if operation.Intent.Swap != nil && v.Target == operation.Intent.Swap.Target {
			inputs = operation.Intent.Swap.InputDigest
		}
		if OperationInputDigest(st, operation, v.Target) != inputs {
			return false
		}
		if !WorkspaceOperationHolds(operation) || operation.CancelRequested || !WorkspaceActionScope(operation, v) || !source.Active || st.DesktopSourceHead != source.ID || source.Epoch != n.SourceEpoch || !nativeViewportCurrent(st, v) {
			return false
		}
		if n.Window != nil {
			b := st.ViewportBindings[n.ViewportBindingID]
			if !b.Active || b.Window == nil || *b.Window != *n.Window || b.Target != v.Target || b.SurfaceID != v.SurfaceID || b.SourceID != n.SourceID || b.ManifestID != v.ManifestID || b.TaskRevision != v.TaskRevision {
				return false
			}
		}
	}
	if v.Browser != nil && v.Browser.Pairing != nil {
		p := v.Browser.Pairing
		s := st.DesktopSources[p.SourceID]
		if !s.Active || st.DesktopSourceHead != p.SourceID || s.Epoch != p.SourceEpoch {
			return false
		}
		previous := p.PreviousViewport
		if previous == "none" {
			previous = ""
		}
		if st.ViewportHeads[v.SurfaceID] != previous {
			b := st.ViewportBindings[st.ViewportHeads[v.SurfaceID]]
			proof := st.BrowserAssociations[b.BrowserAssociationID]
			if proof.ActionRef.ID != v.ID || proof.ActionRef.IntentDigest != ContentDigest(v) {
				return false
			}
		}
	}
	return st.Tasks[v.Target].Revision == v.TaskRevision && st.WorkspaceHeads[v.Target] == v.ManifestID && ActionContextDigest(st, v.Target) == v.ContextDigest
}

func ActionBrowserCurrent(st State, v ActionIntent) bool {
	if v.Workspace != nil {
		op := st.WorkspaceOperations[v.Workspace.OperationID]
		if v.Target == op.Intent.Target && !WorkspaceSwapReady(st, op) {
			return false
		}
	}
	if v.Browser == nil {
		return false
	}
	b := v.Browser
	p := st.Browsers[b.Profile]
	if v.Workspace != nil && p.RecoveryProtocol != 1 {
		return false
	}
	if !p.Paired || p.Epoch != b.Epoch || p.ActionProtocol != 1 {
		return false
	}
	if b.Pairing != nil && (p.PairingProtocol != 1 || p.VerificationProtocol != 1 || !BrowserExtensionIDPattern.MatchString(p.ExtensionID)) {
		return false
	}

	if b.Action == "open" {
		if v.Workspace != nil {
			for _, tab := range p.Tabs {
				if tab.URL == b.URL && tab.OwnerID != v.ID {
					return false
				}
			}
		}
		for id, old := range st.Actions {
			if id == v.ID || old.Intent.SurfaceID != v.SurfaceID || old.Intent.Browser == nil || old.Intent.Browser.Action != "open" || old.Execution == "refused" || old.Execution == "cancelled" {
				continue
			}
			// A new action ID cannot duplicate an already owned logical surface. A
			// browser-session change needs explicit adoption/recovery, not URL matching.
			if old.Intent.Browser.Profile != b.Profile || old.Intent.Browser.Epoch != b.Epoch || old.Report == nil || old.Report.TabID < 1 {
				return false
			}
			if !p.Complete || p.Freshness == nil || !p.Freshness.Stable {
				return false
			}
			for _, id := range p.PresentTabs {
				if id == old.Report.TabID {
					return false
				}
			}
		}
		return true
	}
	for _, tab := range p.Tabs {
		if tab.ID == b.TabID && tab.OwnerID == b.OwnerID && tab.URL == b.ExpectedURL && !tab.NavigationPending && (b.Action != "associate" || tab.WindowID == b.WindowID) {
			if v.Workspace != nil {
				binding := st.ViewportBindings[st.ViewportHeads[v.SurfaceID]]
				proof := st.BrowserAssociations[binding.BrowserAssociationID]
				if !binding.Active || proof.WindowID != tab.WindowID || proof.Profile != p.ID || proof.Epoch != p.Epoch {
					return false
				}
			}
			return true
		}
	}
	return false
}

func nativeViewportCurrent(st State, v ActionIntent) bool {
	n := v.Native
	head := st.ViewportHeads[v.SurfaceID]
	if n.Launch != nil {
		if !ApplicationRecipeCurrent(st, n.Launch.RecipeID, v.Target, v.SurfaceID) {
			return false
		}
		if head != n.Launch.PreviousViewport && st.ViewportBindings[head].ApplicationActionID != v.ID {
			return false
		}
	} else if head != n.ViewportBindingID {
		return false
	}
	if n.RecipeID != "" && !ApplicationRecipeCurrent(st, n.RecipeID, v.Target, v.SurfaceID) {
		return false
	}
	return true
}
