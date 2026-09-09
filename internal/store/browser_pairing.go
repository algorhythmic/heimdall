package store

import (
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"time"
)

// Both browser reads surround independent compositor inventories. The first
// probe is retained separately so the second challenge must follow it.
func applyBrowserAssociation(st *model.State, e Event) error {
	var v model.BrowserAssociation
	if err := model.StrictJSON(e.Payload, &v); err != nil {
		return err
	}
	a, ok := st.Actions[v.ActionRef.ID]
	if !ok || a.Intent.Browser.Pairing == nil || a.Pairing == nil || a.Pairing.Probe == nil || a.Pairing.Ready == nil {
		return fmt.Errorf("association lacks a pairing probe")
	}
	p, b, probe, ready := st.Browsers[v.Profile], a.Intent.Browser, a.Pairing.Probe, a.Pairing.Ready
	if v.Version != 1 || !model.OpaqueID.MatchString(v.ID) || e.EntityID != v.ID || e.CommandID != "browser-association-"+v.ID || e.Actor != "coordinator" || !v.At.Equal(e.TS) || !reflect.DeepEqual(&v.ActionRef, a.BrowserRef()) || a.Pairing.AssociationID != "" || a.Pairing.Abandoned || a.CancelRequested || !v.At.Before(a.Intent.ExpiresAt) || !model.ActionInputsCurrent(*st, a.Intent) || !model.ActionBrowserCurrent(*st, a.Intent) {
		return fmt.Errorf("association authority changed")
	}
	if _, exists := st.BrowserAssociations[v.ID]; exists {
		return fmt.Errorf("duplicate association")
	}
	if !model.BrowserPairFresh(p, a, v.At) || v.Profile != b.Profile || v.Epoch != b.Epoch || v.WindowID != ready.WindowID || v.MarkerTabID != ready.MarkerTabID || v.ProbeID != probe.ID || probe.Connection != p.Connection || probe.EventGeneration != p.EventGeneration || v.SourceID != b.Pairing.SourceID || v.Window != probe.Window || v.Window.SourceEpoch != b.Pairing.SourceEpoch || v.Window.Validate() != nil || v.BrowserDigest != model.ContentDigest(p) || v.MatchingWindows != 1 || (v.MarkerTitle != probe.MarkerTitle || !model.IsBrowserPairNativeTitle(v.MarkerTitle, a.Intent.ID)) || !model.TokenHashPattern.MatchString(v.SnapshotID) || !model.OpaqueID.MatchString(v.ContinuationID) {
		return fmt.Errorf("browser/compositor association changed or ambiguous")
	}
	if v.CapturedAt.Before(p.ReceivedAt) || !v.CapturedAt.After(probe.CapturedAt) || v.CapturedAt.After(v.At) || v.At.Sub(v.CapturedAt) >= 5*time.Second {
		return fmt.Errorf("association observations are not ordered and fresh")
	}
	st.BrowserAssociations[v.ID] = v
	return nil
}
