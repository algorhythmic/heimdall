package model

import (
	"fmt"
	"reflect"
	"regexp"
	"time"
)

func BrowserPairMarker(p BrowserProfile, a ActionRecord) bool {
	if a.Pairing == nil || a.Pairing.Ready == nil {
		return false
	}
	r := a.Pairing.Ready
	for _, m := range p.Markers {
		if m.TabID == r.MarkerTabID && m.WindowID == r.WindowID && m.Active && reflect.DeepEqual(&m.ActionRef, a.BrowserRef()) {
			return true
		}
	}
	return false
}
func BrowserPairFresh(p BrowserProfile, a ActionRecord, now time.Time) bool {
	return p.Paired && p.Epoch == a.Intent.Browser.Epoch && p.PairingProtocol == 1 && p.Complete && p.Freshness != nil && p.Freshness.Stable && p.Freshness.Sequence == p.LastSequence && p.Freshness.Challenge.Connection == p.Connection && p.Freshness.Challenge.AfterEventID >= a.LastEventID && !now.Before(p.ReceivedAt) && now.Sub(p.ReceivedAt) < 5*time.Second && BrowserPairMarker(p, a)
}

var BrowserExtensionIDPattern = regexp.MustCompile(`^[a-p]{32}$`)

// Pairing is an explicit additional authority on a browser action. It pins the
// selected compositor and previous viewport binding, not an inferred page title.
type BrowserPairingIntent struct {
	Version          int    `json:"version"`
	SourceID         string `json:"source_id"`
	SourceEpoch      string `json:"source_epoch"`
	PreviousViewport string `json:"previous_viewport"`
}

func (p BrowserPairingIntent) Validate() error {
	if p.Version != 1 || !OpaqueID.MatchString(p.SourceID) || !TokenHashPattern.MatchString(p.SourceEpoch) || (p.PreviousViewport != "none" && !OpaqueID.MatchString(p.PreviousViewport)) {
		return fmt.Errorf("explicit pairing source, epoch and previous viewport required")
	}
	return nil
}
func BrowserPairTitle(nonce string) string { return "Heimdall pairing " + nonce }
func BrowserPairURL(extension, nonce string) string {
	return "chrome-extension://" + extension + "/pair.html#" + nonce
}

// Native matching is pinned to the supported Linux Chromium normal-window title.
func BrowserPairNativeTitle(nonce string) string { return BrowserPairTitle(nonce) + " - Chromium" }
func IsBrowserPairNativeTitle(title, nonce string) bool {
	return title == BrowserPairNativeTitle(nonce) || title == BrowserPairTitle(nonce)+" - Google Chrome for Testing"
}

// A browser binding owns an outer window, not the contents of every tab. Strip
// page-identifying metadata from scoped viewport/snapshot reads and omit markers.
func ScopedBrowserWindow(st State, b ViewportBinding, w DesktopWindow) (DesktopWindow, bool) {
	if b.BrowserAssociationID == "" {
		return w, true
	}
	proof := st.BrowserAssociations[b.BrowserAssociationID]
	p := st.Browsers[proof.Profile]
	if proof.ID == "" || !p.Paired || p.Epoch != proof.Epoch {
		return DesktopWindow{}, false
	}
	if IsBrowserPairNativeTitle(w.Title, proof.ActionRef.ID) {
		return DesktopWindow{}, false
	}
	a := st.Actions[proof.ActionRef.ID]
	if a.Pairing == nil || a.Pairing.Abandoned || a.Pairing.ContinuationDeliveryID == "" || a.Report == nil {
		return DesktopWindow{}, false
	}
	w.Title = ""
	w.Class = ""
	w.PID = 0
	return w, true
}

type BrowserPairReady struct {
	ActionRef     BrowserActionRef `json:"action_ref"`
	MarkerTabID   int              `json:"marker_tab_id"`
	WindowID      int              `json:"window_id"`
	OriginalTabID int              `json:"original_tab_id,omitempty"`
}
type BrowserMarker struct {
	ActionRef BrowserActionRef `json:"action_ref"`
	TabID     int              `json:"tab_id"`
	WindowID  int              `json:"window_id"`
	Active    bool             `json:"active"`
}
type BrowserPairProbe struct {
	ID              string         `json:"id"`
	BrowserDigest   string         `json:"browser_digest"`
	Connection      string         `json:"connection"`
	EventGeneration int64          `json:"event_generation"`
	SourceID        string         `json:"source_id"`
	Window          WindowIdentity `json:"window"`
	SnapshotID      string         `json:"snapshot_id"`
	CapturedAt      time.Time      `json:"captured_at"`
	MarkerTitle     string         `json:"marker_title"`
	MatchingWindows int            `json:"matching_windows"`
}

type BrowserPairingState struct {
	FirstReport            *ActionReport     `json:"first_report,omitempty"`
	Probe                  *BrowserPairProbe `json:"probe,omitempty"`
	ProbeAttempts          int               `json:"probe_attempts,omitempty"`
	Ready                  *BrowserPairReady `json:"ready,omitempty"`
	ReadyAt                time.Time         `json:"ready_at"`
	AssociationID          string            `json:"association_id,omitempty"`
	ContinuationID         string            `json:"continuation_id,omitempty"`
	ContinuationDeliveryID string            `json:"continuation_delivery_id,omitempty"`
	Abandoned              bool              `json:"abandoned,omitempty"`
}
type BrowserContinuation struct {
	Version       int              `json:"version"`
	ID            string           `json:"id"`
	ActionRef     BrowserActionRef `json:"action_ref"`
	Profile       string           `json:"profile"`
	Epoch         string           `json:"epoch"`
	MarkerTabID   int              `json:"marker_tab_id"`
	WindowID      int              `json:"window_id"`
	OriginalTabID int              `json:"original_tab_id,omitempty"`
	Action        string           `json:"action"`
	URL           string           `json:"url,omitempty"`
	ExpiresAt     time.Time        `json:"expires_at"`
}
type BrowserAssociation struct {
	ProbeID         string           `json:"probe_id"`
	Version         int              `json:"version"`
	ID              string           `json:"id"`
	ActionRef       BrowserActionRef `json:"action_ref"`
	Profile         string           `json:"profile"`
	Epoch           string           `json:"epoch"`
	WindowID        int              `json:"window_id"`
	MarkerTabID     int              `json:"marker_tab_id"`
	SourceID        string           `json:"source_id"`
	Window          WindowIdentity   `json:"window"`
	SnapshotID      string           `json:"snapshot_id"`
	CapturedAt      time.Time        `json:"captured_at"`
	BrowserDigest   string           `json:"browser_digest"`
	MatchingWindows int              `json:"matching_windows"`
	MarkerTitle     string           `json:"marker_title"`
	ContinuationID  string           `json:"continuation_id"`
	At              time.Time        `json:"at"`
}
