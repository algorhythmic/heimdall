// Package conversation defines source observations without task authority or I/O.
package conversation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxDescriptionBytes = 4096
const HistoryLimit = 32
const DefaultInactivity = 30 * time.Minute

var opaqueID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func Digest(b []byte) string         { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func SourceRevision(b []byte) string { return "sha256:" + Digest(b) }
func ValidDigest(s string) bool      { return digestPattern.MatchString(s) }
func ValidRevision(s string) bool {
	return strings.HasPrefix(s, "sha256:") && ValidDigest(strings.TrimPrefix(s, "sha256:"))
}

// Identity implements the published v1 length-prefixed identity specification,
// independently of Skald. Fields remain byte-exact, including Unicode and paths.
func Identity(domain string, fields ...string) string {
	data := []byte("sessionrecord\x00v1\x00")
	for _, field := range append([]string{domain}, fields...) {
		data = binary.BigEndian.AppendUint32(data, uint32(len(field)))
		data = append(data, field...)
	}
	return "sr1:" + domain + ":" + Digest(data)
}

func token(s string, max int) bool {
	if s == "" || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return strings.TrimSpace(s) != ""
}

type ContractVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

func (v ContractVersion) Validate() error {
	if v.Major != 1 || v.Minor < 0 || v.Minor > 65535 {
		return fmt.Errorf("unsupported conversation contract version")
	}
	return nil
}

type SourceKey struct {
	AdapterID     string `json:"adapter_id"`
	ContractMajor int    `json:"contract_major"`
	ContractMinor int    `json:"contract_minor"`
	Namespace     string `json:"namespace"`
	LogicalStream string `json:"logical_stream"`
}

func (s SourceKey) Validate() error {
	if err := (ContractVersion{s.ContractMajor, s.ContractMinor}).Validate(); err != nil {
		return err
	}
	if !token(s.AdapterID, 128) || !token(s.Namespace, 1024) || !token(s.LogicalStream, 1024) {
		return fmt.Errorf("invalid opaque conversation source")
	}
	return nil
}
func (s SourceKey) Key() string {
	return Identity("heimdall-source", s.AdapterID, strconv.Itoa(s.ContractMajor), strconv.Itoa(s.ContractMinor), s.Namespace, s.LogicalStream)
}
func (s SourceKey) NativeKey(nativeID string) string {
	return Identity("conversation", s.Namespace, nativeID)
}
func (s SourceKey) ConversationKey(nativeID string) string {
	return Identity("heimdall-conversation", s.Key(), s.NativeKey(nativeID))
}

type TranscriptRef struct {
	Locator string `json:"locator"`
	Digest  string `json:"digest"`
}

func (r TranscriptRef) Validate() error {
	if !token(r.Locator, 4096) || r.Digest != Digest([]byte(r.Locator)) {
		return fmt.Errorf("invalid transcript locator reference")
	}
	return nil
}

type TaskRef struct {
	Target   string `json:"target"`
	Revision int64  `json:"revision"`
}

// Epoch is an explicit source rewrite epoch, not a connection/session identity.
// Sequence orders deliveries within it. Native source times must be known here;
// future adapters must surface a gap instead of substituting an ingestion time.
type Observation struct {
	RecordKey      string    `json:"record_key"`
	SourceRevision string    `json:"source_revision"`
	Epoch          int64     `json:"epoch"`
	Sequence       int64     `json:"sequence"`
	SourceTime     time.Time `json:"source_time"`
	ObservedAt     time.Time `json:"observed_at"`
}

func (o Observation) Validate() error {
	if !token(o.RecordKey, 1024) || !ValidRevision(o.SourceRevision) || o.Epoch < 0 || o.Sequence < 1 || o.SourceTime.IsZero() || o.ObservedAt.IsZero() || o.SourceTime.After(o.ObservedAt) {
		return fmt.Errorf("invalid conversation source observation")
	}
	return nil
}

type Started struct {
	Version              int             `json:"version"`
	ID                   string          `json:"id"`
	Kind                 string          `json:"kind"`
	Source               SourceKey       `json:"source"`
	NativeConversationID string          `json:"native_conversation_id"`
	TranscriptRef        *TranscriptRef  `json:"transcript_ref,omitempty"`
	Task                 *TaskRef        `json:"task,omitempty"`
	StartedAt            time.Time       `json:"started_at"`
	AdapterVersion       string          `json:"adapter_version"`
	ContractVersion      ContractVersion `json:"contract_version"`
	Observation
}

func (r Started) Validate() error {
	if r.Version != 1 || !opaqueID.MatchString(r.ID) || !oneOf(r.Kind, "claude_code", "codex", "claude_desktop", "browser") || !token(r.NativeConversationID, 1024) || !token(r.AdapterVersion, 128) || r.StartedAt.IsZero() || !r.StartedAt.Equal(r.SourceTime) {
		return fmt.Errorf("invalid conversation start")
	}
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if r.ContractVersion != (ContractVersion{r.Source.ContractMajor, r.Source.ContractMinor}) {
		return fmt.Errorf("conversation contract/source mismatch")
	}
	if r.Task != nil && (!token(r.Task.Target, 128) || r.Task.Revision < 1) {
		return fmt.Errorf("invalid explicit conversation task")
	}
	if r.TranscriptRef != nil {
		if err := r.TranscriptRef.Validate(); err != nil {
			return err
		}
	}
	return r.Observation.Validate()
}

type Ended struct {
	Version        int           `json:"version"`
	ConversationID string        `json:"conversation_id"`
	TranscriptRef  TranscriptRef `json:"transcript_ref"`
	Turns          int64         `json:"turns"`
	Artifacts      []string      `json:"artifacts"`
	EndedAt        time.Time     `json:"ended_at"`
	Observation
}

func (r Ended) Validate() error {
	if r.Version != 1 || !opaqueID.MatchString(r.ConversationID) || r.Turns < 0 || r.Artifacts == nil || len(r.Artifacts) > 128 || r.EndedAt.IsZero() || !r.EndedAt.Equal(r.SourceTime) {
		return fmt.Errorf("invalid conversation end")
	}
	for i, digest := range r.Artifacts {
		if !ValidDigest(digest) || (i > 0 && r.Artifacts[i-1] >= digest) {
			return fmt.Errorf("artifact digests must be unique and sorted")
		}
	}
	if err := r.TranscriptRef.Validate(); err != nil {
		return err
	}
	return r.Observation.Validate()
}

type Order struct {
	StreamGeneration string `json:"stream_generation"`
	Ordinal          int64  `json:"ordinal"`
}
type Coverage struct {
	Kind    string `json:"kind"`
	Through *Order `json:"through,omitempty"`
	Start   *Order `json:"start,omitempty"`
	End     *Order `json:"end,omitempty"`
}

func (c Coverage) Validate() error {
	valid := func(o *Order) bool { return o != nil && token(o.StreamGeneration, 1024) && o.Ordinal >= 0 }
	switch c.Kind {
	case "unknown":
		if c.Through == nil && c.Start == nil && c.End == nil {
			return nil
		}
	case "collector_cutoff":
		if valid(c.Through) && c.Start == nil && c.End == nil {
			return nil
		}
	case "native_range":
		if valid(c.Start) && valid(c.End) && c.Through == nil && c.Start.StreamGeneration == c.End.StreamGeneration && c.Start.Ordinal <= c.End.Ordinal {
			return nil
		}
	}
	return fmt.Errorf("invalid description coverage")
}

type Provenance struct {
	Kind     string `json:"kind"`
	Producer string `json:"producer"`
}

type Description struct {
	Version         int             `json:"version"`
	ConversationID  string          `json:"conversation_id"`
	Source          SourceKey       `json:"source"`
	Kind            string          `json:"kind"`
	AdapterVersion  string          `json:"adapter_version"`
	ContractVersion ContractVersion `json:"contract_version"`
	Provenance      Provenance      `json:"provenance"`
	OriginalDigest  string          `json:"original_digest"`
	RetainedDigest  string          `json:"retained_digest"`
	Truncated       bool            `json:"truncated"`
	RetainedBytes   int             `json:"retained_bytes"`
	Coverage        Coverage        `json:"coverage"`
	Availability    string          `json:"availability"`
	Observation
}

func (d Description) Validate() error {
	if d.Version != 1 || !opaqueID.MatchString(d.ConversationID) || !oneOf(d.Kind, "title", "recap", "summary", "note") {
		return fmt.Errorf("ineligible conversation description")
	}
	if !token(d.AdapterVersion, 128) || !oneOf(d.Provenance.Kind, "native", "user_annotation", "imported_external") || !token(d.Provenance.Producer, 128) || !oneOf(d.Availability, "available", "withdrawn") || !ValidDigest(d.OriginalDigest) || !ValidDigest(d.RetainedDigest) || d.RetainedBytes < 1 || d.RetainedBytes > MaxDescriptionBytes || (d.Truncated && d.RetainedBytes < MaxDescriptionBytes-3) {
		return fmt.Errorf("invalid description metadata")
	}
	if err := d.Source.Validate(); err != nil {
		return err
	}
	if d.ContractVersion != (ContractVersion{d.Source.ContractMajor, d.Source.ContractMinor}) {
		return fmt.Errorf("description contract/source mismatch")
	}
	if err := d.Coverage.Validate(); err != nil {
		return err
	}
	return d.Observation.Validate()
}
func (d Description) Association() string {
	return Identity("heimdall-description", d.ConversationID, d.Source.Key(), d.RecordKey, d.SourceRevision)
}

type DescriptionRef struct {
	Association    string `json:"association"`
	RecordKey      string `json:"record_key"`
	SourceRevision string `json:"source_revision"`
	RetainedDigest string `json:"retained_digest"`
	Availability   string `json:"availability"`
}

func (d Description) Ref() DescriptionRef {
	return DescriptionRef{d.Association(), d.RecordKey, d.SourceRevision, d.RetainedDigest, d.Availability}
}

type Record struct {
	Started
	FirstStartedAt     time.Time              `json:"first_started_at"`
	ResumeCount        int                    `json:"resume_count"`
	Ended              *Ended                 `json:"ended,omitempty"`
	LastObservation    Observation            `json:"last_observation"`
	CurrentDescription *Description           `json:"current_description,omitempty"`
	DescriptionHeads   map[string]Description `json:"description_heads"`
	DescriptionHistory []DescriptionRef       `json:"description_history"`
	// Tombstones are permanent metadata, independent of the bounded display history.
	Withdrawn map[string]bool `json:"withdrawn"`
}

func (r Record) Lifecycle(now time.Time) string {
	if r.Ended != nil {
		return "ended"
	}
	if !now.Before(r.LastObservation.SourceTime.Add(DefaultInactivity)) {
		return "inactive-by-policy"
	}
	if r.ResumeCount > 0 {
		return "resumed"
	}
	return "started"
}

// Normalize only changes line endings and trailing whitespace. Leading and
// internal whitespace, case and Unicode normalization remain byte-significant.
func Normalize(raw []byte) ([]byte, bool, error) {
	if !utf8.Valid(raw) {
		return nil, false, fmt.Errorf("description is not UTF-8")
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	normalized := []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n"))
	if len(normalized) == 0 {
		return nil, false, fmt.Errorf("empty description")
	}
	truncated := len(normalized) > MaxDescriptionBytes
	if truncated {
		end := MaxDescriptionBytes
		for !utf8.RuneStart(normalized[end]) {
			end--
		}
		normalized = normalized[:end]
	}
	return normalized, truncated, nil
}
func oneOf(value string, allowed ...string) bool {
	for _, s := range allowed {
		if value == s {
			return true
		}
	}
	return false
}
