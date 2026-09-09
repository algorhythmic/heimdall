package workspace

import (
	"heimdall/internal/model"
	"time"
)

func browserOperationAction(st model.State, i model.WorkspaceOperationIntent, v Preview, row PreviewSurface, kind string) (model.ActionIntent, string) {
	r := st.ApplicationRecipes[st.ApplicationHeads[row.SurfaceID]]
	if !model.ApplicationRecipeCurrent(st, r.ID, v.Request.Target, row.SurfaceID) || r.Spec.Browser == nil {
		return model.ActionIntent{}, "Reviewed paired-browser recipe required"
	}
	p := st.Browsers[r.Spec.Browser.Profile]
	if !p.Paired || p.ActionProtocol != 1 || p.VerificationProtocol != 1 || p.PairingProtocol != 1 || p.RecoveryProtocol != 1 || !p.Complete || i.At.Before(p.ReceivedAt) || i.At.Sub(p.ReceivedAt) >= 5*time.Second {
		return model.ActionIntent{}, "Fresh complete paired browser profile unavailable"
	}
	owned := []model.BrowserTab{}
	for _, tab := range p.Tabs {
		a := st.Actions[tab.OwnerID]
		if a.Intent.Target == v.Request.Target && a.Intent.SurfaceID == row.SurfaceID && a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Intent.Browser.Epoch == p.Epoch {
			owned = append(owned, tab)
		}
	}
	if len(owned) > 1 {
		return model.ActionIntent{}, "Multiple owned tabs require explicit scoped review"
	}
	b := model.BrowserIntent{Profile: p.ID, Epoch: p.Epoch, Action: kind}
	if len(owned) == 1 {
		t := owned[0]
		if t.NavigationPending || row.Window == nil {
			return model.ActionIntent{}, "Owned tab requires stable navigation and current compositor association"
		}
		if kind == "close" && r.Spec.ClosePolicy != "owned_tab" {
			return model.ActionIntent{}, "Browser leave-open policy preserves the owned tab"
		}
		proof := st.BrowserAssociations[st.ViewportBindings[row.ViewportBindingID].BrowserAssociationID]
		if proof.WindowID != t.WindowID || proof.Profile != p.ID || proof.Epoch != p.Epoch {
			return model.ActionIntent{}, "Owned tab moved outside its associated native window; explicit reassociation required"
		}
		b.TabID, b.OwnerID, b.ExpectedURL = t.ID, t.OwnerID, t.URL
	} else {
		if i.Kind != "open" || kind != "focus" {
			return model.ActionIntent{}, "No current owned tab; browser-wide close or URL adoption is not authorized"
		}
		for _, tab := range p.Tabs {
			if tab.URL == r.Spec.Browser.URL {
				return model.ActionIntent{}, "Browser already restored a matching URL; explicit ownership review required, no duplicate created"
			}
		}
		for _, a := range st.Actions {
			if a.Intent.SurfaceID == row.SurfaceID && a.Intent.Browser != nil && a.Intent.Browser.Action == "open" && !model.Contains([]string{"refused", "cancelled"}, a.Execution) && (a.Intent.Browser.Epoch != p.Epoch || a.Report == nil) {
				return model.ActionIntent{}, "Previous browser attempt or epoch requires explicit reconciliation before creation"
			}
		}
		previous := row.ViewportBindingID
		if previous == "" {
			previous = "none"
		}
		b.Action, b.URL, b.LoadCondition = "open", r.Spec.Browser.URL, "complete"
		b.Pairing = &model.BrowserPairingIntent{Version: 1, SourceID: i.SourceID, SourceEpoch: i.SourceEpoch, PreviousViewport: previous}
	}
	a := model.ActionIntent{Version: 5, ID: model.NewID(), Target: v.Request.Target, TaskRevision: v.TaskRevision, ManifestID: v.Request.ManifestID, SurfaceID: row.SurfaceID, ContextDigest: model.ActionContextDigest(st, v.Request.Target), SnapshotID: v.Request.SnapshotID, Adapter: "browser", AttemptID: model.NewID(), Authority: "cli", AuthorityRef: "workspace-operation-" + i.ID, Workspace: &model.BrowserWorkspaceAction{OperationID: i.ID, RecipeID: r.ID, PreviousViewport: row.ViewportBindingID}, Browser: &b, Expected: model.BrowserPostcondition(b), At: i.At, ExpiresAt: i.At.Add(30 * time.Second)}
	return a, ""
}
