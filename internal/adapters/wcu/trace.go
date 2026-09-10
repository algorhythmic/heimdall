package wcu

import (
	"bufio"
	"context"
	"encoding/json"
	"heimdall/internal/model"
	"io"
	"os"
	"sort"
	"strings"
)

// Trace IDs are explicit correlations supplied by the caller. These records
// cannot bind a window or verify application acceptance.
func (a *Adapter) traces(ctx context.Context, evidence *model.ExternalObservation, action model.ActionRecord) {
	if a.Config.TraceDir == "" {
		return
	}
	root, err := os.OpenRoot(a.Config.TraceDir)
	if err != nil {
		return
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return
	}
	defer dir.Close()
	names, _ := dir.Readdirnames(256)
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	wanted := map[string]bool{}
	for _, r := range action.Reports {
		if requestID.MatchString(r.WCURequestID) {
			wanted[r.WCURequestID] = true
		}
	}
	type summary struct {
		ID         string
		DurationMS int64
		Submitted  bool
		Digest     string
	}
	found := map[string]summary{}
	files := 0
	for _, name := range names {
		if ctx.Err() != nil || files >= 8 {
			break
		}
		if !strings.HasPrefix(name, "trace-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		f, err := openReport(root, name)
		if err != nil {
			continue
		}
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			f.Close()
			continue
		}
		files++
		scanner := bufio.NewScanner(io.LimitReader(f, 1<<20))
		scanner.Buffer(make([]byte, 4096), 256<<10)
		for scanner.Scan() {
			if ctx.Err() != nil {
				break
			}
			raw := scanner.Bytes()
			var r struct {
				Schema  int
				Kind    string
				ID      string `json:"trace_id"`
				Start   int64  `json:"start_ns"`
				End     int64  `json:"end_ns"`
				Outcome struct {
					Submitted *bool `json:"action_performed"`
				} `json:"outcome"`
			}
			if json.Unmarshal(raw, &r) != nil || r.Schema != 1 || r.Kind != "request" || !wanted[r.ID] || r.Start < 0 || r.End < r.Start || r.End-r.Start > 3600000000000 {
				continue
			}
			if _, ok := found[r.ID]; ok {
				continue
			}
			found[r.ID] = summary{r.ID, (r.End - r.Start) / 1000000, r.Outcome.Submitted != nil && *r.Outcome.Submitted, model.ContentDigest(json.RawMessage(raw))}
		}
		f.Close()
	}
	ids := []string{}
	for id := range found {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	records := []summary{}
	for _, id := range ids {
		r := found[id]
		records = append(records, r)
		evidence.TraceDurationMS += r.DurationMS
		if r.Submitted {
			evidence.TraceSubmitted++
		}
	}
	if len(records) > 0 {
		evidence.TraceCount = len(records)
		evidence.TraceDigest = model.ContentDigest(records)
	}
}
