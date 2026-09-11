// Package retrieval supervises a pinned `braid serve --stdio` child against a
// Heimdall-owned dataset. Protocol v1 negotiation is mandatory; all requests
// are sequential with bounded reads and deadlines.
package retrieval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxLine = 16 << 20

// The dataset scope is Heimdall-local; one dataset per database.
const datasetID = "heimdall:local"

const configYAML = `version: 1
dataset_id: "heimdall:local"
store: {backend: sqlite, path: %q}
embed: {provider: none}
index:
  nodes:
    task: [text]
    capture: [text]
    conversation: [text]
    surface: [text]
  cost_field: tokens
graph:
  damping: 0.85
  seed: uniform
  max_hops: 3
  undirected: [targets, bound]
temporal: {half_life_hours: 72}
defaults:
  weights: {lexical: 1.4, dense: 0, graph: 1.0, temporal: 0.5}
  k: {lexical: 60, dense: 60, graph: 60, temporal: 60}
  mmr_lambda: 0.7
  budget: {max: 4000}
`

type Client struct {
	Bin string // braid executable; empty resolves via PATH then ~/.local/bin
	Dir string // heimdall data dir; braid/ is created beneath it

	cmd    *exec.Cmd
	in     io.Writer
	lines  *bufio.Reader
	seq    int
	neg    bool
	Hello  map[string]any
	stderr bytes.Buffer
}

// findBinary resolves the pinned braid executable.
func (c *Client) findBinary() (string, error) {
	if c.Bin != "" {
		return c.Bin, nil
	}
	if p, err := exec.LookPath("braid"); err == nil {
		return p, nil
	}
	if p := filepath.Join(os.Getenv("HOME"), ".local", "bin", "braid"); fileExists(p) {
		return p, nil
	}
	return "", fmt.Errorf("braid executable not found; install it or set BRAID_BIN")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Open launches the child, writes the dataset config and negotiates protocol 1.
func (c *Client) Open(ctx context.Context) error {
	bin, err := c.findBinary()
	if err != nil {
		return err
	}
	dir := filepath.Join(c.Dir, "braid")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "braid.yaml")
	cfg := fmt.Sprintf(configYAML, filepath.Join(dir, "heimdall.db"))
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		return err
	}
	c.cmd = exec.CommandContext(ctx, bin, "serve", "--stdio", "--config", cfgPath)
	c.cmd.Dir = dir
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	c.cmd.Stderr = &c.stderr
	if err := c.cmd.Start(); err != nil {
		return err
	}
	c.in = stdin
	c.lines = bufio.NewReaderSize(stdout, 1<<20)
	res, err := c.call(ctx, "hello", map[string]any{
		"versions":              []int{1},
		"required_capabilities": []string{"query", "apply", "replace_snapshot", "dataset", "strict_requests", "structured_errors", "dataset_binding"},
	})
	if err != nil {
		c.Close()
		return fmt.Errorf("braid negotiation failed: %w (stderr: %s)", err, strings.TrimSpace(c.stderr.String()))
	}
	c.neg = true
	c.Hello = res
	return nil
}

func (c *Client) Close() error {
	if c.cmd == nil {
		return nil
	}
	if c.in != nil {
		if w, ok := c.in.(io.Closer); ok {
			w.Close()
		}
	}
	err := c.cmd.Wait()
	c.cmd = nil
	if err != nil && c.stderr.Len() > 0 {
		return fmt.Errorf("braid exited: %w: %s", err, strings.TrimSpace(c.stderr.String()))
	}
	return err
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *wireError) Error() string { return e.Code + ": " + e.Message }

// call sends one framed request and waits for the matching response.
func (c *Client) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	if c.cmd == nil {
		return nil, fmt.Errorf("braid not running")
	}
	c.seq++
	id := fmt.Sprintf("r%d", c.seq)
	body := map[string]any{"id": id, "method": method}
	for k, v := range params {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxLine {
		return nil, fmt.Errorf("request exceeds protocol limit")
	}
	if _, err := c.in.Write(append(raw, '\n')); err != nil {
		return nil, fmt.Errorf("braid write: %w", err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	type result struct {
		line []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := c.lines.ReadBytes('\n')
		ch <- result{line, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Until(deadline)):
		return nil, fmt.Errorf("braid response timeout")
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("braid transport: %w", r.err)
		}
		var resp struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Err    *wireError      `json:"error"`
		}
		if err := json.Unmarshal(r.line, &resp); err != nil {
			return nil, fmt.Errorf("invalid braid response: %w", err)
		}
		if resp.Err != nil {
			return nil, resp.Err
		}
		var out map[string]any
		if len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, &out); err != nil {
				return nil, fmt.Errorf("invalid braid result: %w", err)
			}
		}
		return out, nil
	}
}

// Dataset reads the bound dataset identity and current revision.
func (c *Client) Dataset(ctx context.Context) (map[string]any, error) {
	return c.call(ctx, "dataset", nil)
}
