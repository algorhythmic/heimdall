package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"reflect"
	"testing"
	"time"
)

func focusEvent(st model.State, o model.BrowserSurfaceObservation, started, ended time.Time) Event {
	p := st.Browsers[o.Profile]
	f := model.SurfaceFocusSpan{
		BrowserFocusSpan: model.BrowserFocusSpan{Version: 1, TabID: o.TabID, WindowID: o.WindowID, Pointer: o.Pointer,
			StartedAt: started, EndedAt: ended, DurationSeconds: ended.Sub(started).Seconds()},
		Profile: o.Profile, Epoch: p.Epoch, Connection: p.Connection, Sequence: p.LastSequence, SurfaceID: o.SurfaceID,
	}
	payload, _ := json.Marshal(f)
	return Event{ID: 2, Version: 1, TS: ended, Subject: "surface", Verb: "focused", Actor: "observer:browser",
		EntityID: f.SurfaceID, CommandID: "browser-" + f.Profile + "-" + model.NewID(), Payload: payload}
}

func focusState(t *testing.T) (model.State, model.BrowserSurfaceObservation, time.Time) {
	t.Helper()
	st, observed, o := surfaceFixture()
	if err := Apply(&st, observed); err != nil {
		t.Fatal(err)
	}
	ended := o.ObservedAt.Add(5 * time.Second)
	p := st.Browsers[o.Profile]
	p.LastSequence = 2
	p.LastObservedAt, p.ReceivedAt = ended, ended
	p.Complete, p.FocusedWindow = true, -1
	st.Browsers[o.Profile] = p
	return st, o, ended
}

func TestSurfaceFocusRequiresObservedConnection(t *testing.T) {
	t.Run("same connection", func(t *testing.T) {
		st, o, ended := focusState(t)
		if got := st.SurfaceContainers[o.ContainerKey()].ObservedConnection; got != st.Browsers[o.Profile].Connection {
			t.Fatal("container did not retain observing connection", got)
		}
		if err := Apply(&st, focusEvent(st, o, o.ObservedAt, ended)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("reconnect without a new observation", func(t *testing.T) {
		st, o, ended := focusState(t)
		p := st.Browsers[o.Profile]
		p.Connection = model.NewID()
		st.Browsers[o.Profile] = p
		before := model.Clone(st)
		if err := Apply(&st, focusEvent(st, o, o.ObservedAt, ended)); err == nil {
			t.Fatal("focus span accepted stale pre-reconnect container")
		}
		if !reflect.DeepEqual(st, before) {
			t.Fatal("rejected focus span mutated state")
		}
	})
}

func TestSurfaceFocusRejectsOverlapAcrossConnectionAndEpoch(t *testing.T) {
	st, o, ended := focusState(t)
	previous := model.SurfaceFocusSpan{Profile: o.Profile, Epoch: model.NewID(), Connection: model.NewID(),
		BrowserFocusSpan: model.BrowserFocusSpan{EndedAt: ended}}
	st.SurfaceFocusSpans[o.Profile] = previous
	p := st.Browsers[o.Profile]
	p.Epoch, p.Connection = model.NewID(), model.NewID()
	end := ended.Add(time.Minute)
	p.LastObservedAt, p.ReceivedAt = end, end
	st.Browsers[o.Profile] = p
	key := (model.BrowserSurfaceObservation{Profile: o.Profile, Epoch: p.Epoch, TabID: o.TabID}).ContainerKey()
	st.SurfaceContainers[key] = model.ObservedSurfaceContainer{Observation: model.BrowserSurfaceObservation{
		Version: 1, Profile: o.Profile, Epoch: p.Epoch, Sequence: 1, TabID: o.TabID, WindowID: o.WindowID,
		SurfaceID: o.SurfaceID, Pointer: o.Pointer, Title: o.Title, ObservedAt: o.ObservedAt,
	}, ObservedConnection: p.Connection, Present: true}
	before := model.Clone(st)
	if err := Apply(&st, focusEvent(st, o, ended.Add(-time.Minute), end)); err == nil {
		t.Fatal("overlapping span accepted across connection and epoch")
	}
	if !reflect.DeepEqual(st, before) {
		t.Fatal("rejected overlapping span mutated state")
	}
}
