package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/algorhythmic/skald/sessioncapture"
)

// HookPayload is the provider hook envelope (Claude Code SessionStart/SessionEnd).
// Source time comes from the payload timestamp or the transcript's own records;
// hook delivery never fabricates a source time.
type HookPayload struct {
	Event          string `json:"hook_event_name"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	Timestamp      int64  `json:"timestamp"` // unix milliseconds when present
}

func (h HookPayload) Validate() error {
	if h.SessionID == "" || len(h.SessionID) > 256 || len(h.TranscriptPath) > 4096 || len(h.CWD) > 4096 {
		return fmt.Errorf("invalid hook payload")
	}
	if h.Event != "SessionStart" && h.Event != "SessionEnd" {
		return fmt.Errorf("unsupported hook event")
	}
	return nil
}

func (h HookPayload) stamp() (time.Time, bool) {
	if h.Timestamp <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(h.Timestamp).UTC(), true
}

// resolveStream finds the registered stream for a native session, or registers
// it under a configured root covering the transcript path.
func (s *Service) resolveStream(ctx context.Context, st model.State, p HookPayload, now time.Time) (conversation.Source, conversation.SourceRoot, error) {
	for _, src := range st.SessionSources {
		if src.NativeID == p.SessionID {
			root, ok := st.SourceRoots[src.RootID]
			if !ok {
				return conversation.Source{}, conversation.SourceRoot{}, fmt.Errorf("stream root missing")
			}
			return src, root, nil
		}
	}
	if p.TranscriptPath == "" {
		return conversation.Source{}, conversation.SourceRoot{}, fmt.Errorf("unknown session source")
	}
	abs, err := filepath.Abs(p.TranscriptPath)
	if err != nil {
		return conversation.Source{}, conversation.SourceRoot{}, err
	}
	for _, root := range st.SourceRoots {
		if !root.Active || root.Provider != sessioncapture.Claude {
			continue
		}
		rel, err := filepath.Rel(root.Root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			continue
		}
		src, err := s.register(ctx, st, root, rel, now)
		if err != nil {
			continue
		}
		return src, root, nil
	}
	return conversation.Source{}, conversation.SourceRoot{}, fmt.Errorf("transcript outside configured roots")
}

// recordTimes returns the first and last complete record timestamps plus the
// last ordinal and user-turn count, without retaining any record bodies.
func recordTimes(path string) (first, last time.Time, ordinal, turns int64, err error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return time.Time{}, time.Time{}, 0, 0, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	for i := 0; i < 200000; i++ {
		line, rerr := r.ReadBytes('\n')
		if len(line) > 0 {
			var n struct {
				Type      string `json:"type"`
				Timestamp string `json:"timestamp"`
				Message   *struct {
					Role string `json:"role"`
				} `json:"message"`
			}
			if json.Unmarshal(line, &n) == nil {
				ordinal++
				if n.Type == "user" && n.Message != nil && n.Message.Role == "user" {
					turns++
				}
				if n.Timestamp != "" {
					if t, terr := time.Parse(time.RFC3339Nano, n.Timestamp); terr == nil {
						if first.IsZero() {
							first = t.UTC()
						}
						last = t.UTC()
					}
				}
			}
		}
		if rerr == io.EOF || rerr != nil {
			break
		}
	}
	return first, last, ordinal - 1, turns, nil
}

// HandleHook applies a provider hook payload to conversation lifecycle.
// Observations reuse the stream's own ordinal band so hook events and
// transcript records order deterministically under the same epoch.
func (s *Service) HandleHook(ctx context.Context, p HookPayload, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	src, _, err := s.resolveStream(ctx, st, p, now)
	if err != nil {
		return err
	}
	full := filepath.Join(src.Root, src.Path)
	if p.TranscriptPath != "" {
		if abs, err := filepath.Abs(p.TranscriptPath); err == nil {
			full = abs
		}
	}
	first, last, lastOrdinal, turns, terr := recordTimes(full)
	if terr != nil {
		return fmt.Errorf("transcript unavailable: %w", terr)
	}
	stamp, ok := p.stamp()
	if !ok {
		stamp = last
		if p.Event == "SessionStart" {
			stamp = first
		}
	}
	if stamp.IsZero() {
		return fmt.Errorf("hook has no native source time")
	}
	switch p.Event {
	case "SessionStart":
		seq := int64(1)
		epoch := src.Checkpoint.Epoch
		if c, ok := findConversation(st, src); ok {
			if c.LastObservation.Epoch == epoch && c.LastObservation.Sequence >= seq {
				seq = c.LastObservation.Sequence + 1
			}
			if c.LastObservation.Epoch > epoch {
				epoch = c.LastObservation.Epoch
			}
		}
		o := conversation.Observation{RecordKey: "hook:session_start:" + p.SessionID,
			SourceRevision: revisionOf(p, full), Epoch: epoch, Sequence: seq,
			SourceTime: stamp, ObservedAt: now.UTC()}
		started := conversation.Started{Version: 1, Kind: src.Provider, Source: src.Key, NativeConversationID: src.NativeID,
			StartedAt: stamp, AdapterVersion: "hook-v1", ContractVersion: conversation.ContractVersion{Major: contractMajor, Minor: contractMinor},
			Observation: o, TranscriptRef: &conversation.TranscriptRef{Locator: full, Digest: conversation.Digest([]byte(full))}}
		_, err := s.Store.RecordConversationStarted(ctx, started)
		return err
	case "SessionEnd":
		c, ok := findConversation(st, src)
		if !ok {
			return fmt.Errorf("end hook without a known conversation")
		}
		epoch, seq := src.Checkpoint.Epoch, lastOrdinal+1
		if c.LastObservation.Epoch > epoch {
			epoch = c.LastObservation.Epoch
		}
		if c.LastObservation.Epoch == epoch && c.LastObservation.Sequence >= seq {
			seq = c.LastObservation.Sequence + 1
		}
		o := conversation.Observation{RecordKey: "hook:session_end:" + p.SessionID,
			SourceRevision: revisionOf(p, full), Epoch: epoch, Sequence: seq,
			SourceTime: stamp, ObservedAt: now.UTC()}
		ended := conversation.Ended{Version: 1, ConversationID: c.ID, Turns: turns, Artifacts: []string{},
			EndedAt: stamp, Observation: o,
			TranscriptRef: conversation.TranscriptRef{Locator: full, Digest: conversation.Digest([]byte(full))}}
		_, err := s.Store.RecordConversationEnded(ctx, ended)
		return err
	}
	return nil
}

func findConversation(st model.State, src conversation.Source) (conversation.Record, bool) {
	for _, c := range st.Conversations {
		if c.Source == src.Key && c.NativeConversationID == src.NativeID {
			return c, true
		}
	}
	return conversation.Record{}, false
}

// revisionOf binds a hook observation to its payload bytes and locator.
func revisionOf(p HookPayload, locator string) string {
	b, _ := json.Marshal(p)
	return conversation.SourceRevision(append(b, []byte("\x00"+locator)...))
}
