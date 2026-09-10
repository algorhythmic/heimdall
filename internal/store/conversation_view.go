package store

import (
	"context"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"sort"
	"strings"
	"time"
)

type ConversationView struct {
	ID            string                      `json:"id"`
	Kind          string                      `json:"kind"`
	Task          *conversation.TaskRef       `json:"task,omitempty"`
	Lifecycle     string                      `json:"lifecycle"`
	StartedAt     time.Time                   `json:"started_at"`
	LastStartedAt time.Time                   `json:"last_started_at"`
	ResumeCount   int                         `json:"resume_count"`
	Ended         *conversation.Ended         `json:"ended,omitempty"`
	TranscriptRef *conversation.TranscriptRef `json:"transcript_ref,omitempty"`
	TranscriptGap string                      `json:"transcript_gap,omitempty"`
	Description   DescriptionEvidence         `json:"description"`
}

func (s *Store) Conversations(ctx context.Context, target string, now time.Time) ([]ConversationView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	st, err := readState(ctx, tx)
	if err != nil {
		return nil, err
	}
	if target != "" {
		if _, _, err := model.ResolveTarget(st, target); err != nil {
			return nil, err
		}
	}
	out := []ConversationView{}
	for _, c := range st.Conversations {
		if target != "" && (c.Task == nil || (c.Task.Target != target && !strings.HasPrefix(c.Task.Target, target+"#"))) {
			continue
		}
		v := ConversationView{ID: c.ID, Kind: c.Kind, Task: c.Task, Lifecycle: c.Lifecycle(now), StartedAt: c.FirstStartedAt, LastStartedAt: c.StartedAt, ResumeCount: c.ResumeCount, Ended: c.Ended, TranscriptRef: c.TranscriptRef, Description: DescriptionEvidence{Availability: "unavailable", Gap: "not_observed"}}
		if v.TranscriptRef == nil {
			v.TranscriptGap = "not_observed"
		}
		if c.CurrentDescription != nil {
			v.Description, err = readDescriptionEvidence(ctx, tx, st, *c.CurrentDescription)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
