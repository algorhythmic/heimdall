package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/adapters/application"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"time"
)

type ApplicationRequest struct {
	Version              int                    `json:"version"`
	ID                   string                 `json:"id"`
	Target               string                 `json:"target"`
	ExpectedTaskRevision int64                  `json:"expected_task_revision"`
	ManifestID           string                 `json:"manifest_id"`
	SurfaceID            string                 `json:"surface_id"`
	Previous             string                 `json:"previous"`
	Spec                 *model.ApplicationSpec `json:"spec,omitempty"`
}

func (s Service) ReviewApplication(ctx context.Context, r ApplicationRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" || (r.Previous != "none" && !model.OpaqueID.MatchString(r.Previous)) {
		return nil, fmt.Errorf("explicit local recipe review and previous head required")
	}
	v := model.ApplicationRecipe{Version: r.Version, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, ManifestID: r.ManifestID, SurfaceID: r.SurfaceID, Previous: previous(r.Previous), Active: r.Spec != nil, Spec: r.Spec, Actor: actor, At: now.UTC()}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(r)
	if len(raw) > MaxRequest {
		return nil, fmt.Errorf("application request too large")
	}
	command := "application-" + r.ID
	if result, found, err := s.Store.CommandReceipt(ctx, command, raw); found || err != nil {
		return result, err
	}
	if r.Spec != nil {
		if err := application.CheckFiles(*r.Spec); err != nil {
			return nil, err
		}
	}
	return s.Store.Transact(ctx, command, actor, raw, now, func(st model.State) (store.Change, error) {
		return store.Change{Revision: st.Revision, Events: []store.Pending{{Subject: "application", Verb: "reviewed", EntityID: v.ID, Payload: v}}, Result: v}, nil
	})
}

func (s Service) Application(ctx context.Context, target, surface string) (model.ApplicationRecipe, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return model.ApplicationRecipe{}, err
	}
	r := st.ApplicationRecipes[st.ApplicationHeads[surface]]
	if r.ID == "" || r.Target != target || r.SurfaceID != surface {
		return r, fmt.Errorf("application recipe not found for task and surface")
	}
	return r, nil
}
