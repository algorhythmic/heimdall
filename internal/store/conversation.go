package store

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"reflect"
)

const conversationActor = "observer:conversation"

type ConversationReceipt struct {
	ConversationID string `json:"conversation_id"`
	DescriptionID  string `json:"description_id,omitempty"`
}

func conversationCommand(source conversation.SourceKey, native, verb, adapter, availability string, o conversation.Observation) string {
	return "conversation-" + conversation.Digest([]byte(conversation.Identity("heimdall-delivery", source.ConversationKey(native), verb, adapter, availability, o.RecordKey, o.SourceRevision)))
}
func checkConversationEnvelope(e Event, source conversation.SourceKey, native, adapter, availability string, o conversation.Observation) error {
	if e.Actor != conversationActor || e.CommandID != conversationCommand(source, native, e.Verb, adapter, availability, o) || !e.TS.Equal(o.ObservedAt) {
		return fmt.Errorf("invalid conversation event provenance")
	}
	return nil
}
func advanceConversation(old conversation.Observation, next conversation.Observation) error {
	if next.Epoch < old.Epoch || (next.Epoch == old.Epoch && next.Sequence <= old.Sequence) || next.ObservedAt.Before(old.ObservedAt) || next.SourceTime.Before(old.SourceTime) {
		return fmt.Errorf("nonmonotonic conversation source observation")
	}
	return nil
}
func findConversation(st model.State, source conversation.SourceKey, native string) (conversation.Record, bool) {
	for _, r := range st.Conversations {
		if r.Source == source && r.NativeConversationID == native {
			return r, true
		}
	}
	return conversation.Record{}, false
}

func applyConversation(st *model.State, e Event) error {
	switch e.Verb {
	case "started":
		var p conversation.Started
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		if e.EntityID != p.ID {
			return fmt.Errorf("conversation entity mismatch")
		}
		if err := checkConversationEnvelope(e, p.Source, p.NativeConversationID, p.AdapterVersion, "", p.Observation); err != nil {
			return err
		}
		if p.Task != nil {
			task, _, err := model.ResolveTarget(*st, p.Task.Target)
			if err != nil {
				return err
			}
			if task.Revision != p.Task.Revision {
				return fmt.Errorf("stale explicit conversation task revision")
			}
		}
		old, exists := findConversation(*st, p.Source, p.NativeConversationID)
		if exists {
			if old.ID != p.ID || old.Kind != p.Kind {
				return fmt.Errorf("conversation resume identity mismatch")
			}
			if err := advanceConversation(old.LastObservation, p.Observation); err != nil {
				return err
			}
			if p.Task == nil {
				p.Task = old.Task
			}
			if p.TranscriptRef == nil {
				p.TranscriptRef = old.TranscriptRef
			}
			old.Started = p
			old.ResumeCount++
			old.Ended = nil
			old.LastObservation = p.Observation
			st.Conversations[p.ID] = old
		} else {
			if _, ok := st.Conversations[p.ID]; ok {
				return fmt.Errorf("conversation ID collision")
			}
			st.Conversations[p.ID] = conversation.Record{Started: p, FirstStartedAt: p.StartedAt, LastObservation: p.Observation, DescriptionHeads: map[string]conversation.Description{}, DescriptionHistory: []conversation.DescriptionRef{}, Withdrawn: map[string]bool{}}
		}
	case "ended":
		var p conversation.Ended
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		old, ok := st.Conversations[p.ConversationID]
		if !ok || e.EntityID != p.ConversationID || old.Ended != nil || p.EndedAt.Before(old.StartedAt) {
			return fmt.Errorf("invalid conversation end reference")
		}
		if err := checkConversationEnvelope(e, old.Source, old.NativeConversationID, "", "", p.Observation); err != nil {
			return err
		}
		if err := advanceConversation(old.LastObservation, p.Observation); err != nil {
			return err
		}
		old.Ended = &p
		old.TranscriptRef = &p.TranscriptRef
		old.LastObservation = p.Observation
		st.Conversations[p.ConversationID] = old
	case "description_observed":
		var p conversation.Description
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		old, ok := st.Conversations[p.ConversationID]
		if !ok || e.EntityID != p.ConversationID || old.Source != p.Source {
			return fmt.Errorf("description source/conversation scope mismatch")
		}
		if err := checkConversationEnvelope(e, p.Source, old.NativeConversationID, p.AdapterVersion, p.Availability, p.Observation); err != nil {
			return err
		}
		if err := advanceConversation(old.LastObservation, p.Observation); err != nil {
			return err
		}
		if old.Withdrawn[p.Association()] {
			return fmt.Errorf("description association withdrawn")
		}
		head, known := old.DescriptionHeads[p.RecordKey]
		if p.Availability == "withdrawn" {
			if !known || head.Association() != p.Association() {
				return fmt.Errorf("withdrawal requires current source record revision")
			}
			// A withdrawal carries the original immutable content metadata and a new
			// delivery stamp. It cannot replace the bytes/digests it is withdrawing.
			expected := head
			expected.Availability = p.Availability
			expected.Observation = p.Observation
			if !reflect.DeepEqual(expected, p) {
				return fmt.Errorf("withdrawal metadata mismatch")
			}
		} else if known && head.Association() == p.Association() {
			return fmt.Errorf("description revision already observed")
		}
		// Copy only after all validation, so a rejected reducer leaves state intact.
		old = model.Clone(old)
		if p.Availability == "withdrawn" {
			old.Withdrawn[p.Association()] = true
		}
		old.DescriptionHeads[p.RecordKey] = p
		old.DescriptionHistory = append(old.DescriptionHistory, p.Ref())
		if len(old.DescriptionHistory) > conversation.HistoryLimit {
			old.DescriptionHistory = old.DescriptionHistory[len(old.DescriptionHistory)-conversation.HistoryLimit:]
		}
		if p.Availability == "available" || old.CurrentDescription == nil || old.CurrentDescription.Association() == p.Association() {
			old.CurrentDescription = &p
		}
		old.LastObservation = p.Observation
		st.Conversations[p.ConversationID] = old
	default:
		return fmt.Errorf("unsupported conversation verb")
	}
	return nil
}

// deliveryRequest excludes delivery counters/clocks from command dedupe. Native
// revision, source time and all semantic metadata remain part of the request.
func deliveryRequest(value any) []byte {
	b, _ := json.Marshal(value)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(b, &fields)
	for _, key := range []string{"id", "epoch", "sequence", "observed_at"} {
		delete(fields, key)
	}
	b, _ = json.Marshal(fields)
	return b
}

// RecordConversationStarted accepts an explicit binding or none. ID allocation
// and resume lookup occur under the same transaction as the command receipt.
func (s *Store) RecordConversationStarted(ctx context.Context, p conversation.Started) (json.RawMessage, error) {
	if p.ID != "" {
		return nil, fmt.Errorf("conversation IDs are allocated by Heimdall")
	}
	probe := p
	probe.ID = "00000000000000000000000000000000"
	if err := probe.Validate(); err != nil {
		return nil, err
	}
	id := conversationCommand(p.Source, p.NativeConversationID, "started", p.AdapterVersion, "", p.Observation)
	return s.Transact(ctx, id, conversationActor, deliveryRequest(p), p.ObservedAt, func(st model.State) (Change, error) {
		if old, ok := findConversation(st, p.Source, p.NativeConversationID); ok {
			p.ID = old.ID
		} else {
			p.ID = model.NewID()
		}
		return Change{Revision: st.Revision, Events: []Pending{{"conversation", "started", p.ID, p}}, Result: ConversationReceipt{ConversationID: p.ID}}, nil
	})
}
func (s *Store) RecordConversationEnded(ctx context.Context, p conversation.Ended) (json.RawMessage, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	st, err := s.State(ctx)
	if err != nil {
		return nil, err
	}
	c, ok := st.Conversations[p.ConversationID]
	if !ok {
		return nil, fmt.Errorf("unknown conversation")
	}
	id := conversationCommand(c.Source, c.NativeConversationID, "ended", "", "", p.Observation)
	return s.Transact(ctx, id, conversationActor, deliveryRequest(p), p.ObservedAt, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"conversation", "ended", p.ConversationID, p}}, Result: ConversationReceipt{ConversationID: p.ConversationID}}, nil
	})
}

// RecordDescription receives the exact native description bytes, not a prompt or
// transcript. SourceRevision independently identifies the enclosing native record.
// Digests/count/truncation are computed here; caller-supplied values must agree.
func (s *Store) RecordDescription(ctx context.Context, p conversation.Description, text []byte) (json.RawMessage, error) {
	if p.Availability != "available" {
		return nil, fmt.Errorf("use WithdrawDescription for withdrawal")
	}
	retained, truncated, err := conversation.Normalize(text)
	if err != nil {
		return nil, err
	}
	originalDigest, retainedDigest := conversation.Digest(text), conversation.Digest(retained)
	if (p.OriginalDigest != "" && p.OriginalDigest != originalDigest) || (p.RetainedDigest != "" && p.RetainedDigest != retainedDigest) || (p.RetainedBytes != 0 && p.RetainedBytes != len(retained)) || (p.Truncated && !truncated) {
		return nil, fmt.Errorf("description digest/retention mismatch")
	}
	p.OriginalDigest, p.RetainedDigest, p.RetainedBytes, p.Truncated = originalDigest, retainedDigest, len(retained), truncated
	return s.recordDescription(ctx, p, retained)
}
func (s *Store) WithdrawDescription(ctx context.Context, p conversation.Description) (json.RawMessage, error) {
	if p.Availability != "withdrawn" {
		return nil, fmt.Errorf("withdrawn availability required")
	}
	return s.recordDescription(ctx, p, nil)
}
func (s *Store) recordDescription(ctx context.Context, p conversation.Description, retained []byte) (json.RawMessage, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	st, err := s.State(ctx)
	if err != nil {
		return nil, err
	}
	c, ok := st.Conversations[p.ConversationID]
	if !ok || c.Source != p.Source {
		return nil, fmt.Errorf("description source/conversation scope mismatch")
	}
	id := conversationCommand(p.Source, c.NativeConversationID, "description_observed", p.AdapterVersion, p.Availability, p.Observation)
	guard := func(st model.State) error {
		if p.Availability == "available" && st.Conversations[p.ConversationID].Withdrawn[p.Association()] {
			return fmt.Errorf("description association withdrawn")
		}
		return nil
	}
	return s.TransactChecked(ctx, id, conversationActor, deliveryRequest(p), p.ObservedAt, guard, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"conversation", "description_observed", p.ConversationID, p}}, Result: ConversationReceipt{p.ConversationID, p.Association()}, descriptionBytes: map[string][]byte{p.Association(): retained}}, nil
	})
}
