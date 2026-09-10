package continuity

import (
	"bytes"
	"encoding/json"
	"heimdall/internal/conversation"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConversationRecapHasNoAcceptedAuthority(t *testing.T) {
	f := setup(t)
	path := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := f.e.Store.State(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	task := st.Tasks[f.target].Task
	task.Done.Checks = []model.Check{{ID: "exists", Kind: "artifact.exists", Path: path}}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	st, _ = f.e.Store.State(f.ctx)
	f.rev = st.Tasks[f.target].Revision
	contract := st.ContractHeads[f.target]
	if contract == "" || len(st.EvaluatorHeads) == 0 {
		t.Fatal("missing accepted contract/evaluator fixture")
	}
	decision := f.request("decision.accept")
	decision.Decision = &DecisionInput{Text: "Keep task authority explicit"}
	f.send(decision)
	cp := f.checkpoint(contract, "none")
	f.send(cp)
	// Seed a real authorized verification record so the authority comparison is
	// non-vacuous for contracts, decisions, checkpoints and verification evidence.
	_, err = f.e.Store.Transact(f.ctx, model.NewID(), "cli", []byte("synthetic verification"), f.now, func(st model.State) (store.Change, error) {
		lineage, err := model.EvidenceContext(st, f.target)
		if err != nil {
			return store.Change{}, err
		}
		v := model.Evidence{Version: 1, ID: model.NewID(), EvaluatorID: st.EvaluatorHeads[model.EvaluatorKey(f.target, "exists")], Target: f.target, TaskRevision: f.rev, SourceEvent: st.LastEventID, StartedAt: f.now, Status: "started", Outcome: "unknown", Observer: "daemon", EvaluatorVersion: "1", Inputs: []model.ResourceVersion{}, Context: lineage, DecisionDigest: model.EvidenceDecisionDigest(st, f.target)}
		return store.Change{Revision: st.Revision, Events: []store.Pending{{Subject: "evidence", Verb: "started", EntityID: v.ID, Payload: v}}, Result: map[string]string{"id": v.ID}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	authority := func(st model.State) []byte {
		st.Conversations = map[string]model.Conversation{}
		st.LastEventID = 0
		b, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	before, _ := f.e.Store.State(f.ctx)
	beforeBytes := authority(before)
	source := conversation.SourceKey{AdapterID: "synthetic", ContractMajor: 1, Namespace: "fixture:authority", LogicalStream: "primary"}
	at := f.now.Add(time.Second)
	start := conversation.Started{Version: 1, Kind: "codex", Source: source, NativeConversationID: "native-authority", Task: &conversation.TaskRef{Target: f.target, Revision: f.rev}, StartedAt: at, AdapterVersion: "fixture-v1", ContractVersion: conversation.ContractVersion{Major: 1}, Observation: conversation.Observation{RecordKey: "start", SourceRevision: conversation.SourceRevision([]byte("start")), Sequence: 1, SourceTime: at, ObservedAt: at}}
	raw, err := f.e.Store.RecordConversationStarted(f.ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	var receipt store.ConversationReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile("../../testdata/conversation/recap.txt")
	if err != nil {
		t.Fatal(err)
	}
	at = at.Add(time.Second)
	d := conversation.Description{Version: 1, ConversationID: receipt.ConversationID, Source: source, Kind: "recap", AdapterVersion: "fixture-v1", ContractVersion: conversation.ContractVersion{Major: 1}, Provenance: conversation.Provenance{Kind: "native", Producer: "fixture"}, Coverage: conversation.Coverage{Kind: "unknown"}, Availability: "available", Observation: conversation.Observation{RecordKey: "recap", SourceRevision: conversation.SourceRevision(text), Sequence: 2, SourceTime: at, ObservedAt: at}}
	if _, err := f.e.Store.RecordDescription(f.ctx, d, text); err != nil {
		t.Fatal(err)
	}
	after, _ := f.e.Store.State(f.ctx)
	if !bytes.Equal(beforeBytes, authority(after)) {
		t.Fatal("recap changed accepted authority")
	}
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !bytes.Equal(beforeBytes, authority(replayed)) {
		t.Fatal("replay changed accepted authority", err)
	}
	views, err := f.e.Store.Conversations(f.ctx, f.target, at)
	if err != nil || len(views) != 1 || views[0].Task.Revision != f.rev {
		t.Fatal("explicit task filter", views, err)
	}
	stale := start
	stale.NativeConversationID = "stale"
	stale.Task = &conversation.TaskRef{Target: f.target, Revision: f.rev + 1}
	if _, err := f.e.Store.RecordConversationStarted(f.ctx, stale); err == nil {
		t.Fatal("stale task binding accepted")
	}
	next := f.checkpoint(contract, cp.ID)
	f.send(next)
	final, _ := f.e.Store.State(f.ctx)
	if final.CheckpointHeads[f.target] != next.ID || final.Tasks[f.target].Task.Status != before.Tasks[f.target].Task.Status {
		t.Fatal("authorized checkpoint path broken")
	}
}
