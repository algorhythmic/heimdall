package model

import "time"

type BrowserTab struct {
	ID       int    `json:"id"`
	WindowID int    `json:"window_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Active   bool   `json:"active"`
	OwnerID  string `json:"owner_id,omitempty"`
}
type BrowserProfile struct {
	ID               string       `json:"id"`
	Label            string       `json:"label"`
	ExtensionVersion string       `json:"extension_version"`
	Epoch            string       `json:"epoch"`
	Connection       string       `json:"connection"`
	Paired           bool         `json:"paired"`
	LastSequence     int64        `json:"last_sequence"`
	LastObservedAt   time.Time    `json:"last_observed_at"`
	Tabs             []BrowserTab `json:"tabs"`
	FocusedWindow    int          `json:"focused_window"`
	Complete         bool         `json:"complete"`
}
type BrowserOperation struct {
	ID          string    `json:"id"`
	Profile     string    `json:"profile"`
	Epoch       string    `json:"epoch"`
	Action      string    `json:"action"`
	TabID       int       `json:"tab_id,omitempty"`
	WindowID    int       `json:"window_id,omitempty"`
	ExpectedURL string    `json:"expected_url,omitempty"`
	URL         string    `json:"url,omitempty"`
	OwnerID     string    `json:"owner_id,omitempty"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *State) Normalize() {
	if s.Grants == nil {
		s.Grants = map[string]Grant{}
	}
	if s.Contracts == nil {
		s.Contracts = map[string]Contract{}
	}
	if s.ContractHeads == nil {
		s.ContractHeads = map[string]string{}
	}
	if s.Decisions == nil {
		s.Decisions = map[string]Decision{}
	}
	if s.Resources == nil {
		s.Resources = map[string]Resource{}
	}
	if s.Checkpoints == nil {
		s.Checkpoints = map[string]Checkpoint{}
	}
	if s.CheckpointHeads == nil {
		s.CheckpointHeads = map[string]string{}
	}
	if s.Tasks == nil {
		s.Tasks = map[string]TaskRecord{}
	}
	if s.Captures == nil {
		s.Captures = map[string]Capture{}
	}
	if s.Proposals == nil {
		s.Proposals = map[string]Proposal{}
	}
	if s.Timers == nil {
		s.Timers = map[string]Timer{}
	}
	if s.Browsers == nil {
		s.Browsers = map[string]BrowserProfile{}
	}
	if s.BrowserOperations == nil {
		s.BrowserOperations = map[string]BrowserOperation{}
	}
}
