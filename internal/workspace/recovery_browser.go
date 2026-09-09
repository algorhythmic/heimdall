package workspace

import (
	"heimdall/internal/model"
	"time"
)

func recoveryBrowser(st model.State, binding model.ViewportBinding, request RecoveryRequest, kind string, op model.WorkspaceOperation, nativeKnown, nativePresent bool, now time.Time, row *RecoverySurface) {
	row.Application = unknown("Fresh challenged browser membership and exact native association required")
	proof := st.BrowserAssociations[binding.BrowserAssociationID]
	p := st.Browsers[proof.Profile]
	if proof.ID == "" || binding.Window == nil || proof.Window != *binding.Window || proof.SourceID != st.DesktopSourceHead || proof.ActionRef.Target != request.Target || proof.ActionRef.SurfaceID != row.SurfaceID {
		row.Ownership = unknown("Current explicit browser/native association required")
		return
	}
	latest := st.Actions[proof.ActionRef.ID].LastEventID
	row.Evidence.BrowserProfile, row.Evidence.BrowserEpoch = p.ID, p.Epoch
	row.Evidence.BrowserReceivedAt = p.ReceivedAt
	if p.Freshness != nil {
		row.Evidence.BrowserChallengeID = p.Freshness.Challenge.ID
		row.Evidence.BrowserSequence = p.Freshness.Sequence
	}
	for _, a := range st.Actions {
		if a.Intent.Target == request.Target && a.Intent.SurfaceID == row.SurfaceID && a.LastEventID > latest {
			latest = a.LastEventID
		}
	}
	if !p.Paired || p.Epoch != proof.Epoch || p.Freshness == nil || !p.Complete || !p.Freshness.Stable || p.Freshness.Sequence != p.LastSequence || p.Freshness.Challenge.Epoch != p.Epoch || p.Freshness.Challenge.Connection != p.Connection || p.Freshness.Challenge.AfterEventID < latest || now.Before(p.ReceivedAt) || now.Sub(p.ReceivedAt) >= 5*time.Second || !now.Before(p.Freshness.Challenge.ExpiresAt) {
		row.Ownership = unknown("Browser epoch, connection, complete census or challenged freshness unavailable")
		return
	}
	if !nativeKnown {
		row.Ownership = unknown("Native identity coverage unavailable")
		return
	}
	if nativePresent {
		w := model.DesktopWindow{Identity: *binding.Window}
		if _, ok := model.ScopedBrowserWindow(st, binding, w); !ok {
			row.Ownership = unknown("Native pairing continuation is incomplete")
			return
		}
	}
	if kind == "close" {
		var close *model.ActionRecord
		for _, aid := range op.ActionIDs {
			a := st.Actions[aid]
			if a.Intent.Target == request.Target && a.Intent.SurfaceID == row.SurfaceID && a.Intent.Browser != nil && a.Intent.Browser.Action == "close" {
				copy := a
				close = &copy
			}
		}
		if close == nil {
			row.Existence = unknown("Exact owned-tab close attempt required")
			return
		}
		status, reason := model.BrowserOutcome(*close, p)
		row.Existence = recoveryCheck(status, reason)
		row.Application = matched("Exact browser ID census checked; saved page data is not asserted")
		if nativePresent && status == "matched" {
			row.Application = recoveryCheck("degraded", "Owned tab is absent; native window remains, and other tabs are left alone")
		}
		return
	}
	owned := []model.BrowserTab{}
	for _, tab := range p.Tabs {
		a := st.Actions[tab.OwnerID]
		if a.Intent.Target == request.Target && a.Intent.SurfaceID == row.SurfaceID && a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Intent.Browser.Epoch == p.Epoch {
			owned = append(owned, tab)
		}
	}
	if len(owned) != 1 {
		row.Application = unknown("One currently owned tab required; URLs never establish ownership")
		return
	}
	tab := owned[0]
	if tab.WindowID != proof.WindowID {
		row.WorkspaceMembership = mismatch("Owned browser tab moved outside its associated native window")
		row.Application = mismatch("Browser window membership changed")
		return
	}
	if tab.NavigationPending || tab.LoadStatus != "complete" || tab.Discarded {
		row.Application = unknown("Browser navigation or load is not settled")
		return
	}
	recipe := st.ApplicationRecipes[st.ApplicationHeads[row.SurfaceID]]
	if !model.ApplicationRecipeCurrent(st, recipe.ID, request.Target, row.SurfaceID) || recipe.Spec.Browser == nil || recipe.Spec.Browser.Profile != p.ID {
		row.Application = unknown("Current reviewed browser URL/profile required")
		return
	}
	if tab.URL != recipe.Spec.Browser.URL {
		row.Application = mismatch("Owned tab URL differs from the reviewed recovery recipe")
		return
	}
	row.Application = matched("Exact owned tab, reviewed committed URL, completed load and paired native window membership")
}
