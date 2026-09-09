package model

import "time"

type BrowserTab struct {
	NavigationPending bool   `json:"navigation_pending,omitempty"`
	LoadStatus        string `json:"load_status,omitempty"`
	Discarded         bool   `json:"discarded,omitempty"`
	ID                int    `json:"id"`
	WindowID          int    `json:"window_id"`
	URL               string `json:"url"`
	Title             string `json:"title"`
	Active            bool   `json:"active"`
	OwnerID           string `json:"owner_id,omitempty"`
}
type BrowserProfile struct {
	VerificationProtocol int               `json:"verification_protocol,omitempty"`
	Challenge            *BrowserChallenge `json:"challenge,omitempty"`
	Freshness            *BrowserFreshness `json:"freshness,omitempty"`
	PresentTabs          []int             `json:"present_tabs,omitempty"`
	ActionProtocol       int               `json:"action_protocol,omitempty"`
	ReceivedAt           time.Time         `json:"received_at"`
	ReceivedEpoch        string            `json:"received_epoch,omitempty"`
	ID                   string            `json:"id"`
	Label                string            `json:"label"`
	ExtensionVersion     string            `json:"extension_version"`
	Epoch                string            `json:"epoch"`
	Connection           string            `json:"connection"`
	Paired               bool              `json:"paired"`
	LastSequence         int64             `json:"last_sequence"`
	LastObservedAt       time.Time         `json:"last_observed_at"`
	Tabs                 []BrowserTab      `json:"tabs"`
	FocusedWindow        int               `json:"focused_window"`
	Complete             bool              `json:"complete"`
}
type BrowserOperation struct {
	ActionRef   *BrowserActionRef `json:"action_ref,omitempty"`
	ID          string            `json:"id"`
	Profile     string            `json:"profile"`
	Epoch       string            `json:"epoch"`
	Action      string            `json:"action"`
	TabID       int               `json:"tab_id,omitempty"`
	WindowID    int               `json:"window_id,omitempty"`
	ExpectedURL string            `json:"expected_url,omitempty"`
	URL         string            `json:"url,omitempty"`
	OwnerID     string            `json:"owner_id,omitempty"`
	Status      string            `json:"status"`
	Detail      string            `json:"detail,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
}

func (s *State) Normalize() {
	if s.Actions == nil {
		s.Actions = map[string]ActionRecord{}
	}
	if s.SnapshotHeads == nil {
		s.SnapshotHeads = map[string]WorkspacePoint{}
	}
	if s.SnapshotPolicies == nil {
		s.SnapshotPolicies = map[string]SnapshotPolicy{}
	}
	if s.SnapshotPins == nil {
		s.SnapshotPins = map[string]SnapshotPin{}
	}
	if s.DesktopSources == nil {
		s.DesktopSources = map[string]DesktopSource{}
	}
	if s.ViewportBindings == nil {
		s.ViewportBindings = map[string]ViewportBinding{}
	}
	if s.ViewportHeads == nil {
		s.ViewportHeads = map[string]string{}
	}
	if s.Dependencies == nil {
		s.Dependencies = map[string]TaskDependency{}
	}
	if s.DependencyHeads == nil {
		s.DependencyHeads = map[string]string{}
	}
	if s.PreservationPlans == nil {
		s.PreservationPlans = map[string]PreservationPlan{}
	}
	if s.PreservationReceipts == nil {
		s.PreservationReceipts = map[string]PreservationReceipt{}
	}
	if s.PreservationHeads == nil {
		s.PreservationHeads = map[string]string{}
	}
	if s.ProgressProposals == nil {
		s.ProgressProposals = map[string]ProgressProposal{}
	}
	if s.ProgressReviews == nil {
		s.ProgressReviews = map[string]ProgressReview{}
	}
	if s.ProgressReviewHeads == nil {
		s.ProgressReviewHeads = map[string]string{}
	}
	if s.ArtifactProgressHeads == nil {
		s.ArtifactProgressHeads = map[string]string{}
	}
	if s.Artifacts == nil {
		s.Artifacts = map[string]Artifact{}
	}
	if s.ArtifactVersions == nil {
		s.ArtifactVersions = map[string]ArtifactVersion{}
	}
	if s.ArtifactHeads == nil {
		s.ArtifactHeads = map[string]string{}
	}
	if s.WorkspaceManifests == nil {
		s.WorkspaceManifests = map[string]WorkspaceManifest{}
	}
	if s.WorkspaceHeads == nil {
		s.WorkspaceHeads = map[string]string{}
	}
	if s.WorkspaceSurfaces == nil {
		s.WorkspaceSurfaces = map[string]SurfaceIdentity{}
	}
	if s.SessionBindings == nil {
		s.SessionBindings = map[string]SessionBinding{}
	}
	if s.SessionHeads == nil {
		s.SessionHeads = map[string]string{}
	}
	if s.Evaluators == nil {
		s.Evaluators = map[string]Evaluator{}
	}
	if s.EvaluatorHeads == nil {
		s.EvaluatorHeads = map[string]string{}
	}
	if s.Evidence == nil {
		s.Evidence = map[string]Evidence{}
	}
	if s.EvidenceInvalidations == nil {
		s.EvidenceInvalidations = map[string]EvidenceInvalidation{}
	}
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
