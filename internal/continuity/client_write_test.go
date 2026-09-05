package continuity

import (
	"encoding/json"
	"errors"
	"heimdall/internal/authz"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClientCheckpointAuthorityRetryAndReplay(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	clock := func() time.Time { return f.now }
	issue := func(token string, write bool) string {
		id := model.NewID()
		_, err := (authz.Service{Store: f.e.Store}).Execute(f.ctx, authz.Request{Version: 1, ID: id, Op: "grant.issue", Grant: &authz.IssueInput{Name: "Fixture", Target: f.target, ResourceIDs: []string{}, TokenHash: authz.HashToken(token), ExpiresAt: f.now.Add(time.Hour), CheckpointWrite: write}}, f.now)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	reader, writer, other := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	issue(reader, false)
	grant := issue(writer, true)
	issue(other, true)
	r := f.checkpoint(contract, "none")
	if _, err := f.s.ExecuteClient(f.ctx, r, reader, clock); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("reader could write", err)
	}
	result, err := f.s.ExecuteClient(f.ctx, r, writer, clock)
	if err != nil {
		t.Fatal(err)
	}
	var cp model.Checkpoint
	if err = json.Unmarshal(result, &cp); err != nil {
		t.Fatal(err)
	}
	if cp.Version != 2 || cp.Actor != "client:"+grant || cp.GrantID != grant {
		t.Fatal("missing authenticated provenance", cp)
	}
	st, _ := f.e.Store.State(f.ctx)
	originalEvent := st.LastEventID
	retry, err := f.s.ExecuteClient(f.ctx, r, writer, clock)
	if err != nil || !reflect.DeepEqual(result, retry) {
		t.Fatal("retry changed result", err)
	}
	st, _ = f.e.Store.State(f.ctx)
	if st.LastEventID != originalEvent {
		t.Fatal("retry wrote events")
	}
	if _, err = f.s.ExecuteClient(f.ctx, r, other, clock); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("cross-grant cached result exposed", err)
	}
	bad := r
	copyCP := *r.Checkpoint
	copyCP.Summary = "Changed logical request"
	bad.Checkpoint = &copyCP
	if _, err = f.s.ExecuteClient(f.ctx, bad, writer, clock); !errors.Is(err, store.ErrConflict) {
		t.Fatal("changed-body reuse", err)
	}
	stale := f.checkpoint(contract, "none")
	if _, err = f.s.ExecuteClient(f.ctx, stale, writer, clock); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale head accepted", err)
	}
	forbidden := f.request("contract.accept")
	forbidden.Contract = &ContractInput{Previous: contract, Objective: "Escalate authority"}
	if _, err = f.s.ExecuteClient(f.ctx, forbidden, writer, clock); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("writer accepted contract", err)
	}
	out := f.checkpoint(contract, r.ID)
	out.Target = "other-project"
	if _, err = f.s.ExecuteClient(f.ctx, out, writer, clock); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("outside target write", err)
	}
	if _, err = f.s.ExecuteClient(f.ctx, r, writer, func() time.Time { return f.now.Add(2 * time.Hour) }); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("expired retry exposed result", err)
	}
	_, err = (authz.Service{Store: f.e.Store}).Execute(f.ctx, authz.Request{Version: 1, ID: model.NewID(), Op: "grant.revoke", GrantID: grant}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ExecuteClient(f.ctx, r, writer, clock); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("revoked retry exposed cached result", err)
	}
	before, _ := f.e.Store.State(f.ctx)
	after, err := f.e.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("client provenance replay failed", err)
	}
}
