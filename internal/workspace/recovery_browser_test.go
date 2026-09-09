package workspace

import (
	"heimdall/internal/model"
	"testing"
	"time"
)

func TestRecoveryBrowserRequiresFreshOwnedMembership(t *testing.T) {
	st, point, request, ids, observed := recoveryPlanFixture(t)
	id, now := ids[0], time.Now().UTC()
	m := st.WorkspaceManifests[st.WorkspaceHeads["alpha"]]
	m.Surfaces[0].Kind = "browser"
	st.WorkspaceManifests[m.ID] = m
	a := model.ActionRecord{Intent: model.ActionIntent{ID: model.NewID(), Target: "alpha", ManifestID: m.ID, SurfaceID: id, Browser: &model.BrowserIntent{Action: "open", Profile: "profile", Epoch: "epoch", URL: "https://example.test/owned"}}, Execution: "api_reported", Verification: "matched", LastEventID: 10, Report: &model.ActionReport{TabID: 1}, Pairing: &model.BrowserPairingState{ContinuationDeliveryID: model.NewID()}}
	st.Actions[a.Intent.ID] = a
	b := st.ViewportBindings[st.ViewportHeads[id]]
	proof := model.BrowserAssociation{ID: model.NewID(), ActionRef: *a.BrowserRef(), Profile: "profile", Epoch: "epoch", WindowID: 10, SourceID: st.DesktopSourceHead, Window: *b.Window}
	b.BrowserAssociationID = proof.ID
	st.ViewportBindings[b.ID] = b
	st.BrowserAssociations[proof.ID] = proof
	p := model.BrowserProfile{ID: "profile", Epoch: "epoch", Connection: "connection", Paired: true, Complete: true, LastSequence: 1, ReceivedAt: now, Freshness: &model.BrowserFreshness{Sequence: 1, Stable: true, Challenge: model.BrowserChallenge{ID: model.NewID(), Epoch: "epoch", Connection: "connection", AfterEventID: 10, ExpiresAt: now.Add(time.Second)}}, Tabs: []model.BrowserTab{{ID: 1, WindowID: 10, OwnerID: a.Intent.ID, URL: a.Intent.Browser.URL, LoadStatus: "complete"}}, PresentTabs: []int{1}}
	st.Browsers[p.ID] = p
	recipe := model.ApplicationRecipe{ID: model.NewID(), Target: "alpha", SurfaceID: id, ManifestID: m.ID, TaskRevision: 1, Active: true, Spec: &model.ApplicationSpec{Adapter: "browser", Browser: &model.BrowserRestore{Profile: p.ID, URL: a.Intent.Browser.URL}}}
	st.ApplicationHeads[id] = recipe.ID
	st.ApplicationRecipes[recipe.ID] = recipe
	v := planRecovery(st, point, request, ids, "open", observed, nil, nil, now)
	if !v.Full || v.Surfaces[0].Application.Status != "matched" || v.ExpiresAt != p.Freshness.Challenge.ExpiresAt {
		t.Fatal(v)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*model.BrowserProfile)
	}{
		{"moved tab", func(p *model.BrowserProfile) { p.Tabs[0].WindowID = 99 }},
		{"self-restored unowned tab", func(p *model.BrowserProfile) { p.Tabs[0].OwnerID = "" }},
		{"changed URL", func(p *model.BrowserProfile) { p.Tabs[0].URL = "https://example.test/other" }},
		{"pending navigation", func(p *model.BrowserProfile) { p.Tabs[0].NavigationPending = true }},
		{"discarded tab", func(p *model.BrowserProfile) { p.Tabs[0].Discarded = true }},
		{"new epoch", func(p *model.BrowserProfile) { p.Epoch = "new" }},
		{"new connection", func(p *model.BrowserProfile) { p.Connection = "new" }},
		{"unchallenged inventory", func(p *model.BrowserProfile) { p.Freshness = nil }},
		{"stale read", func(p *model.BrowserProfile) { p.ReceivedAt = now.Add(-6 * time.Second) }},
		{"old sequence", func(p *model.BrowserProfile) { p.LastSequence++ }},
		{"incomplete census", func(p *model.BrowserProfile) { p.Complete = false }},
		{"before latest action", func(p *model.BrowserProfile) { p.Freshness.Challenge.AfterEventID = 9 }},
		{"expired challenge", func(p *model.BrowserProfile) { p.Freshness.Challenge.ExpiresAt = now }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := model.Clone(st)
			profile := model.Clone(p)
			tc.mutate(&profile)
			s.Browsers[p.ID] = profile
			v := planRecovery(s, point, request, ids, "open", observed, nil, nil, now)
			if v.Full {
				t.Fatal(v)
			}
		})
	}
	// A complete native window absence is not enough for an exact tab close;
	// only a current challenged browser ID census can establish it.
	close := a
	close.Intent.ID = model.NewID()
	close.Intent.Browser = &model.BrowserIntent{Action: "close", Profile: p.ID, Epoch: p.Epoch, TabID: 1, OwnerID: a.Intent.ID, ExpectedURL: a.Intent.Browser.URL}
	close.Pairing = nil
	st.Actions[close.Intent.ID] = close
	op := model.WorkspaceOperation{ActionIDs: []string{close.Intent.ID}}
	row := v.Surfaces[0]
	recoveryBrowser(st, b, request, "close", op, true, false, now, &row)
	if row.Existence.Status != "not_matched" {
		t.Fatal("existing tab was closed by native absence", row)
	}
	p.PresentTabs = []int{}
	p.Tabs = []model.BrowserTab{}
	st.Browsers[p.ID] = p
	recoveryBrowser(st, b, request, "close", op, true, false, now, &row)
	if row.Existence.Status != "matched" {
		t.Fatal(row)
	}
}
