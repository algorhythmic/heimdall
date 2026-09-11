package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// usageLimit is one allowance window from an omarchy agent-usage record.
// Percent is a 0..1 fraction of the window already consumed.
type usageLimit struct {
	Label    string  `json:"label"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resetsAt"`
}

// usageRecord mirrors the JSON files omarchy-agent-usage-update writes under
// $XDG_STATE_HOME/omarchy/agents/usage/. Heimdall reads them as external
// observations only; collectors stay owned by the omarchy agents plugin.
type usageRecord struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Ready     bool         `json:"ready"`
	Limits    []usageLimit `json:"limits"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// readAgentUsage loads the recorded usage observations, skipping malformed or
// oversized files rather than failing the snapshot.
func readAgentUsage() []usageRecord {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		state = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(state, "omarchy", "agents", "usage")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := []usageRecord{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || len(raw) > 1<<20 {
			continue
		}
		var r usageRecord
		if json.Unmarshal(raw, &r) != nil || r.ID == "" || !r.Ready {
			continue
		}
		limits := r.Limits[:0]
		for _, l := range r.Limits {
			if l.Percent >= 0 && l.Percent <= 1 {
				limits = append(limits, l)
			}
		}
		r.Limits = limits
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
