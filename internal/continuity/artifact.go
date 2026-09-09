package continuity

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"reflect"
	"sort"
	"time"
)

type ArtifactInput struct {
	ArtifactID  string `json:"artifact_id"`
	Previous    string `json:"previous"`
	Name        string `json:"name,omitempty"`
	Environment string `json:"environment"`
	ResourceID  string `json:"resource_id"`
	Path        string `json:"path"`
	Git         bool   `json:"git"`
}
type ArtifactRequest struct {
	Version              int           `json:"version"`
	ID                   string        `json:"id"`
	Target               string        `json:"target"`
	ExpectedTaskRevision int64         `json:"expected_task_revision"`
	Artifact             ArtifactInput `json:"artifact"`
}

func (r ArtifactRequest) Validate() error {
	a := r.Artifact
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || r.ExpectedTaskRevision < 1 || !model.ValidArtifactEnvironment(a.Environment) || !model.OpaqueID.MatchString(a.ResourceID) || !model.ValidArtifactPath(a.Path) {
		return fmt.Errorf("invalid artifact request")
	}
	if a.ArtifactID == "new" {
		if a.Previous != "none" || !textValid(a.Name, 256) {
			return fmt.Errorf("new artifact requires name and previous none")
		}
	} else {
		if !model.OpaqueID.MatchString(a.ArtifactID) || !model.OpaqueID.MatchString(a.Previous) || a.Name != "" {
			return fmt.Errorf("existing artifact requires exact ID and previous version; name is immutable")
		}
	}
	return nil
}
func DecodeArtifact(body []byte) (ArtifactRequest, error) {
	var r ArtifactRequest
	if len(body) > MaxRequest {
		return r, fmt.Errorf("artifact request exceeds 64 KiB")
	}
	if err := model.StrictJSON(body, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}

type ArtifactView struct {
	Artifact model.Artifact        `json:"artifact"`
	Head     string                `json:"head"`
	Record   model.ArtifactVersion `json:"record"`
}
type ArtifactCheck struct {
	Artifact  model.Artifact        `json:"artifact"`
	Record    model.ArtifactVersion `json:"record"`
	Status    string                `json:"status"`
	CheckedAt time.Time             `json:"checked_at"`
}

func (s Service) RecordArtifact(ctx context.Context, r ArtifactRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("artifact recording requires CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, _ := json.Marshal(r)
	if len(raw) > MaxRequest {
		return nil, fmt.Errorf("artifact request exceeds 64 KiB")
	}
	return s.Store.Transact(ctx, "artifact-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		in := r.Artifact
		task, _, err := model.ResolveTarget(st, r.Target)
		if err != nil {
			return c, err
		}
		if task.Revision != r.ExpectedTaskRevision {
			return c, fmt.Errorf("artifact task revision: %w", store.ErrConflict)
		}
		scope, err := model.ResourceScope(st, r.Target)
		if err != nil || !model.Contains(scope, in.ResourceID) {
			return c, fmt.Errorf("artifact resource outside target lineage")
		}
		host, err := artifactHost()
		if err != nil {
			return c, err
		}
		a := st.Artifacts[in.ArtifactID]
		previous := in.Previous
		if in.ArtifactID == "new" {
			a = model.Artifact{Version: 1, ID: r.ID, Target: r.Target, Name: in.Name, Environment: in.Environment, Host: host, Platform: "linux", Actor: actor, At: now.UTC()}
			previous = ""
			if err = model.ValidArtifact(a); err != nil {
				return c, err
			}
			c.Events = append(c.Events, store.Pending{Subject: "artifact", Verb: "registered", EntityID: a.ID, Payload: a})
		} else if a.ID == "" || a.Target != r.Target || a.Environment != in.Environment || a.Host != host || st.ArtifactHeads[a.ID] != previous {
			return c, fmt.Errorf("artifact scope or version changed: %w", store.ErrConflict)
		}
		observation, err := ObserveArtifact(ctx, st.Resources[in.ResourceID], in.Path, in.Git)
		if err != nil {
			return c, err
		}
		v := model.ArtifactVersion{Version: 1, ID: r.ID, ArtifactID: a.ID, Target: r.Target, TaskRevision: task.Revision, Previous: previous, ResourceID: in.ResourceID, Path: in.Path, Observation: observation, Actor: actor, At: now.UTC()}
		if err = model.ValidArtifactVersion(v); err != nil {
			return c, err
		}
		c.Events = append(c.Events, store.Pending{Subject: "artifact", Verb: "versioned", EntityID: v.ID, Payload: v})
		c.Result = ArtifactView{a, v.ID, v}
		return c, nil
	})
}
func artifactSelection(st model.State, target, id, version string) (ArtifactView, error) {
	a, ok := st.Artifacts[id]
	if !ok || a.Target != target {
		return ArtifactView{}, fmt.Errorf("artifact not found for target")
	}
	if version == "" {
		version = st.ArtifactHeads[id]
	}
	v, ok := st.ArtifactVersions[version]
	if !ok || v.ArtifactID != id || v.Target != target {
		return ArtifactView{}, fmt.Errorf("artifact version not found for target")
	}
	return ArtifactView{a, st.ArtifactHeads[id], v}, nil
}
func (s Service) ArtifactView(ctx context.Context, target, id, version string) (ArtifactView, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return ArtifactView{}, err
	}
	return artifactSelection(st, target, id, version)
}

type ArtifactListItem struct {
	Artifact model.Artifact `json:"artifact"`
	Head     string         `json:"head"`
}

func (s Service) ArtifactList(ctx context.Context, target string) ([]ArtifactListItem, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if _, _, err = model.ResolveTarget(st, target); err != nil {
		return nil, err
	}
	items := []ArtifactListItem{}
	for _, a := range st.Artifacts {
		if a.Target == target {
			items = append(items, ArtifactListItem{a, st.ArtifactHeads[a.ID]})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Artifact.ID < items[j].Artifact.ID })
	return items, nil
}
func checkArtifact(ctx context.Context, st model.State, v ArtifactView, now time.Time) ArtifactCheck {
	c := ArtifactCheck{Artifact: v.Artifact, Record: v.Record, Status: "unavailable", CheckedAt: now.UTC()}
	latest := st.ArtifactVersions[st.ArtifactHeads[v.Artifact.ID]]
	if latest.ID != v.Record.ID {
		c.Status = "version_changed"
		if latest.ResourceID != v.Record.ResourceID || latest.Path != v.Record.Path {
			c.Status = "relocated"
		}
		return c
	}
	host, err := artifactHost()
	if err != nil {
		return c
	}
	if host != v.Artifact.Host {
		c.Status = "host_changed"
		return c
	}
	r := st.Resources[v.Record.ResourceID]
	if !r.Active {
		c.Status = "unbound"
		return c
	}
	scope, err := model.ResourceScope(st, v.Artifact.Target)
	if err != nil || !model.Contains(scope, r.ID) {
		c.Status = "scope_changed"
		return c
	}
	observed, err := ObserveArtifact(ctx, r, v.Record.Path, v.Record.Observation.Git != nil)
	if err != nil {
		if os.IsNotExist(err) {
			c.Status = "missing"
		}
		return c
	}
	c.Status = "matched"
	if observed.Digest != v.Record.Observation.Digest || observed.Bytes != v.Record.Observation.Bytes {
		c.Status = "content_changed"
	} else if !reflect.DeepEqual(observed, v.Record.Observation) {
		c.Status = "identity_changed"
	}
	return c
}
func (s Service) CheckArtifact(ctx context.Context, target, id, version string) (ArtifactCheck, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return ArtifactCheck{}, err
	}
	v, err := artifactSelection(st, target, id, version)
	if err != nil {
		return ArtifactCheck{}, err
	}
	c := checkArtifact(ctx, st, v, time.Now())
	after, err := s.Store.State(ctx)
	if err != nil {
		return c, err
	}
	// Include lineage/contract changes, not just the artifact head: resource
	// authority can change while a filesystem observation is in progress.
	if st.LastEventID != after.LastEventID {
		c.Status = "state_changed"
	}
	return c, nil
}
func validateLiveArtifactRefs(ctx context.Context, st model.State, target string, refs []model.ArtifactRef, resources []string) error {
	for _, ref := range refs {
		v, err := artifactSelection(st, target, ref.ArtifactID, ref.VersionID)
		if err != nil {
			return err
		}
		if !model.Contains(resources, v.Record.ResourceID) {
			return fmt.Errorf("artifact outside accepted resource scope")
		}
		check := checkArtifact(ctx, st, v, time.Now())
		if check.Status != "matched" {
			return fmt.Errorf("artifact %s is %s: %w", ref.ArtifactID, check.Status, store.ErrConflict)
		}
	}
	return nil
}
