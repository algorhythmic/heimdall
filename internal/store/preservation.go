package store

import (
	"fmt"
	"heimdall/internal/model"
)

func applyPreservation(st *model.State, e Event) error {
	if e.Actor != "cli" {
		return fmt.Errorf("preservation requires CLI authority")
	}
	switch e.Verb {
	case "requested":
		var p model.PreservationPlan
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := model.ValidRecord(p.Version, p.ID, p.Target, p.Actor, p.At); err != nil {
			return err
		}
		if p.ID != e.EntityID || p.Actor != e.Actor || !p.At.Equal(e.TS) || !model.ValidPreservationDigest(p.Host) || !model.ValidPreservationDigest(p.PreviewDigest) || p.Snapshot.RemoteStatus != "not_checked" {
			return fmt.Errorf("invalid preservation plan envelope")
		}
		if _, ok := st.PreservationPlans[p.ID]; ok {
			return fmt.Errorf("duplicate preservation plan")
		}
		if err := p.Input.Validate(); err != nil {
			return err
		}
		if err := model.PreservationPins(*st, p.Target, p.Input); err != nil {
			return err
		}
		t, _, err := model.ResolveTarget(*st, p.Target)
		if err != nil || t.Revision != p.TaskRevision {
			return fmt.Errorf("preservation task revision mismatch")
		}
		if err := p.Snapshot.Validate(p.Input, *st); err != nil {
			return err
		}
		if p.PreviewDigest != model.PreservationDigest([]any{p.Target, p.TaskRevision, p.Host, p.Input, p.Snapshot}) {
			return fmt.Errorf("preservation preview mismatch")
		}
		st.PreservationPlans[p.ID] = p
	case "observed":
		var r model.PreservationReceipt
		if err := model.StrictJSON(e.Payload, &r); err != nil {
			return err
		}
		if err := model.ValidRecord(r.Version, r.ID, r.Target, r.Actor, r.At); err != nil {
			return err
		}
		if r.ID != e.EntityID || r.Actor != e.Actor || !r.At.Equal(e.TS) || !model.ValidPreservationReport(r.ReportedOutcome, r.Note) {
			return fmt.Errorf("invalid preservation receipt envelope")
		}
		if _, ok := st.PreservationReceipts[r.ID]; ok {
			return fmt.Errorf("duplicate preservation receipt")
		}
		p, ok := st.PreservationPlans[r.PlanID]
		if !ok || p.Target != r.Target || st.PreservationHeads[p.ID] != r.Previous {
			return fmt.Errorf("preservation plan or receipt head mismatch")
		}
		t, _, err := model.ResolveTarget(*st, r.Target)
		if err != nil || t.Revision != r.TaskRevision {
			return fmt.Errorf("preservation task revision mismatch")
		}
		if err := r.Snapshot.Validate(p.Input, *st); err != nil {
			return err
		}
		if model.Contains([]string{"matched", "different", "missing"}, r.Snapshot.RemoteStatus) && (r.Snapshot.RemoteFingerprint == "" || r.Snapshot.RemoteFingerprint != p.Snapshot.RemoteFingerprint) {
			return fmt.Errorf("remote readback outside pinned destination")
		}
		st.PreservationReceipts[r.ID] = r
		st.PreservationHeads[p.ID] = r.ID
	default:
		return fmt.Errorf("unknown preservation event")
	}
	return nil
}
