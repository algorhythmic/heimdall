//go:build linux

package continuity

import (
	"encoding/json"
	"errors"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProgressArtifactDigestLifecycleAndReplay(t *testing.T) {
	f, root, file, input := artifactFixture(t)
	a := recordArtifact(t, f, input)
	contract := f.contract()
	p := proposeProgress(t, f, ProgressInput{Kind: "artifact", Text: "Design is ready for review", ContractID: contract, Artifacts: []model.ArtifactRef{{ArtifactID: a.Artifact.ID, VersionID: a.Record.ID}}, Previous: "none"})
	if p.Artifacts[0].Observation.Digest != a.Record.Observation.Digest {
		t.Fatal("proposal did not freeze observed digest")
	}
	review := progressReview(f, p, "reviewed", "none")
	sendProgress(t, f, review)
	view, err := f.s.ArtifactView(f.ctx, f.target, a.Artifact.ID, "")
	if err != nil || view.Lifecycle != "reviewed" {
		t.Fatal(view, err)
	}
	accept := progressReview(f, p, "accepted", review.ID)
	bytes, _ := os.ReadFile(file)
	for _, drift := range []string{"bytes", "permissions", "missing"} {
		switch drift {
		case "bytes":
			os.WriteFile(file, []byte("Material changes after review"), 0600)
		case "permissions":
			os.Chmod(file, 0700)
		case "missing":
			os.Remove(file)
		}
		if _, err := f.s.Progress(f.ctx, accept, "cli", f.now); !errors.Is(err, store.ErrConflict) {
			t.Fatal("accepted changed artifact", drift, err)
		}
		v, err := f.s.ProgressShow(f.ctx, f.target, p.ID)
		if err != nil || v.Status != "reviewed" || v.Freshness != "stale" {
			t.Fatal(v, err)
		}
		os.WriteFile(file, bytes, 0600)
		os.Chmod(file, 0600)
	}
	first := sendProgress(t, f, accept)
	view, err = f.s.ArtifactView(f.ctx, f.target, a.Artifact.ID, "")
	if err != nil || view.Lifecycle != "accepted" {
		t.Fatal(view, err)
	}
	st, _ := f.e.Store.State(f.ctx)
	if st.Tasks[f.target].Task.Status != "active" || st.Tasks[f.target].Revision != f.rev || len(st.Evidence) != 0 || len(st.Proposals) != 0 {
		t.Fatal("artifact acceptance completed work")
	}
	// Scoped readers see text/status, never the newly observed digest, host,
	// Git identity, or expanded artifact records under legacy resource grants.
	g := model.Grant{Target: f.target, ResourceIDs: []string{input.Artifact.ResourceID}}
	bundle, err := ScopedContext(f.ctx, st, g, f.target, 16000)
	if err != nil || bundle.Progress[0].Freshness != "requires_cli_check" {
		t.Fatal(bundle, err)
	}
	raw, _ := json.Marshal(bundle.Progress)
	if strings.Contains(string(raw), a.Record.Observation.Digest) || strings.Contains(string(raw), a.Artifact.Host) {
		t.Fatal("scoped summary leaked expanded identity")
	}
	// Same bytes still need review for an explicitly recorded replacement.
	input.ID = model.NewID()
	input.Artifact.ArtifactID = a.Artifact.ID
	input.Artifact.Previous = a.Record.ID
	input.Artifact.Name = ""
	newVersion := recordArtifact(t, f, input)
	view, _ = f.s.ArtifactView(f.ctx, f.target, a.Artifact.ID, "")
	if view.Lifecycle != "draft" {
		t.Fatal("replacement inherited acceptance", view)
	}
	old, _ := f.s.ProgressShow(f.ctx, f.target, p.ID)
	if old.Status != "superseded" || old.Review.Status != "accepted" {
		t.Fatal("historical acceptance lost", old)
	}
	newP := proposeProgress(t, f, ProgressInput{Kind: "artifact", Text: "Review replacement version", ContractID: contract, Artifacts: []model.ArtifactRef{{ArtifactID: a.Artifact.ID, VersionID: newVersion.Record.ID}}, Previous: p.ID})
	sendProgress(t, f, progressReview(f, newP, "rejected", "none"))
	view, _ = f.s.ArtifactView(f.ctx, f.target, a.Artifact.ID, "")
	if view.Lifecycle != "draft" {
		t.Fatal("rejection not reflected", view)
	}
	os.RemoveAll(root)
	if retry := sendProgress(t, f, accept); string(retry) != string(first) {
		t.Fatal("retry reobserved original")
	}
	before, _ := f.e.Store.State(f.ctx)
	replay, err := f.e.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, replay) {
		t.Fatal("replay touched originals", err)
	}
}

func TestProgressArtifactReplacementHeadAndLinkedDecision(t *testing.T) {
	f, _, file, input := artifactFixture(t)
	a := recordArtifact(t, f, input)
	contract := f.contract()
	in := ProgressInput{Kind: "artifact", Text: "First review", ContractID: contract, Artifacts: []model.ArtifactRef{{ArtifactID: a.Artifact.ID, VersionID: a.Record.ID}}, Previous: "none"}
	first := proposeProgress(t, f, in)
	in.Text = "Replacement review"
	r := ProgressRequest{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, Proposal: &in}
	if _, err := f.s.Progress(f.ctx, r, "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("competing artifact head accepted", err)
	}
	in.Previous = first.ID
	proposeProgress(t, f, in)
	if _, err := f.s.Progress(f.ctx, progressReview(f, first, "accepted", "none"), "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("superseded proposal accepted", err)
	}
	d := proposeProgress(t, f, ProgressInput{Kind: "decision", Text: "Adopt this design", ContractID: contract, Artifacts: in.Artifacts})
	os.WriteFile(file, []byte("Design changed"), 0600)
	if _, err := f.s.Progress(f.ctx, progressReview(f, d, "accepted", "none"), "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale artifact-linked decision accepted", err)
	}
	// Stale proposals can be explicitly rejected, with no observation needed.
	sendProgress(t, f, progressReview(f, d, "rejected", "none"))
}
