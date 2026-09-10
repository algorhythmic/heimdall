package store

import (
	"fmt"
	"heimdall/internal/model"
	"sort"
	"strings"
)

func compositorSource(st *model.State, id, epoch string) bool {
	source := st.DesktopSources[id]
	return id == st.DesktopSourceHead && source.Active && source.Epoch == epoch
}
func applyCompositorSurfaceFocus(st *model.State, e Event) error {
	var f model.CompositorSurfaceFocusSpan
	if err := model.StrictJSON(e.Payload, &f); err != nil {
		return err
	}
	if f.Validate() != nil || e.Actor != "observer:hyprland" || e.EntityID != f.Window.StableID || !strings.HasPrefix(e.CommandID, "hyprland-attention-"+f.SourceID+"-"+f.SourceEpoch+"-") || !compositorSource(st, f.SourceID, f.SourceEpoch) || !f.EndedAt.Equal(e.TS) {
		return fmt.Errorf("invalid compositor surface focus provenance")
	}
	key := f.SourceID + ":" + f.SourceEpoch
	if previous, ok := st.CompositorSurfaceFocusSpans[key]; ok && (previous.Sequence >= f.Sequence || previous.EndedAt.After(f.StartedAt)) {
		return fmt.Errorf("overlapping or duplicate compositor surface focus span")
	}
	if st.CompositorSurfaceFocusSpans == nil {
		st.CompositorSurfaceFocusSpans = map[string]model.CompositorSurfaceFocusSpan{}
	}
	if st.CompositorWindowFocusSpans == nil {
		st.CompositorWindowFocusSpans = map[string]model.CompositorSurfaceFocusSpan{}
	}
	st.CompositorSurfaceFocusSpans[key] = f
	st.CompositorWindowFocusSpans[f.Window.StableID] = f
	for id, span := range st.CompositorWindowFocusSpans {
		if span.SourceEpoch != f.SourceEpoch {
			delete(st.CompositorWindowFocusSpans, id)
		}
	}
	if len(st.CompositorWindowFocusSpans) > 256 {
		spans := make([]model.CompositorSurfaceFocusSpan, 0, len(st.CompositorWindowFocusSpans))
		for _, span := range st.CompositorWindowFocusSpans {
			spans = append(spans, span)
		}
		sort.Slice(spans, func(i, j int) bool {
			if spans[i].EndedAt.Equal(spans[j].EndedAt) {
				return spans[i].Window.StableID < spans[j].Window.StableID
			}
			return spans[i].EndedAt.Before(spans[j].EndedAt)
		})
		for _, span := range spans[:len(spans)-256] {
			delete(st.CompositorWindowFocusSpans, span.Window.StableID)
		}
	}
	return nil
}
func applyCompositorWorkspaceFocus(st *model.State, e Event) error {
	var f model.CompositorWorkspaceFocusSpan
	if err := model.StrictJSON(e.Payload, &f); err != nil {
		return err
	}
	if f.Validate() != nil || e.Actor != "observer:hyprland" || !strings.HasPrefix(e.CommandID, "hyprland-attention-"+f.SourceID+"-"+f.SourceEpoch+"-") || !compositorSource(st, f.SourceID, f.SourceEpoch) || !f.EndedAt.Equal(e.TS) {
		return fmt.Errorf("invalid compositor workspace focus provenance")
	}
	key := f.SourceID + ":" + f.SourceEpoch
	if previous, ok := st.CompositorWorkspaceFocusSpans[key]; ok && (previous.Sequence >= f.Sequence || previous.EndedAt.After(f.StartedAt)) {
		return fmt.Errorf("overlapping or duplicate compositor workspace focus span")
	}
	if st.CompositorWorkspaceFocusSpans == nil {
		st.CompositorWorkspaceFocusSpans = map[string]model.CompositorWorkspaceFocusSpan{}
	}
	st.CompositorWorkspaceFocusSpans[key] = f
	return nil
}
