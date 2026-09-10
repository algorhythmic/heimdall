package store

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"heimdall/internal/model"
)

func compositorState(t *testing.T) (model.State, string, string, time.Time) {
	t.Helper()
	st := model.Empty()
	now := time.Now().UTC().Truncate(time.Millisecond)
	sourceID, epoch := model.NewID(), strings.Repeat("a", 64)
	st.DesktopSources[sourceID] = model.DesktopSource{ID: sourceID, Active: true, Epoch: epoch}
	st.DesktopSourceHead = sourceID
	return st, sourceID, epoch, now
}

func compositorSpan(sourceID, epoch, stableID string, sequence int64, started, ended time.Time) model.CompositorSurfaceFocusSpan {
	return model.CompositorSurfaceFocusSpan{Version: 1, SourceID: sourceID, SourceEpoch: epoch, Sequence: sequence,
		Window: model.WindowIdentity{SourceEpoch: epoch, StableID: stableID}, Class: "foot", CompositorWorkspaceID: 1, CompositorWorkspaceName: "one",
		Title: "one", StartedAt: started, EndedAt: ended, DurationSeconds: ended.Sub(started).Seconds(), Gaps: []string{}}
}

func compositorEvent(t *testing.T, id int64, span model.CompositorSurfaceFocusSpan) Event {
	t.Helper()
	return Event{Version: 1, ID: id, TS: span.EndedAt, Subject: "surface", Verb: "focused", EntityID: span.Window.StableID, Actor: "observer:hyprland",
		CommandID: "hyprland-attention-" + span.SourceID + "-" + span.SourceEpoch + "-" + strconv.FormatInt(span.Sequence, 10), Payload: compositorPayload(t, span)}
}

func TestCompositorFocusAcceptsSpanWithoutInventoryEvent(t *testing.T) {
	st, sourceID, epoch, now := compositorState(t)
	span := compositorSpan(sourceID, epoch, "18000001", 2, now.Add(time.Second), now.Add(3*time.Second))
	if err := Apply(&st, compositorEvent(t, 1, span)); err != nil {
		t.Fatal(err)
	}
	if st.CompositorWindowFocusSpans["18000001"].Sequence != 2 || st.CompositorSurfaceFocusSpans[sourceID+":"+epoch].Sequence != 2 {
		t.Fatal("projection missing", st.CompositorWindowFocusSpans)
	}
	// Replaying the same events into a fresh state yields the same projection.
	again, _, _, _ := compositorState(t)
	again.DesktopSources, again.DesktopSourceHead = st.DesktopSources, st.DesktopSourceHead
	if err := Apply(&again, compositorEvent(t, 1, span)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.CompositorWindowFocusSpans, st.CompositorWindowFocusSpans) || !reflect.DeepEqual(again.CompositorSurfaceFocusSpans, st.CompositorSurfaceFocusSpans) {
		t.Fatal("replay diverged")
	}
}

func TestCompositorFocusRejectsDuplicateOverlapRegressionAndForeignEpoch(t *testing.T) {
	st, sourceID, epoch, now := compositorState(t)
	span := compositorSpan(sourceID, epoch, "18000001", 2, now.Add(time.Second), now.Add(3*time.Second))
	if err := Apply(&st, compositorEvent(t, 1, span)); err != nil {
		t.Fatal(err)
	}
	before := model.Clone(st)
	if err := Apply(&st, compositorEvent(t, 2, span)); err == nil {
		t.Fatal("duplicate accepted")
	}
	overlap := compositorSpan(sourceID, epoch, "18000002", 3, now.Add(2*time.Second), now.Add(5*time.Second))
	if err := Apply(&st, compositorEvent(t, 2, overlap)); err == nil {
		t.Fatal("overlapping span accepted")
	}
	regressed := compositorSpan(sourceID, epoch, "18000002", 1, now.Add(4*time.Second), now.Add(7*time.Second))
	if err := Apply(&st, compositorEvent(t, 2, regressed)); err == nil {
		t.Fatal("sequence regression accepted")
	}
	foreign := compositorSpan(sourceID, strings.Repeat("b", 64), "18000002", 4, now.Add(4*time.Second), now.Add(7*time.Second))
	if err := Apply(&st, compositorEvent(t, 2, foreign)); err == nil {
		t.Fatal("span from a non-current source epoch accepted")
	}
	inactive := compositorSpan(model.NewID(), epoch, "18000002", 4, now.Add(4*time.Second), now.Add(7*time.Second))
	if err := Apply(&st, compositorEvent(t, 2, inactive)); err == nil {
		t.Fatal("span from a non-head source accepted")
	}
	if !reflect.DeepEqual(st, before) {
		t.Fatal("rejected spans mutated state")
	}
	next := compositorSpan(sourceID, epoch, "18000002", 3, now.Add(3*time.Second), now.Add(6*time.Second))
	if err := Apply(&st, compositorEvent(t, 2, next)); err != nil {
		t.Fatal(err)
	}
	if len(st.CompositorWindowFocusSpans) != 2 {
		t.Fatal("per-window projection lost an entry", st.CompositorWindowFocusSpans)
	}
}

func TestCompositorFocusProjectionBoundedByEpochAndCount(t *testing.T) {
	st, sourceID, epoch, now := compositorState(t)
	at := now
	for i := 0; i < 300; i++ {
		span := compositorSpan(sourceID, epoch, "18000"+strconv.Itoa(1000+i), int64(i+1), at, at.Add(2*time.Second))
		if err := Apply(&st, compositorEvent(t, int64(i+1), span)); err != nil {
			t.Fatal(i, err)
		}
		at = at.Add(2 * time.Second)
	}
	if len(st.CompositorWindowFocusSpans) != 256 {
		t.Fatal("window projection not bounded", len(st.CompositorWindowFocusSpans))
	}
	if _, ok := st.CompositorWindowFocusSpans["180001000"]; ok {
		t.Fatal("oldest span retained past the bound")
	}
	// A new source epoch drops every window entry from the previous epoch.
	newEpoch := strings.Repeat("c", 64)
	st.DesktopSources[sourceID] = model.DesktopSource{ID: sourceID, Active: true, Epoch: newEpoch}
	span := compositorSpan(sourceID, newEpoch, "18000001", 1, at, at.Add(3*time.Second))
	if err := Apply(&st, compositorEvent(t, 301, span)); err != nil {
		t.Fatal(err)
	}
	if len(st.CompositorWindowFocusSpans) != 1 || len(st.CompositorSurfaceFocusSpans) != 2 {
		t.Fatal("epoch change did not prune window spans", len(st.CompositorWindowFocusSpans), len(st.CompositorSurfaceFocusSpans))
	}
}

func TestCompositorWorkspaceFocusSpan(t *testing.T) {
	st, sourceID, epoch, now := compositorState(t)
	span := model.CompositorWorkspaceFocusSpan{Version: 1, SourceID: sourceID, SourceEpoch: epoch, Sequence: 2, CompositorWorkspaceID: 1,
		CompositorWorkspaceName: "one", Monitor: "HDMI-A-1", StartedAt: now, EndedAt: now.Add(4 * time.Second), DurationSeconds: 4, Gaps: []string{}}
	event := Event{Version: 1, ID: 1, TS: span.EndedAt, Subject: "workspace", Verb: "focused", EntityID: sourceID, Actor: "observer:hyprland",
		CommandID: "hyprland-attention-" + sourceID + "-" + epoch + "-2", Payload: compositorPayload(t, span)}
	if err := Apply(&st, event); err != nil {
		t.Fatal(err)
	}
	if st.CompositorWorkspaceFocusSpans[sourceID+":"+epoch].CompositorWorkspaceID != 1 {
		t.Fatal("workspace projection missing")
	}
	if err := Apply(&st, event); err == nil {
		t.Fatal("duplicate workspace span accepted")
	}
	span.Monitor, span.Gaps = "", []string{}
	event.Payload = compositorPayload(t, span)
	if err := Apply(&st, event); err == nil {
		t.Fatal("missing monitor accepted without explicit gap")
	}
}

func compositorPayload(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
