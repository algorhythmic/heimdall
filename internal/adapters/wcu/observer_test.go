package wcu

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWCUHelper(t *testing.T) {
	if os.Getenv("HEIMDALL_WCU_TEST_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var r struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.Unmarshal(scanner.Bytes(), &r)
		if r.ID == nil {
			continue
		}
		var result any
		switch r.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "fixture-observer", "version": "1"}}
		case "tools/call":
			args := r.Params.Arguments
			if r.Params.Name != "observe" || args["images"] != false || args["max_age_ms"] != float64(0) || model.ContentDigest(args["channels"]) != model.ContentDigest([]string{"metadata", "accessibility"}) {
				os.Exit(3)
			}
			if os.Getenv("HEIMDALL_WCU_HANG") == "1" {
				time.Sleep(time.Minute)
			}
			body := map[string]any{"revision": "runtime:1", "freshness_satisfied": true, "status": "observed", "state": map[string]any{"windows": []any{map[string]any{"address": args["window"], "pid": 123}}, "accessibility": map[string]any{"status": "partial", "nodes": []string{"PRIVATE FIXTURE TEXT"}}}}
			raw, _ := json.Marshal(body)
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": string(raw)}}, "isError": false}
		default:
			result = map[string]any{}
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": r.ID, "result": result})
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func TestObserverProcessBoundsAndPrivateContent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux observer subprocess")
	}
	t.Setenv("HEIMDALL_WCU_TEST_HELPER", "1")
	a := Adapter{Config: Config{Argv: []string{os.Args[0], "-test.run=TestWCUHelper"}, Revision: strings.Repeat("a", 64)}}
	o, err := a.Observe(context.Background(), model.DesktopWindow{Address: "0x123", PID: 123}, model.ActionRecord{})
	if err != nil || !o.Freshness || !o.Partial || o.Revision != a.Config.Revision+":runtime:1" {
		t.Fatal(o, err)
	}
	raw, _ := json.Marshal(o)
	if strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("raw accessibility persisted")
	}
	t.Setenv("HEIMDALL_WCU_HANG", "1")
	started := time.Now()
	if _, err = a.Observe(context.Background(), model.DesktopWindow{Address: "0x123", PID: 123}, model.ActionRecord{}); err == nil {
		t.Fatal("hung observer accepted")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("observer deadline exceeded")
	}
}
func TestWCUReportCorroborationNeverCreatesBinding(t *testing.T) {
	dir := t.TempDir()
	w := model.DesktopWindow{Address: "0x123", PID: 123}
	raw := []byte(`{"request_id":"request-1","target_window":{"address":"0x999","pid":999},"sequence":{"steps_total":2,"steps_completed":1},"images":["PRIVATE"]}`)
	if err := os.WriteFile(filepath.Join(dir, "request-1.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	summary, ok := readReport(dir, "request-1", w)
	if !ok || !summary.TargetMismatch || summary.Completed != 1 {
		t.Fatal(summary)
	}
	body, _ := json.Marshal(summary)
	if strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "0x999") {
		t.Fatal("raw request content retained")
	}
	if _, ok = readReport(dir, "wrong-id", w); ok {
		t.Fatal("unknown request matched")
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	os.WriteFile(outside, raw, 0600)
	os.Symlink(outside, filepath.Join(dir, "escape.json"))
	if _, ok = readReport(dir, "escape", w); ok {
		t.Fatal("escaped selected reports root")
	}
}
func TestObserverMissingPartialAndStaleAreNotCompleteEvidence(t *testing.T) {
	for _, raw := range []string{`{"revision":"a:1","status":"observed","freshness_satisfied":false,"state":{"windows":[{"address":"x","pid":1}],"accessibility":{"status":"available"}}}`, `{"revision":"a:1","status":"observed","freshness_satisfied":true,"state":{"windows":[],"accessibility":{"status":"available"}}}`} {
		o, err := Normalize([]byte(raw), model.DesktopWindow{Address: "x", PID: 1}, strings.Repeat("a", 64))
		if err != nil || o.Freshness {
			t.Fatal(o, err)
		}
	}
}

func TestTraceCorrelationIsBoundedMetadataOnly(t *testing.T) {
	dir := t.TempDir()
	raw := `{"schema":1,"kind":"request","trace_id":"request-1","start_ns":1000000,"end_ns":4000000,"outcome":{"action_performed":true},"private":"DO NOT RETAIN"}`
	os.WriteFile(filepath.Join(dir, "trace-fixture.jsonl"), []byte(raw+"\n"), 0600)
	a := Adapter{Config: Config{TraceDir: dir}}
	evidence := &model.ExternalObservation{}
	action := model.ActionRecord{Reports: []model.ActionReport{{WCURequestID: "request-1"}, {WCURequestID: "request-1"}, {WCURequestID: "absent"}}}
	a.traces(context.Background(), evidence, action)
	if evidence.TraceCount != 1 || evidence.TraceDurationMS != 3 || evidence.TraceSubmitted != 1 || evidence.TraceDigest == "" || evidence.TargetMismatch {
		t.Fatal(evidence)
	}
	body, _ := json.Marshal(evidence)
	if strings.Contains(string(body), "DO NOT") {
		t.Fatal("raw trace retained")
	}
	evidence = &model.ExternalObservation{}
	a.traces(context.Background(), evidence, model.ActionRecord{})
	if evidence.TraceCount != 0 {
		t.Fatal("uncorrelated trace accepted")
	}
}

func TestExplicitIncompleteAccessibility(t *testing.T) {
	o, err := Normalize([]byte(`{"revision":"a:1","status":"observed","freshness_satisfied":true,"state":{"windows":[{"address":"x","pid":1}],"accessibility":{"status":"available","complete":false}}}`), model.DesktopWindow{Address: "x", PID: 1}, strings.Repeat("a", 64))
	if err != nil || !o.Partial {
		t.Fatal(o, err)
	}
}
