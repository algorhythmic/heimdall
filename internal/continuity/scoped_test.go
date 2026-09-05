package continuity

import (
	"errors"
	"heimdall/internal/authz"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestScopedResourceObservationAndContractAcceptance(t *testing.T) {
	f := setup(t)
	f.contract()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "work"), []byte("fixture"), 0600)
	r := f.request("resource.bind")
	r.Resource = &ResourceInput{Kind: "file", Root: root, Path: "work"}
	f.send(r)
	st, _ := f.e.Store.State(f.ctx)
	accept := f.request("contract.accept")
	accept.Contract = &ContractInput{Previous: st.ContractHeads[f.target], Objective: "Reviewed work"}
	if _, err := f.s.Execute(f.ctx, accept, "cli", f.now); err == nil {
		t.Fatal("unreviewed binding automatically accepted")
	}
	accept.Contract.ResourceIDs = []string{r.ID}
	f.send(accept)
	cp := f.checkpoint(accept.ID, "none")
	f.send(cp)
	st, _ = f.e.Store.State(f.ctx)
	g := model.Grant{ID: model.NewID(), Target: f.target}
	// Delete the file: a missing grant must fail before any attempted observation.
	os.Remove(filepath.Join(root, "work"))
	if _, err := ScopedContext(f.ctx, st, g, f.target, 20000); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("ungranted observation", err)
	}
	g.ResourceIDs = []string{r.ID}
	b, err := ScopedContext(f.ctx, st, g, f.target, 20000)
	if err != nil || !hasIssue(b, "resource_unavailable") {
		t.Fatal(b, err)
	}
}
