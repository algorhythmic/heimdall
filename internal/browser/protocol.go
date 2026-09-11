// Package browser defines a bounded, observation-only browser ingress and explicit
// user-authorized browser operations. It does not implement conversation scraping.
package browser

import (
	"fmt"
	"heimdall/internal/model"
	"regexp"
	"time"
)

const MaxFrame = 256 << 10
const HostName = "dev.heimdall.browser"

var IDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var ExtensionPattern = regexp.MustCompile(`^[a-p]{32}$`)

type Message struct {
	FocusSpans []model.BrowserFocusSpan `json:"focus_spans,omitempty"`

	Delta                bool                    `json:"delta,omitempty"`
	BaseSequence         int64                   `json:"base_seq,omitempty"`
	Removed              []int                   `json:"removed,omitempty"`
	RecoveryProtocol     int                     `json:"recovery_protocol,omitempty"`
	PairingProtocol      int                     `json:"pairing_protocol,omitempty"`
	ExtensionID          string                  `json:"extension_id,omitempty"`
	PairReady            *model.BrowserPairReady `json:"pair_ready,omitempty"`
	Markers              []model.BrowserMarker   `json:"markers,omitempty"`
	EventGeneration      *int64                  `json:"event_generation,omitempty"`
	VerificationProtocol int                     `json:"verification_protocol,omitempty"`
	ChallengeID          string                  `json:"challenge_id,omitempty"`
	Stable               *bool                   `json:"stable,omitempty"`
	PresentTabs          []int                   `json:"present_tabs,omitempty"`
	Instances            []model.BrowserInstance `json:"instances,omitempty"`
	ActionProtocol       int                     `json:"action_protocol,omitempty"`
	V                    int                     `json:"v"`
	Type                 string                  `json:"type"`
	ID                   string                  `json:"id"`
	Profile              string                  `json:"profile"`
	Epoch                string                  `json:"epoch"`
	Connection           string                  `json:"connection"`
	Label                string                  `json:"label,omitempty"`
	ExtensionVersion     string                  `json:"extension_version,omitempty"`
	Sequence             int64                   `json:"seq,omitempty"`
	ObservedAt           string                  `json:"observed_at,omitempty"`
	Tabs                 []model.BrowserTab      `json:"tabs,omitempty"`
	FocusedWindow        *int                    `json:"focused_window,omitempty"`
	Complete             *bool                   `json:"complete,omitempty"`
	Result               *OperationResult        `json:"result,omitempty"`
	Capture              *BrowserCapture         `json:"capture,omitempty"`
}

// BrowserCapture is a paired profile's explicit capture request; the pointer
// is the operator-chosen URL the popup records, never an inferred tab.
type BrowserCapture struct {
	Line    string `json:"line"`
	Pointer string `json:"pointer"`
	Title   string `json:"title,omitempty"`
}
type OperationResult struct {
	ContinuationID string                  `json:"continuation_id,omitempty"`
	ActionRef      *model.BrowserActionRef `json:"action_ref,omitempty"`
	OperationID    string                  `json:"operation_id"`
	Status         string                  `json:"status"`
	TabID          int                     `json:"tab_id,omitempty"`
	WindowID       int                     `json:"window_id,omitempty"`
	URL            string                  `json:"url,omitempty"`
	Detail         string                  `json:"detail,omitempty"`
}
type Reply struct {
	Continuations []model.BrowserContinuation `json:"continuations,omitempty"`
	Challenge     *model.BrowserChallenge     `json:"challenge,omitempty"`
	V             int                         `json:"v"`
	Type          string                      `json:"type"`
	ID            string                      `json:"id"`
	Profile       string                      `json:"profile,omitempty"`
	Paired        bool                        `json:"paired"`
	LastSequence  int64                       `json:"last_sequence,omitempty"`
	Commands      []model.BrowserOperation    `json:"commands,omitempty"`
	CaptureID     string                      `json:"capture_id,omitempty"`
	Error         string                      `json:"error,omitempty"`
}
type Control struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	Profile     string `json:"profile"`
	Epoch       string `json:"epoch,omitempty"`
	TabID       int    `json:"tab_id,omitempty"`
	WindowID    int    `json:"window_id,omitempty"`
	ExpectedURL string `json:"expected_url,omitempty"`
	URL         string `json:"url,omitempty"`
}

func Decode(b []byte) (Message, error) {
	var m Message
	if len(b) > MaxFrame {
		return m, fmt.Errorf("frame too large")
	}
	if err := model.StrictJSON(b, &m); err != nil {
		return m, err
	}
	return m, m.Validate()
}
func ValidURL(s string) bool {
	return model.BrowserURL(s)
}
func (m Message) Validate() error {
	if len(m.FocusSpans) > 1 || (m.FocusSpans != nil && m.Type != "inventory") {
		return fmt.Errorf("focus spans require a bounded inventory observation")
	}
	for _, span := range m.FocusSpans {
		observed, err := time.Parse(time.RFC3339Nano, m.ObservedAt)
		if span.Validate() != nil || err != nil || !span.EndedAt.Equal(observed) || m.Complete == nil || !*m.Complete {
			return fmt.Errorf("focus span requires a matching complete observation")
		}
	}
	if (m.Delta || m.BaseSequence != 0 || len(m.Removed) > 0) && m.Type != "inventory" {
		return fmt.Errorf("delta fields on another message")
	}
	if len(m.Removed) > 2048 || (!m.Delta && (m.BaseSequence != 0 || len(m.Removed) > 0)) || (m.Delta && m.BaseSequence < 1) {
		return fmt.Errorf("invalid inventory delta")
	}
	seenRemoved := map[int]bool{}
	for _, id := range m.Removed {
		if id < 1 || seenRemoved[id] {
			return fmt.Errorf("invalid removed tab")
		}
		seenRemoved[id] = true
	}
	for _, tab := range m.Tabs {
		if seenRemoved[tab.ID] {
			return fmt.Errorf("changed and removed tab")
		}
	}
	if m.V != 1 {
		return fmt.Errorf("unsupported protocol version")
	}
	for _, id := range []string{m.ID, m.Profile, m.Epoch, m.Connection} {
		if !IDPattern.MatchString(id) {
			return fmt.Errorf("invalid protocol identity")
		}
	}
	if len(m.Label) > 80 || len(m.ExtensionVersion) > 40 {
		return fmt.Errorf("metadata too long")
	}
	if m.Type != "inventory" && m.Type != "readback" && (m.Sequence != 0 || m.ObservedAt != "" || m.Tabs != nil || m.FocusedWindow != nil || m.Complete != nil) {
		return fmt.Errorf("inventory fields on another message")
	}
	if m.Type != "hello" && (m.Label != "" || m.ExtensionVersion != "") {
		return fmt.Errorf("hello fields on another message")
	}
	if (m.ActionProtocol != 0 && m.ActionProtocol != 1) || (m.Type != "hello" && m.ActionProtocol != 0) {
		return fmt.Errorf("invalid action protocol capability")
	}
	if (m.VerificationProtocol != 0 && m.VerificationProtocol != 1) || (m.Type != "hello" && m.VerificationProtocol != 0) {
		return fmt.Errorf("invalid verification capability")
	}
	if m.Type == "readback" {
		if !IDPattern.MatchString(m.ChallengeID) || m.Stable == nil || len(m.PresentTabs) > 2048 || len(m.Instances) > 128 {
			return fmt.Errorf("invalid challenged readback")
		}
	} else if m.ChallengeID != "" || m.Stable != nil || m.PresentTabs != nil || m.Instances != nil {
		return fmt.Errorf("readback fields on another message")
	}
	if (m.PairingProtocol != 0 && m.PairingProtocol != 1) || (m.Type != "hello" && (m.PairingProtocol != 0 || m.ExtensionID != "")) || (m.PairingProtocol == 1 && (!model.BrowserExtensionIDPattern.MatchString(m.ExtensionID) || m.VerificationProtocol != 1 || m.ActionProtocol != 1)) || (m.PairingProtocol == 0 && m.ExtensionID != "") {
		return fmt.Errorf("invalid pairing capability")
	}
	if m.RecoveryProtocol != 0 && (m.RecoveryProtocol != 1 || m.Type != "hello" || m.PairingProtocol != 1) {
		return fmt.Errorf("invalid browser recovery capability")
	}
	if m.Type != "pairing_ready" && m.PairReady != nil {
		return fmt.Errorf("pairing report on another message")
	}
	if m.Type != "readback" && (m.Markers != nil || m.EventGeneration != nil) {
		return fmt.Errorf("marker fields on another message")
	}
	if len(m.Markers) > 128 || (m.EventGeneration != nil && *m.EventGeneration < 0) {
		return fmt.Errorf("invalid marker readback")
	}
	if m.Type != "command_result" && m.Result != nil {
		return fmt.Errorf("result on another message")
	}
	if (m.Type == "capture") != (m.Capture != nil) {
		return fmt.Errorf("capture fields on another message")
	}
	if m.Capture != nil && (len(m.Capture.Line) > 4096 || len(m.Capture.Pointer) < 1 || len(m.Capture.Pointer) > 8192 || len(m.Capture.Title) > 1024) {
		return fmt.Errorf("invalid capture request")
	}
	switch m.Type {
	case "hello":
		if m.ExtensionVersion == "" {
			return fmt.Errorf("extension version required")
		}
	case "pairing_ready":
		if m.PairReady == nil || m.PairReady.MarkerTabID < 1 || m.PairReady.WindowID < 1 {
			return fmt.Errorf("invalid pairing ready report")
		}
		return m.PairReady.ActionRef.Validate()
	case "poll":
	case "capture":
	case "inventory", "readback":
		if m.Sequence < 1 || m.FocusedWindow == nil || m.Complete == nil || len(m.Tabs) > 2048 {
			return fmt.Errorf("invalid inventory metadata")
		}
		if _, e := time.Parse(time.RFC3339Nano, m.ObservedAt); e != nil {
			return fmt.Errorf("invalid observation time")
		}
		seen := map[int]bool{}
		for _, t := range m.Tabs {
			if t.ID < 1 || t.WindowID < 1 || seen[t.ID] || len(t.Title) > 1024 || !ValidURL(t.URL) || t.OwnerID != "" || (t.LoadStatus != "" && t.LoadStatus != "loading" && t.LoadStatus != "complete" && t.LoadStatus != "unloaded") {
				return fmt.Errorf("invalid or duplicated tab; ownership cannot be asserted")
			}
			seen[t.ID] = true
		}
	case "command_result":
		if m.Result == nil || !IDPattern.MatchString(m.Result.OperationID) || !model.Contains([]string{"succeeded", "refused", "failed", "uncertain"}, m.Result.Status) || len(m.Result.Detail) > 512 {
			return fmt.Errorf("invalid command result")
		}
		if m.Result.ContinuationID != "" && !model.OpaqueID.MatchString(m.Result.ContinuationID) {
			return fmt.Errorf("invalid continuation reference")
		}
		if m.Result.ActionRef != nil {
			if m.Result.OperationID != m.Result.ActionRef.ID {
				return fmt.Errorf("result action identity mismatch")
			}
			return m.Result.ActionRef.Validate()
		}
	default:
		return fmt.Errorf("unsupported message type")
	}
	return nil
}
