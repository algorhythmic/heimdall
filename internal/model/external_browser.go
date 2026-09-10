package model

import "time"

// The pairing owns a native container. Input additionally pins its active owned
// tab. Unknown/foreign tabs in that container refuse admission; no title inference
// creates a binding. The external harness still owns all input/approval guards.
type ExternalBrowser struct {
	AfterEventID  int64  `json:"after_event_id"`
	AssociationID string `json:"association_id"`
	Profile       string `json:"profile"`
	Epoch         string `json:"epoch"`
	TabID         int    `json:"tab_id"`
	WindowID      int    `json:"window_id"`
	OwnerID       string `json:"owner_id"`
}
type ExternalBrowserObservation struct {
	Profile  string `json:"profile"`
	Epoch    string `json:"epoch"`
	Sequence int64  `json:"sequence"`
	Digest   string `json:"digest"`
}

func ExternalBrowserCurrent(st State, x ExternalBrowser, w OwnedWindow) bool {
	b := st.ViewportBindings[w.BindingID]
	proof := st.BrowserAssociations[x.AssociationID]
	p := st.Browsers[x.Profile]
	owner := st.Actions[x.OwnerID]
	task, _, err := ResolveTarget(st, w.Target)
	return err == nil && b.BrowserAssociationID == x.AssociationID && proof.ID != "" && proof.SourceID == w.SourceID && proof.Window == w.Window && proof.WindowID == x.WindowID && proof.Profile == x.Profile && proof.Epoch == x.Epoch && p.Paired && p.Epoch == x.Epoch && owner.Intent.Target == task.Task.ID && owner.Intent.SurfaceID == w.SurfaceID && owner.Intent.Browser != nil && owner.Intent.Browser.Profile == x.Profile && owner.Intent.Browser.Epoch == x.Epoch && x.TabID > 0
}
func ExternalBrowserScope(st State, w OwnedWindow) (ExternalBrowser, bool) {
	proof := st.BrowserAssociations[st.ViewportBindings[w.BindingID].BrowserAssociationID]
	p := st.Browsers[proof.Profile]
	x := ExternalBrowser{AssociationID: proof.ID, Profile: proof.Profile, Epoch: proof.Epoch, WindowID: proof.WindowID}
	// A filtered-out tab has unknown container membership. It cannot be silently
	// counted as a safe tab; a complete allowed-URL census is required for input.
	for _, id := range p.PresentTabs {
		found := false
		for _, tab := range p.Tabs {
			if tab.ID == id {
				found = true
				break
			}
		}
		if !found {
			return x, false
		}
	}
	found := false
	for _, tab := range p.Tabs {
		if tab.WindowID != x.WindowID {
			continue
		}
		candidate := x
		candidate.TabID = tab.ID
		candidate.OwnerID = tab.OwnerID
		if !ExternalBrowserCurrent(st, candidate, w) {
			return x, false
		}
		if tab.Active {
			if found {
				return x, false
			}
			x.TabID = tab.ID
			x.OwnerID = tab.OwnerID
			found = true
		}
	}
	return x, found && ExternalBrowserCurrent(st, x, w)
}
func ExternalBrowserFresh(p BrowserProfile, after int64, now time.Time) bool {
	f := p.Freshness
	return p.Paired && p.Complete && f != nil && f.Stable && f.Sequence == p.LastSequence && f.Challenge.Profile == p.ID && f.Challenge.Epoch == p.Epoch && f.Challenge.Connection == p.Connection && f.Challenge.AfterEventID >= after && !now.Before(p.ReceivedAt) && now.Sub(p.ReceivedAt) < 5*time.Second
}
func ExternalBrowserTitle(st State, x ExternalBrowser, title string) bool {
	for _, tab := range st.Browsers[x.Profile].Tabs {
		if tab.ID == x.TabID && tab.OwnerID == x.OwnerID && tab.Active {
			return title == tab.Title+" - Chromium" || title == tab.Title+" - Google Chrome for Testing" || title == tab.Title+" - Google Chrome"
		}
	}
	return false
}
func ExternalBrowserOutcome(st State, a ActionRecord, o *ActionObservation, now time.Time) (string, string) {
	x := a.Intent.External.Browser
	p := st.Browsers[x.Profile]
	b := o.Browser
	if b == nil || b.Profile != x.Profile || b.Epoch != x.Epoch || b.Sequence != p.LastSequence || b.Digest != ContentDigest(p) || !ExternalBrowserFresh(p, a.LastEventID, now) {
		return "unknown", "Fresh challenged browser readback required"
	}
	var tab *BrowserTab
	present := false
	for _, id := range p.PresentTabs {
		if id == x.TabID {
			present = true
		}
	}
	for i := range p.Tabs {
		if p.Tabs[i].ID == x.TabID && p.Tabs[i].OwnerID == x.OwnerID {
			tab = &p.Tabs[i]
		}
	}
	expected := a.Intent.Expected
	if expected.Kind == "owned_tab_absent" {
		if !present {
			return "matched", "Exact owned tab absent; saved data not asserted"
		}
		return "not_matched", "Exact owned tab remains present"
	}
	if expected.Kind == "window_absent" {
		if !present && o.Native.Window == nil {
			return "matched", "Exact owned window and tab absent; saved data not asserted"
		}
		return "not_matched", "Owned browser remains present"
	}
	if tab == nil {
		if present {
			return "unknown", "Owned tab outside URL or ownership coverage"
		}
		return "not_matched", "Owned tab absent"
	}
	if tab.WindowID != x.WindowID {
		return "not_matched", "Owned tab moved outside registered window"
	}
	if expected.Kind == "owned_tab_url" {
		if tab.NavigationPending {
			return "unknown", "Navigation pending"
		}
		if tab.URL != expected.URL || (expected.LoadCondition == "complete" && (tab.LoadStatus != "complete" || tab.Discarded)) {
			return "not_matched", "Owned tab URL/load differs"
		}
		return "matched", "Exact owned tab URL/load observed"
	}
	if expected.Kind == "owned_tab_focused" || expected.Kind == "window_focused" {
		if !tab.Active || p.FocusedWindow != x.WindowID {
			return "not_matched", "Owned tab is not focused"
		}
	}
	return "", "" // Native container predicate is checked by ExternalOutcome.
}
