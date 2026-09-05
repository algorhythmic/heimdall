// Package browser defines a bounded, observation-only browser ingress and explicit
// user-authorized browser operations. It does not implement conversation scraping.
package browser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"io"
	"net/url"
	"regexp"
	"time"
)

const MaxFrame = 256 << 10
const HostName = "dev.heimdall.browser"

var IDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var ExtensionPattern = regexp.MustCompile(`^[a-p]{32}$`)

type Message struct {
	V                int                `json:"v"`
	Type             string             `json:"type"`
	ID               string             `json:"id"`
	Profile          string             `json:"profile"`
	Epoch            string             `json:"epoch"`
	Connection       string             `json:"connection"`
	Label            string             `json:"label,omitempty"`
	ExtensionVersion string             `json:"extension_version,omitempty"`
	Sequence         int64              `json:"seq,omitempty"`
	ObservedAt       string             `json:"observed_at,omitempty"`
	Tabs             []model.BrowserTab `json:"tabs,omitempty"`
	FocusedWindow    *int               `json:"focused_window,omitempty"`
	Complete         *bool              `json:"complete,omitempty"`
	Result           *OperationResult   `json:"result,omitempty"`
}
type OperationResult struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	TabID       int    `json:"tab_id,omitempty"`
	WindowID    int    `json:"window_id,omitempty"`
	URL         string `json:"url,omitempty"`
	Detail      string `json:"detail,omitempty"`
}
type Reply struct {
	V            int                      `json:"v"`
	Type         string                   `json:"type"`
	ID           string                   `json:"id"`
	Profile      string                   `json:"profile,omitempty"`
	Paired       bool                     `json:"paired"`
	LastSequence int64                    `json:"last_sequence,omitempty"`
	Commands     []model.BrowserOperation `json:"commands,omitempty"`
	Error        string                   `json:"error,omitempty"`
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
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return m, fmt.Errorf("expected one JSON message")
	}
	return m, m.Validate()
}
func ValidURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && len(s) <= 8192
}
func (m Message) Validate() error {
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
	if m.Type != "inventory" && (m.Sequence != 0 || m.ObservedAt != "" || m.Tabs != nil || m.FocusedWindow != nil || m.Complete != nil) {
		return fmt.Errorf("inventory fields on another message")
	}
	if m.Type != "hello" && (m.Label != "" || m.ExtensionVersion != "") {
		return fmt.Errorf("hello fields on another message")
	}
	if m.Type != "command_result" && m.Result != nil {
		return fmt.Errorf("result on another message")
	}
	switch m.Type {
	case "hello":
		if m.ExtensionVersion == "" {
			return fmt.Errorf("extension version required")
		}
	case "poll":
	case "inventory":
		if m.Sequence < 1 || m.FocusedWindow == nil || m.Complete == nil || len(m.Tabs) > 2048 {
			return fmt.Errorf("invalid inventory metadata")
		}
		if _, e := time.Parse(time.RFC3339Nano, m.ObservedAt); e != nil {
			return fmt.Errorf("invalid observation time")
		}
		seen := map[int]bool{}
		for _, t := range m.Tabs {
			if t.ID < 1 || t.WindowID < 1 || seen[t.ID] || len(t.Title) > 1024 || !ValidURL(t.URL) || t.OwnerID != "" {
				return fmt.Errorf("invalid or duplicated tab; ownership cannot be asserted")
			}
			seen[t.ID] = true
		}
	case "command_result":
		if m.Result == nil || !IDPattern.MatchString(m.Result.OperationID) || !model.Contains([]string{"succeeded", "refused", "failed", "uncertain"}, m.Result.Status) || len(m.Result.Detail) > 512 {
			return fmt.Errorf("invalid command result")
		}
	default:
		return fmt.Errorf("unsupported message type")
	}
	return nil
}
