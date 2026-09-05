package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"testing"
	"time"
)

func TestBrowserLifecycleReplayAndGuards(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := Service{st}
	ctx := context.Background()
	now := time.Now().UTC()
	counter := 0
	next := func() string { counter++; return fmt.Sprintf("%032x", counter) }
	profile, epoch, connection := next(), next(), next()
	message := func(kind string) Message {
		return Message{V: 1, Type: kind, ID: next(), Profile: profile, Epoch: epoch, Connection: connection}
	}
	send := func(m Message) Reply {
		t.Helper()
		b, e := s.Handle(ctx, m, now)
		if e != nil {
			t.Fatal(e)
		}
		var r Reply
		if e = json.Unmarshal(b, &r); e != nil {
			t.Fatal(e)
		}
		return r
	}
	hello := message("hello")
	hello.ExtensionVersion = "0.2.0"
	if send(hello).Paired {
		t.Fatal("auto paired")
	}
	complete, focus := true, 1
	inv := message("inventory")
	inv.Sequence = 1
	inv.ObservedAt = now.Format(time.RFC3339Nano)
	inv.Complete = &complete
	inv.FocusedWindow = &focus
	inv.Tabs = []model.BrowserTab{{ID: 1, WindowID: 1, URL: "https://example.test/", Title: "one"}}
	send(inv)
	state, _ := st.State(ctx)
	if len(state.Browsers[profile].Tabs) != 0 {
		t.Fatal("unpaired metadata retained")
	}
	control := func(c Control) {
		t.Helper()
		c.ID = next()
		c.Profile = profile
		if _, e := s.Control(ctx, c, now); e != nil {
			t.Fatal(e)
		}
	}
	control(Control{Action: "pair"})
	inv.ID = next()
	send(inv)
	before, _ := st.State(ctx)
	send(inv)
	after, _ := st.State(ctx)
	if before.LastEventID != after.LastEventID {
		t.Fatal("retry duplicated event")
	}
	inv.ID = next()
	if _, e := s.Handle(ctx, inv, now); e == nil {
		t.Fatal("stale sequence accepted")
	}
	if _, e := s.Control(ctx, Control{ID: next(), Profile: profile, Epoch: epoch, Action: "close", TabID: 1, ExpectedURL: "https://example.test/"}, now); e == nil {
		t.Fatal("unowned tab control accepted")
	}
	openID := next()
	if _, e := s.Control(ctx, Control{ID: openID, Profile: profile, Epoch: epoch, Action: "open", URL: "https://example.test/"}, now); e != nil {
		t.Fatal(e)
	}
	if got := send(message("poll")); len(got.Commands) != 1 {
		t.Fatal(got)
	}
	now = now.Add(31 * time.Second)
	if got := send(message("poll")); len(got.Commands) != 0 {
		t.Fatal("expired command delivered")
	}
	expired, _ := st.State(ctx)
	if expired.BrowserOperations[openID].Status != "expired" {
		t.Fatal("deadline not recorded")
	}
	result := message("command_result")
	result.Result = &OperationResult{OperationID: openID, Status: "succeeded", TabID: 2, WindowID: 2, URL: "https://example.test/"}
	send(result)
	late, _ := st.State(ctx)
	if late.BrowserOperations[openID].Status != "succeeded" || late.BrowserOperations[openID].Detail == "" {
		t.Fatal("late acknowledged result lost")
	}
	inv.ID = next()
	inv.Sequence = 2
	inv.Tabs = append(inv.Tabs, model.BrowserTab{ID: 2, WindowID: 2, URL: "https://example.test/"})
	send(inv)
	control(Control{Action: "close", Epoch: epoch, TabID: 2, ExpectedURL: "https://example.test/"})
	control(Control{Action: "unpair"})
	control(Control{Action: "pair"})
	if len(send(message("poll")).Commands) != 0 {
		t.Fatal("unpair left a pending operation")
	}
	stale := message("poll")
	connection = next()
	hello = message("hello")
	hello.ExtensionVersion = "0.2.0"
	send(hello)
	if _, e := s.Handle(ctx, stale, now); e == nil {
		t.Fatal("stale connection accepted")
	}
	epoch = next()
	hello = message("hello")
	hello.ExtensionVersion = "0.2.0"
	send(hello)
	state, _ = st.State(ctx)
	if len(state.Browsers[profile].Tabs) != 0 || state.Browsers[profile].LastSequence != 0 {
		t.Fatal("epoch did not reset instance identity")
	}
	b, _ := json.Marshal(state)
	replayed, e := st.Replay(ctx)
	a, _ := json.Marshal(replayed)
	if e != nil || string(a) != string(b) {
		t.Fatalf("replay mismatch: %v", e)
	}
}
func TestProtocolRejectsInvalidPayloads(t *testing.T) {
	for _, body := range []string{`{}`, `{"v":1,"type":"hello","actor":"cli"}`, `{} {}`, string(make([]byte, MaxFrame+1))} {
		if _, e := Decode([]byte(body)); e == nil {
			t.Fatal("invalid payload accepted")
		}
	}
	for _, u := range []string{"file:///tmp/a", "javascript:alert(1)", "https://user:pass@example.com"} {
		if ValidURL(u) {
			t.Fatal(u)
		}
	}
}
