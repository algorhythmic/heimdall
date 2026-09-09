package store

import (
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"time"
)

func applyActionPairing(st model.State, a *model.ActionRecord, v model.ActionTransition) error {
	if v.Version != 3 || a.Intent.Browser.Pairing == nil {
		return fmt.Errorf("pairing transition requires versioned intent")
	}
	b := a.Intent.Browser
	p := st.Browsers[b.Profile]
	switch v.Kind {
	case "pair_ready":
		r := v.PairReady
		if v.Actor != "observer:browser" || r == nil || !reflect.DeepEqual(&r.ActionRef, a.BrowserRef()) || r.MarkerTabID < 1 || r.WindowID < 1 || a.DeliveryID == "" || !model.Contains([]string{"dispatching", "uncertain"}, a.Execution) || (a.Pairing != nil && a.Pairing.Ready != nil) {
			return fmt.Errorf("invalid marker-ready report")
		}
		if (b.Action == "open" && r.OriginalTabID != 0) || (b.Action == "associate" && (r.OriginalTabID != b.TabID || r.WindowID != b.WindowID || r.MarkerTabID == b.TabID)) {
			return fmt.Errorf("pairing report changed selected browser window")
		}
		a.Pairing = &model.BrowserPairingState{Ready: r, ReadyAt: v.At, FirstReport: a.Report}
		a.Report = nil
	case "pair_probe":
		probe := v.PairProbe
		if v.Actor != "coordinator" || a.Pairing == nil || a.Pairing.Ready == nil || a.Pairing.ContinuationDeliveryID != "" || a.Pairing.ProbeAttempts >= 3 || a.CancelRequested || !v.At.Before(a.Intent.ExpiresAt) || !model.ActionInputsCurrent(st, a.Intent) || !model.BrowserPairFresh(p, *a, v.At) || probe == nil || !model.OpaqueID.MatchString(probe.ID) || probe.BrowserDigest != model.ContentDigest(p) || probe.Connection != p.Connection || probe.EventGeneration != p.EventGeneration || probe.SourceID != b.Pairing.SourceID || probe.Window.Validate() != nil || probe.Window.SourceEpoch != b.Pairing.SourceEpoch || !model.TokenHashPattern.MatchString(probe.SnapshotID) || !model.IsBrowserPairNativeTitle(probe.MarkerTitle, a.Intent.ID) || probe.MatchingWindows != 1 || probe.CapturedAt.Before(p.ReceivedAt) || probe.CapturedAt.After(v.At) || v.At.Sub(probe.CapturedAt) > 5*time.Second {
			return fmt.Errorf("invalid first browser/native pairing observation")
		}
		copy := *a.Pairing
		a.Pairing = &copy
		a.Pairing.Probe = probe
		a.Pairing.ProbeAttempts++
		a.Pairing.AssociationID = ""
		a.Pairing.ContinuationID = ""
	case "pair_bound":
		proof := st.BrowserAssociations[v.AssociationID]
		if st.ViewportBindings[st.ViewportHeads[a.Intent.SurfaceID]].BrowserAssociationID != proof.ID || proof.ID == "" {
			return fmt.Errorf("association and viewport binding must be committed together")
		}
		if v.Actor != "coordinator" || a.Pairing == nil || a.Pairing.AssociationID != "" || !reflect.DeepEqual(&proof.ActionRef, a.BrowserRef()) || v.ContinuationID != proof.ContinuationID || v.ContinuationID == "" {
			return fmt.Errorf("pairing lacks observed association")
		}
		copy := *a.Pairing
		a.Pairing = &copy
		a.Pairing.AssociationID = proof.ID
		a.Pairing.ContinuationID = v.ContinuationID
	case "pair_continue":
		if v.Actor != "coordinator" || a.Pairing == nil || a.Pairing.AssociationID == "" || a.Pairing.ContinuationDeliveryID != "" || a.CancelRequested || v.ContinuationID != a.Pairing.ContinuationID || !model.OpaqueID.MatchString(v.DeliveryID) || !v.At.Before(a.Intent.ExpiresAt) || !model.ActionInputsCurrent(st, a.Intent) || !model.ActionBrowserCurrent(st, a.Intent) || !model.BrowserPairFresh(p, *a, v.At) {
			return fmt.Errorf("pairing continuation is not authorized")
		}
		o := v.Observation
		if o == nil || o.Status != "unknown" || !model.OpaqueID.MatchString(o.ID) || o.Digest != model.ContentDigest(p) || o.SourceEpoch != p.Epoch || !o.ObservedAt.Equal(p.ReceivedAt) || p.Freshness == nil || !p.Freshness.Stable || !p.Complete || p.Freshness.Challenge.AfterEventID < a.LastEventID || v.At.Before(p.ReceivedAt) || v.At.Sub(p.ReceivedAt) > 5*time.Second {
			return fmt.Errorf("continuation lacks fresh marker readback")
		}
		copy := *a.Pairing
		a.Pairing = &copy
		a.Pairing.ContinuationDeliveryID = v.DeliveryID
		a.Execution = "dispatching"
	case "pair_abandoned":
		if v.Actor != "observer:browser" || a.Pairing == nil || a.Pairing.Ready == nil || a.Pairing.ContinuationDeliveryID != "" || a.Pairing.Abandoned || p.Epoch != b.Epoch || !p.Paired || p.Freshness == nil || !p.Freshness.Stable || !p.Complete || p.Freshness.Challenge.AfterEventID < a.LastEventID {
			return fmt.Errorf("incomplete pairing cannot be released")
		}
		for _, id := range p.PresentTabs {
			if id == a.Pairing.Ready.MarkerTabID {
				return fmt.Errorf("pairing marker still exists")
			}
		}
		copy := *a.Pairing
		a.Pairing = &copy
		a.Pairing.Abandoned = true
		a.Execution = "cancelled"
		a.Verification = "not_matched"
	default:
		return fmt.Errorf("unknown pairing transition")
	}
	return nil
}
