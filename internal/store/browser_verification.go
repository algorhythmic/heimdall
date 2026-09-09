package store

import (
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"slices"
	"time"
)

func applyBrowserVerification(st *model.State, e Event) error {
	if e.Verb == "challenge_issued" {
		var c model.BrowserChallenge
		if err := model.StrictJSON(e.Payload, &c); err != nil {
			return err
		}
		p := st.Browsers[c.Profile]
		if c.Version != 1 || e.Actor != "coordinator" || e.EntityID != p.ID || !p.Paired || p.VerificationProtocol != 1 || c.Epoch != p.Epoch || c.Connection != p.Connection || !model.OpaqueID.MatchString(c.ID) || !model.OpaqueID.MatchString(c.RuntimeID) || c.AfterEventID != e.ID-2 || !c.IssuedAt.Equal(e.TS) || c.ExpiresAt.Sub(c.IssuedAt) != 5*time.Second || len(c.Actions) > 128 {
			return fmt.Errorf("invalid browser challenge provenance")
		}
		seen := map[string]bool{}
		for _, r := range c.Actions {
			a := st.Actions[r.ID]
			if r.Validate() != nil || !reflect.DeepEqual(&r, a.BrowserRef()) || a.Intent.Browser.Profile != p.ID || seen[r.ID] {
				return fmt.Errorf("invalid challenge action scope")
			}
			seen[r.ID] = true
		}
		p.Challenge = &c
		st.Browsers[p.ID] = p
		return nil
	}
	var r model.BrowserReadback
	if err := model.StrictJSON(e.Payload, &r); err != nil {
		return err
	}
	p := st.Browsers[r.Profile]
	c := p.Challenge
	if r.Version != 1 || e.Actor != "observer:browser" || e.EntityID != p.ID || !p.Paired || c == nil || r.ChallengeID != c.ID || p.Epoch != c.Epoch || p.Connection != c.Connection || r.Sequence <= p.LastSequence || !r.ReceivedAt.Equal(e.TS) || r.ReceivedAt.Before(c.IssuedAt) || !r.ReceivedAt.Before(c.ExpiresAt) || r.ObservedAt.IsZero() || len(r.Tabs) > 2048 || len(r.PresentTabs) > 2048 || len(r.Instances) > 128 {
		return fmt.Errorf("invalid challenged browser readback")
	}
	present := map[int]bool{}
	for _, id := range r.PresentTabs {
		if id < 1 || present[id] {
			return fmt.Errorf("invalid tab ID census")
		}
		present[id] = true
	}
	tabs := map[int]model.BrowserTab{}
	for _, t := range r.Tabs {
		if t.ID < 1 || t.WindowID < 1 || !present[t.ID] || !model.BrowserURL(t.URL) || len(t.Title) > 1024 || t.OwnerID != "" || (t.LoadStatus != "" && t.LoadStatus != "loading" && t.LoadStatus != "complete" && t.LoadStatus != "unloaded") {
			return fmt.Errorf("invalid readback tab")
		}
		if _, ok := tabs[t.ID]; ok {
			return fmt.Errorf("duplicate readback tab")
		}
		// Historical unscoped opens keep their existing adapter ownership only.
		for _, op := range st.BrowserOperations {
			if op.ActionRef == nil && op.Profile == p.ID && op.Epoch == p.Epoch && op.Action == "open" && op.Status == "succeeded" && op.TabID == t.ID {
				t.OwnerID = op.ID
				break
			}
		}
		tabs[t.ID] = t
	}
	owners := map[string]bool{}
	instances := map[int]bool{}
	for _, i := range r.Instances {
		a, ok := st.Actions[i.ActionRef.ID]
		if !ok || i.ActionRef.Validate() != nil || !reflect.DeepEqual(&i.ActionRef, a.BrowserRef()) || a.Intent.Browser.Profile != p.ID || a.Intent.Browser.Epoch != p.Epoch || a.Intent.Browser.Action != "open" || a.DeliveryID == "" || !present[i.TabID] || owners[i.ActionRef.ID] || instances[i.TabID] {
			return fmt.Errorf("invalid exact browser instance provenance")
		}
		// A recovered journal result, when present, must name this exact tab.
		if a.Report != nil && a.Report.TabID > 0 && a.Report.TabID != i.TabID {
			return fmt.Errorf("browser instance differs from retained attempt report")
		}
		owners[i.ActionRef.ID] = true
		instances[i.TabID] = true
		if t, ok := tabs[i.TabID]; ok {
			t.OwnerID = i.ActionRef.ID
			tabs[i.TabID] = t
		}
	}

	if r.EventGeneration < 0 || r.EventGeneration < p.EventGeneration || len(r.Markers) > 128 || (p.PairingProtocol != 1 && len(r.Markers) > 0) {
		return fmt.Errorf("invalid browser event/marker coverage")
	}
	markerIDs := map[int]bool{}
	markerActions := map[string]bool{}
	for _, m := range r.Markers {
		a, ok := st.Actions[m.ActionRef.ID]
		if !ok || a.Intent.Browser == nil || a.Intent.Browser.Profile != p.ID || a.Intent.Browser.Epoch != p.Epoch || a.Intent.Browser.Pairing == nil || a.Pairing == nil || a.Pairing.Ready == nil || !reflect.DeepEqual(&m.ActionRef, a.BrowserRef()) || m.TabID != a.Pairing.Ready.MarkerTabID || m.WindowID != a.Pairing.Ready.WindowID || !present[m.TabID] || markerIDs[m.TabID] || markerActions[m.ActionRef.ID] {
			return fmt.Errorf("marker does not match its issued browser attempt")
		}
		markerIDs[m.TabID] = true
		markerActions[m.ActionRef.ID] = true
	}
	p.Markers = slices.Clone(r.Markers)
	if len(p.Markers) == 0 {
		p.Markers = nil
	}
	p.EventGeneration = r.EventGeneration
	p.Tabs = []model.BrowserTab{}
	for _, t := range tabs {
		p.Tabs = append(p.Tabs, t)
	}
	slices.SortFunc(p.Tabs, func(a, b model.BrowserTab) int { return a.ID - b.ID })
	p.PresentTabs = slices.Clone(r.PresentTabs)
	if len(p.PresentTabs) == 0 {
		p.PresentTabs = nil
	}
	slices.Sort(p.PresentTabs)
	p.FocusedWindow = r.FocusedWindow
	p.Complete = r.Complete
	p.LastSequence = r.Sequence
	p.LastObservedAt = r.ObservedAt
	p.ReceivedAt = r.ReceivedAt
	p.ReceivedEpoch = c.RuntimeID
	p.Freshness = &model.BrowserFreshness{Challenge: *c, Sequence: r.Sequence, Stable: r.Stable}
	st.Browsers[p.ID] = p
	return nil
}
