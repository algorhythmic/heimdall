// Package session ingests configured native conversation sources through the
// pinned shared capture contract. It records lifecycle and description
// observations plus coverage gaps, and commits checkpoints only after their
// records are journaled.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/algorhythmic/skald/sessioncapture"
	"github.com/algorhythmic/skald/sessionrecord"
)

const AdapterID = "skald/sessioncapture"
const contractMajor, contractMinor = 1, 0

type Service struct {
	Store     *store.Store
	PollEvery time.Duration
	Limits    sessioncapture.Limits
}

func (s Service) Run(ctx context.Context, clock func() time.Time) {
	interval := s.PollEvery
	if interval <= 0 {
		interval = 5 * time.Second
	}
	limits := s.Limits
	if limits == (sessioncapture.Limits{}) {
		limits = sessioncapture.DefaultLimits()
	}
	s.Limits = limits
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		s.poll(ctx, clock().UTC())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s Service) limits() sessioncapture.Limits {
	if s.Limits == (sessioncapture.Limits{}) {
		return sessioncapture.DefaultLimits()
	}
	return s.Limits
}

func (s Service) poll(ctx context.Context, now time.Time) {
	if s.Store == nil {
		return
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return
	}
	for _, root := range st.SourceRoots {
		if root.Active {
			s.pollRoot(ctx, st, root, now)
		}
	}
}

func (s Service) pollRoot(ctx context.Context, st model.State, root conversation.SourceRoot, now time.Time) {
	ioctx, stop := context.WithTimeout(ctx, 10*time.Second)
	candidates, err := sessioncapture.Inventory(ioctx, root.Root, root.Provider, time.Time{}, 256)
	stop()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, cand := range candidates {
		seen[cand.Path] = true
		src, ok := findStream(st, root, cand.Path)
		if !ok {
			src, err = s.register(ctx, st, root, cand.Path, now)
			if err != nil || !src.Active {
				continue
			}
		}
		s.ingest(ctx, st, root, src, now)
	}
	// A complete inventory authorizes loss marking; a truncated scan does not.
	if len(candidates) < 256 {
		for id, src := range st.SessionSources {
			if src.Active && src.RootID == root.ID && !seen[src.Path] {
				_, _ = s.Store.MarkSourceLost(ctx, id, "file_removed", now)
			}
		}
	}
}

func findStream(st model.State, root conversation.SourceRoot, path string) (conversation.Source, bool) {
	for _, src := range st.SessionSources {
		if src.RootID == root.ID && src.Path == path {
			return src, true
		}
	}
	return conversation.Source{}, false
}

// register identifies a native stream and persists its identity before the
// first capture. Files without a native conversation ID are not streams.
func (s Service) register(ctx context.Context, st model.State, root conversation.SourceRoot, path string, now time.Time) (conversation.Source, error) {
	ioctx, stop := context.WithTimeout(ctx, 5*time.Second)
	ident, err := sessioncapture.Identify(ioctx, root.Root, path, root.Provider)
	stop()
	if err != nil || ident.ConversationID == "" {
		return conversation.Source{}, err
	}
	// The same native conversation at a new path is a relocation, not a stream.
	for id, old := range st.SessionSources {
		if old.RootID == root.ID && old.NativeID == ident.ConversationID {
			if old.Path != path {
				relocated := old
				relocated.Path = path
				if _, err := s.Store.RegisterStream(ctx, relocated, now); err == nil {
					old.Path = path
					st.SessionSources[id] = old
				}
			}
			return old, nil
		}
	}
	src := conversation.Source{Version: 1, RootID: root.ID, Provider: root.Provider, Root: root.Root, Path: path,
		NativeID: ident.ConversationID, Project: ident.Project, Originator: ident.Originator,
		Evidence: ident.Evidence, ProviderVer: ident.ProviderVersion, RegisteredAt: now,
		Key: conversation.SourceKey{AdapterID: AdapterID, ContractMajor: contractMajor, ContractMinor: contractMinor,
			Namespace: root.Namespace, LogicalStream: sessionrecord.Key("stream", root.Namespace, root.Provider, model.NewID())}}
	if _, err := s.Store.RegisterStream(ctx, src, now); err != nil {
		return conversation.Source{}, err
	}
	src.Active = true
	src.Checkpoint = conversation.Checkpoint{Version: 1}
	st.SessionSources[src.ID()] = src
	return src, nil
}

func toCaptureCheckpoint(cp conversation.Checkpoint, key string) sessioncapture.Checkpoint {
	if cp.Offset == 0 {
		// The contract's initial checkpoint is the all-zero value, not v1 fields.
		return sessioncapture.Checkpoint{}
	}
	return sessioncapture.Checkpoint{Version: cp.Version, SourceKey: key, Generation: cp.Generation, Epoch: cp.Epoch,
		Offset: cp.Offset, ParsedOffset: cp.ParsedOffset, Ordinal: cp.Ordinal, PrefixDigest: cp.PrefixDigest,
		ConversationID: cp.ConversationID, ProviderVersion: cp.ProviderVersion, HasGaps: cp.HasGaps}
}

func fromCaptureCheckpoint(cp sessioncapture.Checkpoint) conversation.Checkpoint {
	return conversation.Checkpoint{Version: cp.Version, Generation: cp.Generation, Epoch: cp.Epoch,
		Offset: cp.Offset, ParsedOffset: cp.ParsedOffset, Ordinal: cp.Ordinal, PrefixDigest: cp.PrefixDigest,
		ConversationID: cp.ConversationID, ProviderVersion: cp.ProviderVersion, HasGaps: cp.HasGaps}
}

func ingestObservation(rec sessionrecord.Record, epoch int64, now time.Time) (conversation.Observation, error) {
	if rec.SourceTime == nil || rec.SourceOrder == nil {
		return conversation.Observation{}, fmt.Errorf("record without native time/order")
	}
	return conversation.Observation{RecordKey: rec.RecordKey, SourceRevision: rec.SourceRevision,
		Epoch: epoch, Sequence: rec.SourceOrder.Ordinal + 1, SourceTime: rec.SourceTime.UTC(), ObservedAt: now.UTC()}, nil
}

// taskBinding maps a record's native cwd onto the longest bound resource root.
func taskBinding(st model.State, rec sessionrecord.Record) *conversation.TaskRef {
	raw, ok := rec.Extensions["native_cwd"]
	if !ok {
		return nil
	}
	var cwd string
	if json.Unmarshal(raw, &cwd) != nil || cwd == "" {
		return nil
	}
	match, depth := "", -1
	for _, res := range st.Resources {
		if !res.Active || res.Kind != "tree" || res.Root == "" {
			continue
		}
		if cwd == res.Root || strings.HasPrefix(cwd, res.Root+"/") {
			if len(res.Root) > depth {
				match, depth = res.Target, len(res.Root)
			}
		}
	}
	if match == "" {
		return nil
	}
	return &conversation.TaskRef{Target: match, Revision: st.Tasks[match].Revision}
}

// ingest reads one stream from its committed checkpoint and journals derived
// observations before advancing the checkpoint. Reprocessing after an
// interruption re-derives identical command IDs and dedupes cleanly.
func (s Service) ingest(ctx context.Context, st model.State, root conversation.SourceRoot, src conversation.Source, now time.Time) {
	f, err := os.Open(filepath.Join(src.Root, filepath.Clean(src.Path)))
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = s.Store.MarkSourceLost(ctx, src.ID(), "file_removed", now)
		}
		return
	}
	defer f.Close()
	scSource := sessioncapture.Source{Namespace: root.Namespace, Provider: root.Provider,
		StreamID: src.Key.LogicalStream, ConversationID: src.NativeID, ProviderVersion: src.ProviderVer}
	batch, err := sessioncapture.Read(f, scSource, toCaptureCheckpoint(src.Checkpoint, scSource.Key()), s.limits(), now)
	if err != nil {
		return
	}
	epoch := batch.Checkpoint.Epoch
	changed := batch.Checkpoint.Ordinal != src.Checkpoint.Ordinal || batch.Checkpoint.Offset != src.Checkpoint.Offset
	for _, captured := range batch.Records {
		rec := captured.Record
		if !conversationStarted(st, src, rec) {
			raw, err := s.startConversation(ctx, st, src, rec, epoch, now)
			if err != nil {
				continue
			}
			var receipt store.ConversationReceipt
			if json.Unmarshal(raw, &receipt) == nil {
				// Marker so the rest of this batch resolves the conversation
				// without a state re-fetch.
				marker := conversation.Record{}
				marker.Source, marker.NativeConversationID = src.Key, src.NativeID
				st.Conversations[receipt.ConversationID] = marker
			}
		}
		s.ingestRecord(ctx, st, src, rec, captured.Raw, epoch, now)
	}
	for _, g := range batch.Gaps {
		_, _ = s.Store.RecordGap(ctx, src.ID(), conversation.Gap{Version: 1, Code: g.Code, Offset: g.Offset, Ordinal: g.Ordinal, Generation: g.Generation}, now)
	}
	if changed {
		_, _ = s.Store.AdvanceCheckpoint(ctx, src.ID(), fromCaptureCheckpoint(batch.Checkpoint), now)
	}
}

func conversationStarted(st model.State, src conversation.Source, rec sessionrecord.Record) bool {
	for _, c := range st.Conversations {
		if c.Source == src.Key && c.NativeConversationID == src.NativeID {
			return true
		}
	}
	return false
}

func (s Service) startConversation(ctx context.Context, st model.State, src conversation.Source, rec sessionrecord.Record, epoch int64, now time.Time) (json.RawMessage, error) {
	o, err := ingestObservation(rec, epoch, now)
	if err != nil {
		return nil, err
	}
	started := conversation.Started{Version: 1, Kind: src.Provider, Source: src.Key, NativeConversationID: src.NativeID,
		StartedAt: o.SourceTime, AdapterVersion: rec.AdapterVersion,
		ContractVersion: conversation.ContractVersion{Major: contractMajor, Minor: contractMinor},
		Observation:     o, Task: taskBinding(st, rec),
		TranscriptRef: &conversation.TranscriptRef{Locator: filepath.Join(src.Root, src.Path), Digest: conversation.Digest([]byte(filepath.Join(src.Root, src.Path)))}}
	return s.Store.RecordConversationStarted(ctx, started)
}

// ingestRecord maps one normalized record to conversation events. Content
// kinds Heimdall does not retain advance the checkpoint without an event.
func (s Service) ingestRecord(ctx context.Context, st model.State, src conversation.Source, rec sessionrecord.Record, raw []byte, epoch int64, now time.Time) {
	var kind string
	switch rec.Kind {
	case "title":
		kind = "title"
	case "native_recap":
		kind = "recap"
	default:
		return
	}
	if rec.Body.Text == "" {
		return
	}
	o, err := ingestObservation(rec, epoch, now)
	if err != nil {
		return
	}
	convID := ""
	for id, c := range st.Conversations {
		if c.Source == src.Key && c.NativeConversationID == src.NativeID {
			convID = id
			break
		}
	}
	if convID == "" {
		return
	}
	coverage := conversation.Coverage{Kind: "unknown"}
	if rec.SourceOrder != nil {
		order := conversation.Order{StreamGeneration: rec.SourceOrder.StreamGeneration, Ordinal: rec.SourceOrder.Ordinal}
		coverage = conversation.Coverage{Kind: "native_range", Start: &order, End: &order}
	}
	d := conversation.Description{Version: 1, ConversationID: convID, Source: src.Key, Kind: kind,
		AdapterVersion: rec.AdapterVersion, ContractVersion: conversation.ContractVersion{Major: contractMajor, Minor: contractMinor},
		Provenance: conversation.Provenance{Kind: "native", Producer: src.Provider}, Availability: "available",
		Coverage: coverage, Observation: o}
	_, _ = s.Store.RecordDescription(ctx, d, []byte(rec.Body.Text))
}

// SourcesView lists configured roots and streams for CLI inspection.
func SourcesView(st model.State) ([]conversation.SourceRoot, []conversation.Source) {
	roots := []conversation.SourceRoot{}
	for _, r := range st.SourceRoots {
		roots = append(roots, r)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	sources := []conversation.Source{}
	for _, src := range st.SessionSources {
		sources = append(sources, src)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID() < sources[j].ID() })
	return roots, sources
}
