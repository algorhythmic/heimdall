package model

import (
	"os"
	"testing"
)

func TestFixtures(t *testing.T) {
	b, _ := os.ReadFile("../../testdata/types.yaml")
	c, err := ParseCatalog(b)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile("../../testdata/tasks.yaml")
	d, err := ParseDocument(b)
	if err != nil {
		t.Fatal(err)
	}
	for i := range d.Tasks {
		Defaults(&d.Tasks[i], c.Types[d.Tasks[i].Type])
	}
	if err = Validate(d, c); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Document)
	}{
		{"parent cycle", func(x *Document) { x.Tasks[0].Parent = x.Tasks[1].ID }},
		{"bad date", func(x *Document) { x.Tasks[1].ResumeBy = "2026-02-30" }},
		{"step cycle", func(x *Document) { x.Tasks[2].Subtasks[0].After = []string{"tasks"} }},
		{"unknown check", func(x *Document) { x.Tasks[0].Done.Checks[0].Kind = "silense" }},
		{"missing mode", func(x *Document) { x.Tasks[1].Done.Mode = "" }},
		{"bad anchor", func(x *Document) { x.Tasks[1].Done.Checks[1].After = "job-search#send" }},
		{"invalid importance", func(x *Document) { x.Tasks[0].Importance = Int(0) }},
		{"manual with parameters", func(x *Document) { x.Tasks[2].Subtasks[0].Done.Checks[0].Days = 4 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := Clone(d)
			tc.mutate(&x)
			if Validate(x, c) == nil {
				t.Fatal("accepted invalid document")
			}
		})
	}
	for _, s := range []string{"version: 1\nversion: 1\nrevision: 0\ntasks: []", "version: 1\nrevision: 0\ntasks: []\nunknown: 2", "version: 1\nrevision: 0\ntasks: []\n---\nx: 2"} {
		if _, err := ParseDocument([]byte(s)); err == nil {
			t.Fatal("accepted malformed YAML")
		}
	}
}
