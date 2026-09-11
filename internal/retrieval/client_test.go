package retrieval

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"heimdall/internal/model"
)

func TestClientPublishQuery(t *testing.T) {
	if _, err := exec.LookPath("braid"); err != nil {
		t.Skip("braid executable not installed")
	}
	dir := t.TempDir()
	c := &Client{Dir: dir}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	if err := c.Open(ctx); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Hello["protocol_version"].(float64) != 1 {
		t.Fatalf("unnegotiated hello: %v", c.Hello)
	}

	st := model.Empty()
	st.Tasks["alpha"] = model.TaskRecord{Task: model.Task{ID: "alpha", Title: "ship the TUI", Status: "active", NextAction: "wire sensors"}, Revision: 1, UpdatedAt: time.Now().UTC()}
	st.Captures["cap1"] = model.Capture{ID: "cap1", Client: "cli", Pointer: "https://example.test", Title: "TUI notes", Targets: []string{"alpha"}, Kind: "note", Why: "layout reference", CreatedAt: time.Now().UTC()}
	if _, err := c.Publish(ctx, st); err != nil {
		t.Fatal(err)
	}
	res, err := c.Query(ctx, "TUI sensors", 5)
	if err != nil {
		t.Fatal(err)
	}
	items, _ := res["items"].([]any)
	if len(items) == 0 {
		t.Fatal("query returned no candidates")
	}
	first, _ := items[0].(map[string]any)
	if first["id"] != "task:alpha" {
		t.Fatalf("expected task:alpha on top, got %v", first["id"])
	}
	if _, ok := first["why"]; !ok {
		t.Fatal("hit lacks why provenance")
	}
}
