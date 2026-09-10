package workspace

import (
	"errors"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"strings"
	"testing"
	"time"
)

func TestPairedBrowserWindowExclusion(t *testing.T) {
	sourceID, profile, epoch := model.NewID(), model.NewID(), model.NewID()
	window := model.WindowIdentity{SourceEpoch: strings.Repeat("a", 64), StableID: "18000001"}
	st := model.Empty()
	st.BrowserAssociations[model.NewID()] = model.BrowserAssociation{Version: 1, Profile: profile, Epoch: epoch, SourceID: sourceID, Window: window}
	st.Browsers[profile] = model.BrowserProfile{ID: profile, Epoch: epoch, Connection: model.NewID(), Paired: true}
	if !pairedBrowserWindow(st, sourceID, window) {
		t.Fatal("live paired browser window not excluded")
	}
	p := st.Browsers[profile]
	p.Connection = ""
	st.Browsers[profile] = p
	if pairedBrowserWindow(st, sourceID, window) {
		t.Fatal("disconnected browser window still excluded; compositor is the fallback")
	}
	p.Connection, p.Epoch = model.NewID(), model.NewID()
	st.Browsers[profile] = p
	if pairedBrowserWindow(st, sourceID, window) {
		t.Fatal("association from an old browser epoch still excluded")
	}
	if pairedBrowserWindow(st, model.NewID(), window) {
		t.Fatal("association from another desktop source matched")
	}
}

func TestAttentionRecordReportsRefusals(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := AttentionService{Store: db}
	if err := s.Record(hyprland.AttentionObservation{}); err == nil {
		t.Fatal("empty observation accepted")
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	epoch := strings.Repeat("a", 64)
	span := model.CompositorSurfaceFocusSpan{Version: 1, SourceID: model.NewID(), SourceEpoch: epoch, Sequence: 1,
		Window: model.WindowIdentity{SourceEpoch: epoch, StableID: "18000001"}, Class: "foot", CompositorWorkspaceID: 1, CompositorWorkspaceName: "one",
		Title: "one", StartedAt: now, EndedAt: now.Add(3 * time.Second), DurationSeconds: 3, Gaps: []string{}}
	// No selected desktop source: the reducer refuses and the refusal is
	// returned rather than swallowed.
	err = s.Record(hyprland.AttentionObservation{Surface: &span})
	if err == nil || errors.Is(err, hyprland.ErrAttentionExcluded) {
		t.Fatal("store refusal not reported", err)
	}
	st, err := db.State(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.CompositorSurfaceFocusSpans) != 0 || len(st.CompositorWindowFocusSpans) != 0 {
		t.Fatal("refused span reached the projection")
	}
}
