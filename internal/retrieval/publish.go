package retrieval

import (
	"context"
	"heimdall/internal/model"
	"sort"
	"strings"
)

// Node/edge wire shapes (braid Batch).
type node struct {
	ID    string         `json:"id"`
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	TS    string         `json:"ts"`
	Attrs map[string]any `json:"attrs,omitempty"`
}
type edge struct {
	Src    string  `json:"src"`
	Dst    string  `json:"dst"`
	Type   string  `json:"type"`
	Weight float64 `json:"weight"`
	TS     string  `json:"ts"`
}

// Snapshot projects permitted, nonpurged Heimdall state into a Braid batch.
// Conversation text is metadata only (kind + project) — description prose is
// purgeable evidence and never enters the index.
func Snapshot(st model.State) map[string]any {
	nodes := []node{}
	edges := []edge{}
	for id, t := range st.Tasks {
		text := strings.TrimSpace(t.Task.Title + " " + t.Task.NextAction)
		nodes = append(nodes, node{ID: "task:" + id, Type: "task", Text: text,
			TS:    t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Attrs: map[string]any{"status": t.Task.Status, "revision": t.Revision}})
	}
	for id, c := range st.Captures {
		if c.Expired {
			continue
		}
		text := strings.TrimSpace(c.Title + " " + c.Why + " " + c.Pointer)
		nodes = append(nodes, node{ID: "capture:" + id, Type: "capture", Text: text,
			TS: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), Attrs: map[string]any{"kind": c.Kind}})
		for _, target := range c.Targets {
			if _, ok := st.Tasks[target]; ok {
				edges = append(edges, edge{Src: "capture:" + id, Dst: "task:" + target, Type: "targets", Weight: 1,
					TS: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")})
			}
		}
	}
	for id, c := range st.Conversations {
		text := strings.TrimSpace(c.Kind + " " + c.NativeConversationID + " " + projectOf(c))
		nodes = append(nodes, node{ID: "conversation:" + id, Type: "conversation", Text: text,
			TS: c.StartedAt.UTC().Format("2006-01-02T15:04:05Z"), Attrs: map[string]any{"kind": c.Kind}})
		if c.Task != nil {
			if _, ok := st.Tasks[c.Task.Target]; ok {
				edges = append(edges, edge{Src: "conversation:" + id, Dst: "task:" + c.Task.Target, Type: "bound", Weight: 1,
					TS: c.StartedAt.UTC().Format("2006-01-02T15:04:05Z")})
			}
		}
	}
	for id, s := range st.ObservedSurfaces {
		nodes = append(nodes, node{ID: "surface:" + id, Type: "surface", Text: s.NormalizedPointer,
			TS: s.FirstRecordedAt.UTC().Format("2006-01-02T15:04:05Z"), Attrs: map[string]any{"kind": s.Kind}})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Src != edges[j].Src {
			return edges[i].Src < edges[j].Src
		}
		return edges[i].Dst < edges[j].Dst
	})
	return map[string]any{"nodes": nodes, "edges": edges, "embeddings": []any{}}
}

func projectOf(c model.Conversation) string {
	if c.TranscriptRef != nil {
		return c.TranscriptRef.Locator
	}
	return ""
}

// Publish replaces the dataset snapshot with the current permitted projection.
func (c *Client) Publish(ctx context.Context, st model.State) (map[string]any, error) {
	ds, err := c.Dataset(ctx)
	if err != nil {
		return nil, err
	}
	revision := int64(0)
	if r, ok := ds["revision"].(float64); ok {
		revision = int64(r)
	}
	return c.call(ctx, "replace_snapshot", map[string]any{"snapshot": map[string]any{
		"dataset_id":        datasetID,
		"expected_revision": revision,
		"batch":             Snapshot(st),
	}})
}

// Query asks for candidate assignments with full why provenance.
func (c *Client) Query(ctx context.Context, text string, limit int) (map[string]any, error) {
	return c.call(ctx, "query", map[string]any{"query": map[string]any{
		"dataset_id": datasetID,
		"text":       text,
		"explain":    true,
		"filters":    map[string]any{"types": []string{"task", "capture"}},
	}, "timeout_ms": 8000})
}
