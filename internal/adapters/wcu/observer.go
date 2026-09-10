// Package wcu consults an explicitly configured read-only Desktop Observer.
// It has no input methods. Raw responses are bounded, hashed, then discarded.
package wcu

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const MaxBytes = 2 << 20

var requestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Config struct {
	Argv       []string `json:"argv"`
	Revision   string   `json:"revision"`
	TraceDir   string   `json:"trace_dir,omitempty"`
	ReportsDir string   `json:"reports_dir,omitempty"`
}

func Load(dir string) (*Adapter, error) {
	f, err := os.Open(filepath.Join(dir, "wcu-observer.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(raw) > 8192 {
		return nil, fmt.Errorf("bounded WCU observer configuration required")
	}
	var c Config
	if err = model.StrictJSON(raw, &c); err != nil {
		return nil, err
	}
	if c.TraceDir == "" {
		c.TraceDir = os.Getenv("WAYLAND_CU_TRACE_DIR")
	}
	if c.TraceDir != "" && !filepath.IsAbs(c.TraceDir) {
		return nil, fmt.Errorf("WCU trace directory must be absolute")
	}
	if len(c.Argv) < 1 || len(c.Argv) > 16 || !filepath.IsAbs(c.Argv[0]) || !model.TokenHashPattern.MatchString(c.Revision) || (c.ReportsDir != "" && !filepath.IsAbs(c.ReportsDir)) {
		return nil, fmt.Errorf("WCU requires absolute argv and pinned revision digest")
	}
	for _, arg := range c.Argv {
		if len(arg) > 4096 {
			return nil, fmt.Errorf("WCU argument too long")
		}
	}
	return &Adapter{Config: c}, nil
}

type Adapter struct{ Config Config }
type limitedReader struct {
	io.Reader
	io.Closer
}

func (a *Adapter) Observe(ctx context.Context, w model.DesktopWindow, action model.ActionRecord) (evidence *model.ExternalObservation, resultErr error) {
	parent := ctx
	defer func() {
		if resultErr != nil && evidence == nil {
			evidence = &model.ExternalObservation{Source: "wcu", Revision: a.Config.Revision + ":unavailable", Partial: true, Digest: model.ContentDigest("observer_unavailable")}
		}
		if evidence != nil {
			bounded, cancel := context.WithTimeout(parent, 250*time.Millisecond)
			defer cancel()
			a.corroborate(bounded, evidence, w, action)
		}
	}()

	// No process or observer lease survives a single bounded verification.
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.Config.Argv[0], a.Config.Argv[1:]...)
	if err := configureProcess(cmd); err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 100 * time.Millisecond
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	defer func() { stdin.Close(); killProcess(cmd); cmd.Wait() }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "heimdall-observer", Version: "1"}, nil).Connect(ctx, &mcp.IOTransport{Reader: limitedReader{io.LimitReader(stdout, MaxBytes), stdout}, Writer: stdin}, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "observe", Arguments: map[string]any{"window": w.Address, "images": false, "channels": []string{"metadata", "accessibility"}, "max_age_ms": 0}})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("WCU observation unavailable")
	}
	var raw []byte
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			if len(raw) > 0 {
				return nil, fmt.Errorf("ambiguous WCU observation")
			}
			raw = []byte(c.Text)
		default:
			return nil, fmt.Errorf("WCU returned non-text evidence")
		}
	}
	evidence, err = Normalize(raw, w, a.Config.Revision)
	if err != nil {
		return nil, err
	}

	return evidence, nil
}
func Normalize(raw []byte, w model.DesktopWindow, pin string) (*model.ExternalObservation, error) {
	if len(raw) == 0 || len(raw) > MaxBytes {
		return nil, fmt.Errorf("bounded WCU observation required")
	}
	var v struct {
		Revision  string `json:"revision"`
		Freshness bool   `json:"freshness_satisfied"`
		Status    string `json:"status"`
		State     struct {
			Windows []struct {
				Address string `json:"address"`
				PID     int    `json:"pid"`
			} `json:"windows"`
			Accessibility struct {
				Complete *bool  `json:"complete"`
				Status   string `json:"status"`
			} `json:"accessibility"`
		} `json:"state"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	if v.Revision == "" || len(v.Revision) > 128 {
		return nil, fmt.Errorf("WCU revision missing")
	}
	exact := false
	for _, window := range v.State.Windows {
		if window.Address == w.Address && window.PID == w.PID {
			exact = true
		}
	}
	return &model.ExternalObservation{Source: "wcu", Revision: pin + ":" + v.Revision, Freshness: v.Freshness && v.Status == "observed" && exact, Partial: !exact || v.State.Accessibility.Status != "available" || (v.State.Accessibility.Complete != nil && !*v.State.Accessibility.Complete), Digest: model.ContentDigest(json.RawMessage(raw))}, nil
}

type reportSummary struct {
	RequestID      string `json:"request_id"`
	TargetMismatch bool   `json:"target_mismatch"`
	Steps          int    `json:"steps"`
	Completed      int    `json:"completed"`
	Digest         string `json:"digest"`
}

func readReport(dir, id string, w model.DesktopWindow) (reportSummary, bool) {
	result := reportSummary{RequestID: id}
	// os.Root prevents traversal, including symlinks escaping the selected root.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return result, false
	}
	defer root.Close()
	f, err := openReport(root, id+".json")
	if err != nil {
		return result, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return result, false
	}
	raw, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
	if err != nil || len(raw) > 256*1024 {
		return result, false
	}
	var v struct {
		RequestID string `json:"request_id"`
		Target    struct {
			Address string `json:"address"`
			PID     int    `json:"pid"`
		} `json:"target_window"`
		Sequence struct {
			Steps     int `json:"steps_total"`
			Completed int `json:"steps_completed"`
		} `json:"sequence"`
	}
	if json.Unmarshal(raw, &v) != nil || v.RequestID != id || v.Target.Address == "" || v.Sequence.Steps < 1 || v.Sequence.Steps > 64 || v.Sequence.Completed < 0 || v.Sequence.Completed > v.Sequence.Steps {
		return result, false
	}
	result.TargetMismatch = v.Target.Address != w.Address || (v.Target.PID != 0 && v.Target.PID != w.PID)
	result.Steps, result.Completed = v.Sequence.Steps, v.Sequence.Completed
	result.Digest = model.ContentDigest(json.RawMessage(raw))
	return result, true
}

func (a *Adapter) corroborate(ctx context.Context, evidence *model.ExternalObservation, w model.DesktopWindow, action model.ActionRecord) {
	// Only request-local results explicitly saved in this selected directory are
	// consulted. Never scan arbitrary trace paths or infer ownership from them.
	if a.Config.ReportsDir != "" {
		summaries := []reportSummary{}
		for _, report := range action.Reports {
			if ctx.Err() != nil {
				break
			}
			if !requestID.MatchString(report.WCURequestID) {
				continue
			}
			summary, ok := readReport(a.Config.ReportsDir, report.WCURequestID, w)
			if ok {
				summaries = append(summaries, summary)
				evidence.TargetMismatch = evidence.TargetMismatch || summary.TargetMismatch
			}
		}
		if len(summaries) > 0 {
			evidence.ReportsDigest = model.ContentDigest(summaries)
			evidence.ReportCount = len(summaries)
		}
	}
	a.traces(ctx, evidence, action)
}
