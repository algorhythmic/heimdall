package conversation

import (
	"fmt"
	"time"
)

// Checkpoint is the durable read position of one configured native stream. It
// mirrors the shared capture contract's candidate checkpoint; only committed
// transactions may advance it, never a speculative read.
type Checkpoint struct {
	Version         int    `json:"version"`
	Generation      string `json:"generation"`
	Epoch           int64  `json:"epoch"`
	Offset          int64  `json:"offset"`
	ParsedOffset    int64  `json:"parsed_offset"`
	Ordinal         int64  `json:"ordinal"`
	PrefixDigest    string `json:"prefix_digest"`
	ConversationID  string `json:"conversation_id"`
	ProviderVersion string `json:"provider_version"`
	HasGaps         bool   `json:"has_gaps"`
}

func (c Checkpoint) Validate() error {
	if c.Version != 1 || c.Epoch < 0 || c.Offset < 0 || c.ParsedOffset < 0 || c.ParsedOffset > c.Offset || c.Ordinal < 0 {
		return fmt.Errorf("invalid source checkpoint")
	}
	if c.Offset == 0 {
		if c.Generation != "" || c.PrefixDigest != "" || c.ConversationID != "" {
			return fmt.Errorf("a zero checkpoint carries no stream state")
		}
		return nil
	}
	if !token(c.Generation, 1024) || !ValidRevision(c.PrefixDigest) || !token(c.ConversationID, 1024) {
		return fmt.Errorf("invalid committed source checkpoint")
	}
	return nil
}

// SourceRoot is an explicitly configured scan root. It authorizes the session
// observer to register and read the native streams discovered beneath it.
type SourceRoot struct {
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	Provider     string    `json:"provider"`
	Root         string    `json:"root"`
	Namespace    string    `json:"namespace"`
	Host         string    `json:"host"`
	RegisteredAt time.Time `json:"registered_at"`
	Active       bool      `json:"active"`
}

func (r SourceRoot) Validate() error {
	if r.Version != 1 || !opaqueID.MatchString(r.ID) || !oneOf(r.Provider, "claude_code", "codex") || !token(r.Root, 4096) || !token(r.Namespace, 1024) || !token(r.Host, 256) || r.RegisteredAt.IsZero() {
		return fmt.Errorf("invalid conversation source root")
	}
	return nil
}

// Source is one native conversation stream under a configured root: a logical
// stream token persisted before first capture, never a collector path
// substituted late. Path is the current locator; relocation keeps identity.
type Source struct {
	Version      int        `json:"version"`
	Key          SourceKey  `json:"key"`
	RootID       string     `json:"root_id"`
	Provider     string     `json:"provider"`
	Root         string     `json:"root"`
	Path         string     `json:"path"`
	NativeID     string     `json:"native_conversation_id"`
	Project      string     `json:"project,omitempty"`
	Originator   string     `json:"originator,omitempty"`
	Evidence     string     `json:"evidence"`
	ProviderVer  string     `json:"provider_version,omitempty"`
	RegisteredAt time.Time  `json:"registered_at"`
	Checkpoint   Checkpoint `json:"checkpoint"`
	Active       bool       `json:"active"`
	Lost         string     `json:"lost,omitempty"`
}

func (s Source) Validate() error {
	if s.Version != 1 || !opaqueID.MatchString(s.RootID) || !oneOf(s.Provider, "claude_code", "codex") || !token(s.Root, 4096) || !token(s.Path, 4096) || !token(s.NativeID, 1024) || !token(s.Evidence, 64) || s.RegisteredAt.IsZero() {
		return fmt.Errorf("invalid conversation source")
	}
	if err := s.Key.Validate(); err != nil {
		return err
	}
	return s.Checkpoint.Validate()
}

func (s Source) ID() string { return s.Key.Key() }

// Gap is one persisted coverage gap observed while reading a stream.
type Gap struct {
	Version    int       `json:"version"`
	SourceID   string    `json:"source_id"`
	Code       string    `json:"code"`
	Offset     int64     `json:"offset"`
	Ordinal    int64     `json:"ordinal"`
	Generation string    `json:"generation"`
	Detail     string    `json:"detail,omitempty"`
	At         time.Time `json:"at"`
}

func (g Gap) Validate() error {
	if g.Version != 1 || g.SourceID == "" || g.Code == "" || g.Offset < 0 || g.Ordinal < 0 || len(g.Code) > 64 || len(g.Detail) > 512 || g.At.IsZero() {
		return fmt.Errorf("invalid source gap")
	}
	return nil
}
