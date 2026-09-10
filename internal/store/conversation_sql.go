package store

import (
	"context"
	"database/sql"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
)

// Both tables are disposable projections, not migrations or replay inputs.
// Creation under the still-open schema 22 is idempotent. Missing evidence is a
// gap, unlike the authoritative immutable workspace snapshot payloads.
const conversationTables = `
CREATE TABLE IF NOT EXISTS conversation_evidence (
 digest TEXT PRIMARY KEY, body BLOB NOT NULL CHECK(length(body) BETWEEN 1 AND 4096)
);
CREATE TABLE IF NOT EXISTS conversation_description_associations (
 id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, source_key TEXT NOT NULL,
 record_key TEXT NOT NULL, revision TEXT NOT NULL, digest TEXT NOT NULL,
 availability TEXT NOT NULL CHECK(availability IN ('available','withdrawn')),
 metadata TEXT NOT NULL, gap TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS conversation_description_scope ON conversation_description_associations(conversation_id,record_key);
`

func applyConversationSQL(ctx context.Context, tx *sql.Tx, e Event, payloads map[string][]byte) error {
	if e.Subject != "conversation" || e.Verb != "description_observed" {
		return nil
	}
	var d conversation.Description
	if err := model.StrictJSON(e.Payload, &d); err != nil {
		return err
	}
	id := d.Association()
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_description_associations(id,conversation_id,source_key,record_key,revision,digest,availability,metadata,gap)
 VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET availability=excluded.availability,metadata=excluded.metadata,gap=excluded.gap`, id, d.ConversationID, d.Source.Key(), d.RecordKey, d.SourceRevision, d.RetainedDigest, d.Availability, string(e.Payload), ""); err != nil {
		return err
	}
	if d.Availability == "withdrawn" {
		// Remove the shared bytes too. Other available associations become gaps and
		// may hydrate independently; this association remains denied permanently.
		_, err := tx.ExecContext(ctx, "DELETE FROM conversation_evidence WHERE digest=?", d.RetainedDigest)
		return err
	}
	if bytes := payloads[id]; bytes != nil {
		if len(bytes) != d.RetainedBytes || conversation.Digest(bytes) != d.RetainedDigest {
			return fmt.Errorf("invalid description evidence payload")
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO conversation_evidence(digest,body) VALUES(?,?) ON CONFLICT(digest) DO UPDATE SET body=excluded.body", d.RetainedDigest, bytes)
		return err
	}
	return nil
}

type DescriptionEvidence struct {
	Availability string `json:"availability"`
	Kind         string `json:"kind,omitempty"`
	Digest       string `json:"digest,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	Text         string `json:"text,omitempty"`
	Gap          string `json:"gap,omitempty"`
}

// checkDescriptionScope uses current replay metadata before consulting shared
// bytes. A digest or historical SQL row is never sufficient read authority.
func checkDescriptionScope(st model.State, d conversation.Description) string {
	c, ok := st.Conversations[d.ConversationID]
	if !ok || c.Source != d.Source {
		return "out_of_scope"
	}
	if c.Withdrawn[d.Association()] {
		return "withdrawn"
	}
	head, ok := c.DescriptionHeads[d.RecordKey]
	if !ok || head.Association() != d.Association() || head.RetainedDigest != d.RetainedDigest {
		return "out_of_scope"
	}
	if head.Availability != "available" {
		return "withdrawn"
	}
	return ""
}
func readDescriptionEvidence(ctx context.Context, tx *sql.Tx, st model.State, d conversation.Description) (DescriptionEvidence, error) {
	v := DescriptionEvidence{Availability: d.Availability, Kind: d.Kind, Digest: d.RetainedDigest, Truncated: d.Truncated}
	if gap := checkDescriptionScope(st, d); gap != "" {
		v.Gap = gap
		if gap == "withdrawn" {
			v.Availability = "withdrawn"
		}
		return v, nil
	}
	var availability, gap string
	var bytes []byte
	err := tx.QueryRowContext(ctx, `SELECT a.availability,a.gap,e.body FROM conversation_description_associations a LEFT JOIN conversation_evidence e ON e.digest=a.digest
 WHERE a.id=? AND a.conversation_id=? AND a.source_key=? AND a.record_key=? AND a.revision=? AND a.digest=?`, d.Association(), d.ConversationID, d.Source.Key(), d.RecordKey, d.SourceRevision, d.RetainedDigest).Scan(&availability, &gap, &bytes)
	if err == sql.ErrNoRows {
		v.Gap = "evidence_missing"
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if availability != "available" {
		v.Availability, v.Gap = "withdrawn", "withdrawn"
		return v, nil
	}
	if len(bytes) == 0 {
		v.Gap = gap
		if v.Gap == "" {
			v.Gap = "evidence_missing"
		}
		return v, nil
	}
	if conversation.Digest(bytes) != d.RetainedDigest || len(bytes) != d.RetainedBytes {
		v.Gap = "digest_mismatch"
		return v, nil
	}
	v.Text = string(bytes)
	return v, nil
}
func (s *Store) DescriptionEvidence(ctx context.Context, conversationID string, source conversation.SourceKey, recordKey, revision string) (DescriptionEvidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DescriptionEvidence{}, err
	}
	defer tx.Rollback()
	st, err := readState(ctx, tx)
	if err != nil {
		return DescriptionEvidence{}, err
	}
	c, ok := st.Conversations[conversationID]
	d, known := c.DescriptionHeads[recordKey]
	if !ok || !known || c.Source != source || d.SourceRevision != revision {
		return DescriptionEvidence{Availability: "unavailable", Gap: "out_of_scope"}, nil
	}
	return readDescriptionEvidence(ctx, tx, st, d)
}

// ResolvedDescription carries the exact eligible description bytes and the
// resolver's native record revision, never a transcript or synthesized summary.
type ResolvedDescription struct {
	SourceRevision string
	Text           []byte
}
type DescriptionResolver interface {
	ResolveDescription(context.Context, conversation.Description) (ResolvedDescription, error)
}
type HydrationDiagnostic struct {
	Association string `json:"association"`
	Gap         string `json:"gap,omitempty"`
	Hydrated    bool   `json:"hydrated"`
}

// Hydrate is an explicit bounded pass after replay. Resolver I/O holds no store
// lock. Every result rechecks current scope under a transaction to defeat races
// with withdrawal. Diagnostics live only in the disposable SQL association cache.
func (s *Store) Hydrate(ctx context.Context, resolver DescriptionResolver, limit int) ([]HydrationDiagnostic, error) {
	if resolver == nil || limit < 1 || limit > 128 {
		return nil, fmt.Errorf("resolver and hydration limit 1..128 required")
	}
	candidates, err := s.hydrationCandidates(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]HydrationDiagnostic, 0, len(candidates))
	for _, d := range candidates {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		resolved, resolveErr := resolver.ResolveDescription(ctx, d)
		gap := ""
		var retained []byte
		if resolveErr != nil {
			gap = "source_unavailable"
		} else {
			var truncated bool
			retained, truncated, err = conversation.Normalize(resolved.Text)
			if err != nil || resolved.SourceRevision != d.SourceRevision || conversation.Digest(resolved.Text) != d.OriginalDigest || conversation.Digest(retained) != d.RetainedDigest || len(retained) != d.RetainedBytes || truncated != d.Truncated {
				gap = "digest_mismatch"
			}
		}
		result, err := s.finishHydration(ctx, d, retained, gap)
		if err != nil {
			return out, err
		}
		out = append(out, result)
	}
	return out, nil
}
func (s *Store) hydrationCandidates(ctx context.Context, limit int) ([]conversation.Description, error) {
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
	rows, err := tx.QueryContext(ctx, `SELECT a.metadata FROM conversation_description_associations a LEFT JOIN conversation_evidence e ON e.digest=a.digest WHERE a.availability='available' AND e.digest IS NULL ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []conversation.Description{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var d conversation.Description
		if err := model.StrictJSON(raw, &d); err != nil {
			return nil, err
		}
		if checkDescriptionScope(st, d) == "" {
			out = append(out, d)
			if len(out) == limit {
				break
			}
		}
	}
	return out, rows.Err()
}
func (s *Store) finishHydration(ctx context.Context, d conversation.Description, bytes []byte, gap string) (HydrationDiagnostic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := HydrationDiagnostic{Association: d.Association()}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	st, err := readState(ctx, tx)
	if err != nil {
		return result, err
	}
	if current := checkDescriptionScope(st, d); current != "" {
		gap = current
	}
	var availability string
	err = tx.QueryRowContext(ctx, "SELECT availability FROM conversation_description_associations WHERE id=?", d.Association()).Scan(&availability)
	if err != nil {
		if err == sql.ErrNoRows {
			result.Gap = "source_unavailable"
			return result, nil
		}
		return result, err
	}
	if availability == "withdrawn" {
		gap = "withdrawn"
	}
	if gap == "" {
		if _, err = tx.ExecContext(ctx, "INSERT INTO conversation_evidence(digest,body) VALUES(?,?) ON CONFLICT(digest) DO UPDATE SET body=excluded.body", d.RetainedDigest, bytes); err != nil {
			return result, err
		}
		result.Hydrated = true
	}
	result.Gap = gap
	if _, err = tx.ExecContext(ctx, "UPDATE conversation_description_associations SET gap=? WHERE id=?", gap, d.Association()); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
