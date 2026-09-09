package model

import "time"

func BrowserOutcomeInState(st State, a ActionRecord, p BrowserProfile) (string, string) {
	if a.Intent.Browser.Pairing != nil && a.Pairing != nil && a.Pairing.AssociationID != "" {
		proof := st.BrowserAssociations[a.Pairing.AssociationID]
		binding := st.ViewportBindings[st.ViewportHeads[a.Intent.SurfaceID]]
		if proof.ID == "" || binding.BrowserAssociationID != proof.ID || !binding.Active || st.DesktopSourceHead != proof.SourceID || !st.DesktopSources[proof.SourceID].Active || st.DesktopSources[proof.SourceID].Epoch != proof.Window.SourceEpoch {
			return "unknown", "Native association source or viewport changed; explicit recovery required"
		}
	}
	return BrowserOutcome(a, p)
}

// A nonce requests a new read after this event cursor. It is usable only in the
// issuing daemon and connection; the transport additionally enforces monotonic TTL.
type BrowserChallenge struct {
	Version      int                `json:"version"`
	ID           string             `json:"id"`
	Profile      string             `json:"profile"`
	Epoch        string             `json:"epoch"`
	Connection   string             `json:"connection"`
	RuntimeID    string             `json:"runtime_id"`
	AfterEventID int64              `json:"after_event_id"`
	IssuedAt     time.Time          `json:"issued_at"`
	ExpiresAt    time.Time          `json:"expires_at"`
	Actions      []BrowserActionRef `json:"actions"`
}
type BrowserInstance struct {
	TabID     int              `json:"tab_id"`
	ActionRef BrowserActionRef `json:"action_ref"`
}
type BrowserReadback struct {
	EventGeneration int64             `json:"event_generation,omitempty"`
	Markers         []BrowserMarker   `json:"markers,omitempty"`
	Version         int               `json:"version"`
	Profile         string            `json:"profile"`
	ChallengeID     string            `json:"challenge_id"`
	Sequence        int64             `json:"sequence"`
	ObservedAt      time.Time         `json:"observed_at"`
	ReceivedAt      time.Time         `json:"received_at"`
	Complete        bool              `json:"complete"`
	Stable          bool              `json:"stable"`
	Tabs            []BrowserTab      `json:"tabs"`
	PresentTabs     []int             `json:"present_tabs"`
	Instances       []BrowserInstance `json:"instances"`
	FocusedWindow   int               `json:"focused_window"`
}
type BrowserFreshness struct {
	Challenge BrowserChallenge `json:"challenge"`
	Sequence  int64            `json:"sequence"`
	Stable    bool             `json:"stable"`
}

// BrowserOutcome evaluates independently observed browser metadata. Missing
// filtered URLs cannot prove closure: absence requires the complete ID census.
// The report establishes API settlement, never the postcondition itself.
func BrowserOutcome(a ActionRecord, p BrowserProfile) (string, string) {
	if a.Intent.Browser == nil || a.Execution == "queued" || a.Execution == "refused" || a.Execution == "cancelled" {
		return "unsupported", "No dispatched browser attempt"
	}
	b := a.Intent.Browser
	if !p.Paired || p.Epoch != b.Epoch || p.Freshness == nil || p.Freshness.Challenge.Epoch != p.Epoch || p.Freshness.Challenge.Connection != p.Connection || p.Freshness.Sequence != p.LastSequence || !p.Freshness.Stable || !p.Complete || p.Freshness.Challenge.AfterEventID < a.LastEventID {
		return "unknown", "Fresh stable readback after the latest action transition required"
	}
	// An unfinished API call can still change the app after this observation.
	if a.Report == nil {
		return "unknown", "Attempt result is missing; retained journal reconciliation required"
	}
	if b.Pairing != nil && (a.Pairing == nil || a.Pairing.AssociationID == "" || a.Pairing.ContinuationDeliveryID == "") {
		return "unknown", "Native window association and continuation are incomplete"
	}
	owner, tabID := b.OwnerID, b.TabID
	if b.Action == "open" {
		owner = a.Intent.ID
		tabID = a.Report.TabID
	}
	var tab *BrowserTab
	for i := range p.Tabs {
		t := &p.Tabs[i]
		if t.ID == tabID && t.OwnerID == owner {
			tab = t
			break
		}
	}
	present := false
	for _, id := range p.PresentTabs {
		if id == tabID {
			present = true
		}
	}
	if tabID < 1 {
		return "unknown", "No exact runtime instance was recovered"
	}
	if b.Action == "close" {
		if !present {
			return "matched", "Exact owned tab is absent; application data preservation is not asserted"
		}
		if tab == nil {
			return "unknown", "Tab remains but its allowed URL/ownership is unavailable"
		}
		return "not_matched", "Exact owned tab remains open"
	}
	if tab == nil {
		if present {
			return "unknown", "Tab exists outside verified URL/ownership coverage"
		}
		return "not_matched", "Exact owned tab is absent"
	}
	if b.Pairing != nil {
		if tab.WindowID != a.Pairing.Ready.WindowID {
			return "not_matched", "Owned tab moved out of the associated browser window"
		}
		if b.Action == "associate" {
			for _, id := range p.PresentTabs {
				if id == a.Pairing.Ready.MarkerTabID {
					return "not_matched", "Temporary pairing tab remains open"
				}
			}
			if tab.URL != b.ExpectedURL || tab.NavigationPending {
				return "not_matched", "Associated original tab navigation changed"
			}
		}
	}
	if b.Action == "open" || b.Action == "navigate" {
		if tab.NavigationPending {
			return "unknown", "Navigation is still pending; committed URL is not a settled outcome"
		}
		if tab.URL != b.URL {
			return "not_matched", "URL differs from the explicit exact redirect policy"
		}
		if a.Intent.Expected.LoadCondition == "complete" && (tab.LoadStatus != "complete" || tab.Discarded) {
			return "not_matched", "Requested completed load was not observed"
		}
	}
	if b.Action == "focus" && (!tab.Active || p.FocusedWindow != tab.WindowID) {
		return "not_matched", "Tab is not active in the focused window"
	}
	if b.Action == "move" && tab.WindowID != b.WindowID {
		return "not_matched", "Tab is not in the requested window"
	}
	return "matched", "Requested browser postcondition observed in challenged readback"
}
