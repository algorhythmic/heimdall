package store

import (
	"fmt"
	"heimdall/internal/model"
	"time"
)

func externalGrant(st model.State, id, target string, now time.Time) bool {
	g, ok := st.Grants[id]
	return ok && g.Version == 2 && g.ActionWrite && !g.CheckpointWrite && g.RevokedAt == nil && !now.Before(g.At) && now.Before(g.ExpiresAt) && g.Contains(st, target)
}
func applyExternalAction(st *model.State, e Event) error {
	if e.Verb == "queued" {
		var v model.ActionIntent
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := v.ValidateExternal(); err != nil {
			return err
		}
		if e.Actor != "grant:"+v.AuthorityRef || e.EntityID != v.ID || !e.TS.Equal(v.At) || !externalGrant(*st, v.AuthorityRef, v.Target, e.TS) || v.ExpiresAt.After(st.Grants[v.AuthorityRef].ExpiresAt) || !model.ExternalInputsCurrent(*st, v) {
			return fmt.Errorf("external intent authority or scope changed")
		}
		if pin := v.External.Browser; pin != nil {
			scope, ok := model.ExternalBrowserScope(*st, v.External.Owned)
			scope.AfterEventID = pin.AfterEventID
			if !ok || scope != *pin || !model.ExternalBrowserFresh(st.Browsers[pin.Profile], pin.AfterEventID, v.At) {
				return fmt.Errorf("external browser registration scope changed")
			}
		}
		if _, ok := st.Actions[v.ID]; ok {
			return fmt.Errorf("duplicate action")
		}
		if _, ok := st.BrowserOperations[v.ID]; ok {
			return fmt.Errorf("action identity already used")
		}
		count := 0
		for _, a := range st.Actions {
			if model.ActionHolds(a) {
				count++
			}
		}
		if count >= 128 || model.ActionConflict(*st, v) != "" {
			return fmt.Errorf("unresolved action conflicts with external intent")
		}
		st.Actions[v.ID] = model.ActionRecord{Intent: v, IntentDigest: model.ContentDigest(v), Revision: 1, Execution: "external", Verification: "pending", UpdatedAt: v.At, LastEventID: e.ID}
		return nil
	}
	var v model.ActionTransition
	if err := model.StrictJSON(e.Payload, &v); err != nil {
		return err
	}
	a := st.Actions[e.EntityID]
	if a.Intent.External == nil || v.Version != 5 || !model.OpaqueID.MatchString(v.ID) || v.ActionID != e.EntityID || v.AttemptID != a.Intent.AttemptID || v.PreviousRevision != a.Revision || v.Actor != e.Actor || !v.At.Equal(e.TS) || v.At.Before(a.UpdatedAt) || len(v.Reason) > 512 || v.DeliveryID != "" || v.PairProbe != nil || v.PairReady != nil || v.AssociationID != "" || v.ContinuationID != "" {
		return fmt.Errorf("invalid external transition")
	}
	if (v.Kind != "report" && v.Report != nil) || (v.Kind != "reconcile" && v.Observation != nil) {
		return fmt.Errorf("foreign external transition payload")
	}
	switch v.Kind {
	case "report":
		r := v.Report
		if e.Actor != "grant:"+a.Intent.AuthorityRef || !externalGrant(*st, a.Intent.AuthorityRef, a.Intent.Target, e.TS) || !model.ExternalInputsCurrent(*st, a.Intent) || a.CancelRequested || a.FinalReport || a.Execution != "external" || !v.At.Before(a.Intent.ExpiresAt) || r == nil || r.Step != len(a.Reports)+1 || r.Step > a.Intent.External.Steps || r.Final != (r.Step == a.Intent.External.Steps) || !model.Contains([]string{"succeeded", "failed", "uncertain"}, r.Status) || r.Native != nil || r.TabID != 0 || r.WindowID != 0 || r.URL != "" || len(r.Detail) > 512 || len(r.WCURequestID) > 128 || (r.MetricsDigest != "" && !model.TokenHashPattern.MatchString(r.MetricsDigest)) {
			return fmt.Errorf("external report not authorized or out of order")
		}
		a.Reports = append(a.Reports, *r)
		a.Report = r
		a.FinalReport = r.Final
		a.Verification = "pending"
		a.VerificationAttempts = 0
		if r.Final {
			a.Execution = "api_reported"
			if r.Status == "uncertain" {
				a.Execution = "uncertain"
				a.UncertainSince = v.At
			}
		}
	case "cancel", "expire", "interrupt":
		if (e.Actor != "cli" && e.Actor != "coordinator") || (v.Kind != "cancel" && e.Actor != "coordinator") || (v.Kind == "expire" && v.At.Before(a.Intent.ExpiresAt)) || (v.Kind != "cancel" && a.Execution != "external") || (v.Kind == "cancel" && a.CancelRequested) {
			return fmt.Errorf("external termination not applicable")
		}
		if v.Kind == "cancel" {
			a.CancelRequested = true
		}
		if !a.FinalReport {
			a.Execution = "uncertain"
		}
		a.Verification = "pending"
		a.VerificationAttempts = 0
		if a.Execution == "uncertain" && a.UncertainSince.IsZero() {
			a.UncertainSince = v.At
		}
	case "reconcile":
		if e.Actor == "cli" && v.Observation == nil {
			a.Verification = "pending"
			a.VerificationAttempts = 0
			break
		}
		if e.Actor != "coordinator" || v.Observation == nil {
			return fmt.Errorf("independent observation required")
		}
		o := v.Observation
		if !model.OpaqueID.MatchString(o.ID) || (o.External != nil && (o.External.Source != "wcu" || len(o.External.Revision) > 256 || !model.TokenHashPattern.MatchString(o.External.Digest))) {
			return fmt.Errorf("invalid observation metadata")
		}
		if o.External != nil {
			x := o.External
			if x.TraceCount < 0 || x.TraceCount > 64 || x.TraceSubmitted < 0 || x.TraceSubmitted > x.TraceCount || x.TraceDurationMS < 0 || x.TraceDurationMS > 64*3600000 || (x.TraceCount == 0) != (x.TraceDigest == "") || (x.TraceDigest != "" && !model.TokenHashPattern.MatchString(x.TraceDigest)) {
				return fmt.Errorf("invalid WCU trace corroboration")
			}
			if x.Revision == "" || x.ReportCount < 0 || x.ReportCount > 64 || (x.ReportCount == 0) != (x.ReportsDigest == "") || (x.ReportsDigest != "" && !model.TokenHashPattern.MatchString(x.ReportsDigest)) || (x.TargetMismatch && x.ReportCount == 0) || ((v.Reason == "target_mismatch") != x.TargetMismatch) {
				return fmt.Errorf("invalid WCU corroboration")
			}
		}
		status, detail := model.ExternalOutcome(*st, a, o, v.At)
		if o.Status != status || o.Detail != detail {
			return fmt.Errorf("verification must be derived from observation")
		}
		a.Observation = o
		a.Verification = status
		a.VerificationAttempts++
	default:
		return fmt.Errorf("external actions never dispatch input")
	}
	a.Revision++
	a.UpdatedAt = v.At
	a.LastEventID = e.ID
	a.LastReason = v.Reason
	st.Actions[e.EntityID] = a
	return nil
}
