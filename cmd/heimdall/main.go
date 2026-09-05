package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"heimdall/internal/core"
	"heimdall/internal/daemon"
	"heimdall/internal/model"
	"heimdall/internal/nativebridge"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if strings.HasPrefix(filepath.Base(os.Args[0]), "heimdall-browser-host") || (len(os.Args) > 1 && os.Args[1] == "browser-host") {
		args := os.Args[1:]
		if len(args) > 0 && args[0] == "browser-host" {
			args = args[1:]
		}
		if err := nativebridge.Main(ctx, args, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "heimdall:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, now, requestID string
	args                []string
}

func globals(args []string) (options, error) {
	o := options{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, has := strings.Cut(a, "=")
		switch name {
		case "--json":
			continue
		case "--data-dir", "--now", "--request-id":
			if !has {
				i++
				if i >= len(args) {
					return o, fmt.Errorf("%s requires a value", name)
				}
				val = args[i]
			}
			switch name {
			case "--data-dir":
				o.dir = val
			case "--now":
				o.now = val
			case "--request-id":
				o.requestID = val
			}
		default:
			o.args = append(o.args, a)
		}
	}
	if o.dir == "" {
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			o.dir = filepath.Join(d, "heimdall")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return o, err
			}
			o.dir = filepath.Join(home, ".local", "share", "heimdall")
		}
	}
	abs, err := filepath.Abs(o.dir)
	o.dir = abs
	return o, err
}
func nowFunc(s string) (func() time.Time, error) {
	if s == "" {
		return time.Now, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, err
	}
	return func() time.Time { return t }, nil
}
func run(ctx context.Context, args []string, out io.Writer) error {
	o, err := globals(args)
	if err != nil {
		return err
	}
	clock, err := nowFunc(o.now)
	if err != nil {
		return err
	}
	if len(o.args) == 0 {
		return fmt.Errorf("usage: heimdall init|start|doctor|ls|state|add|update|import-tasks|capture|assign|complete|reopen|drop|ratify|checks|tick|sync|fmt|events|replay|browser|contract|decision|resource|checkpoint|context|backup|grant|client|mcp|evidence [--data-dir PATH] [--json]")
	}
	verb := o.args[0]
	rest := o.args[1:]
	if verb == "ui" {
		return uiCLI(ctx, o, rest, out)
	}
	if verb == "evidence" {
		return evidenceCLI(ctx, o, rest, out)
	}
	if verb == "mcp" {
		return mcpCLI(ctx, rest)
	}
	enc := json.NewEncoder(out)
	if verb == "grant" || verb == "client" {
		return grantCLI(ctx, o, verb, rest, out)
	}
	if model.Contains([]string{"contract", "decision", "resource", "checkpoint", "context", "backup"}, verb) {
		return continuityCLI(ctx, o, verb, rest, out)
	}
	if verb == "browser" {
		return browserCLI(ctx, o, rest, out)
	}
	if verb == "init" {
		if len(rest) > 0 {
			return fmt.Errorf("init has no integration-install flags in this core release")
		}
		e, err := core.Open(o.dir)
		if err != nil {
			return err
		}
		defer e.Close()
		return enc.Encode(map[string]any{"data_dir": o.dir, "initialized": true, "daemon_started": false})
	}
	if verb == "start" {
		if len(rest) > 0 {
			return fmt.Errorf("start accepts no service-install flags in this core release")
		}
		return daemon.Serve(ctx, o.dir, clock, func(ep daemon.Endpoint) {
			_ = enc.Encode(map[string]any{"status": "running", "data_dir": o.dir, "pid": ep.PID})
		})
	}
	if verb == "doctor" {
		health, err := call(ctx, o, "GET", "/health", nil)
		if err != nil {
			return fmt.Errorf("daemon unavailable; run heimdall start with this data directory: %w", err)
		}
		_, err = fmt.Fprintln(out, string(health))
		return err
	}
	if verb == "events" || verb == "replay" || verb == "sync" || verb == "fmt" {
		if len(rest) > 0 {
			return fmt.Errorf("unexpected arguments")
		}
		method := "POST"
		var body any = daemon.Request{Now: o.now}
		if verb == "events" {
			method = "GET"
			body = nil
		}
		r, err := call(ctx, o, method, "/"+verb, body)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(r))
		return err
	}
	raw, err := call(ctx, o, "GET", "/state", nil)
	if err != nil {
		return fmt.Errorf("daemon unavailable; run heimdall start: %w", err)
	}
	var state model.State
	if err = json.Unmarshal(raw, &state); err != nil {
		return err
	}
	switch verb {
	case "state":
		if len(rest) == 0 {
			return enc.Encode(state)
		}
		if len(rest) != 1 {
			return fmt.Errorf("state accepts at most one task id")
		}
		r, ok := state.Tasks[rest[0]]
		if !ok {
			return fmt.Errorf("unknown task")
		}
		return enc.Encode(r)
	case "ls":
		if len(rest) > 0 {
			return fmt.Errorf("ls takes no arguments")
		}
		return enc.Encode(state.Document())
	case "checks":
		if len(rest) != 1 {
			return fmt.Errorf("checks requires task or task#step")
		}
		if _, _, err := model.ResolveTarget(state, rest[0]); err != nil {
			return err
		}
		return enc.Encode(core.Evaluate(state, rest[0]))
	case "export-tasks":
		b, err := yaml.Marshal(state.Document())
		if err != nil {
			return err
		}
		_, err = out.Write(b)
		return err
	}
	c := core.Command{ID: o.requestID, Op: verb, ExpectedRevision: &state.Revision}
	if c.ID == "" {
		c.ID = model.NewID()
	}
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	switch verb {
	case "add":
		title, tail, err := positional(rest)
		if err != nil {
			return err
		}
		id := fs.String("id", "", "")
		typ := fs.String("type", "project", "")
		parent := fs.String("parent", "", "")
		status := fs.String("status", "", "")
		importance := fs.Int("importance", 3, "")
		next := fs.String("next-action", "", "")
		if err = fs.Parse(tail); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		c.Task = &model.Task{ID: *id, Title: title, Type: *typ, Parent: *parent, Status: *status, Importance: importance, NextAction: *next}
	case "update":
		id, tail, err := positional(rest)
		if err != nil {
			return err
		}
		r, ok := state.Tasks[id]
		if !ok {
			return fmt.Errorf("unknown task")
		}
		t := r.Task
		title := fs.String("title", t.Title, "")
		status := fs.String("status", t.Status, "")
		next := fs.String("next-action", t.NextAction, "")
		due := fs.String("resume-by", t.ResumeBy, "")
		parent := fs.String("parent", t.Parent, "")
		importance := fs.Int("importance", *t.Importance, "")
		if err = fs.Parse(tail); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		t.Title = *title
		t.Status = *status
		t.NextAction = *next
		t.ResumeBy = *due
		t.Parent = *parent
		t.Importance = importance
		c.Task = &t
	case "import-tasks":
		if len(rest) != 1 {
			return fmt.Errorf("import-tasks requires a YAML file")
		}
		b, err := os.ReadFile(rest[0])
		if err != nil {
			return err
		}
		d, err := model.ParseDocument(b)
		if err != nil {
			return err
		}
		c.Op = "replace"
		c.Document = &d
	case "capture":
		line, tail, err := positional(rest)
		if err != nil {
			return err
		}
		pointer := fs.String("pointer", "", "")
		title := fs.String("title", "", "")
		client := fs.String("client", "cli", "")
		origin := fs.String("origin-id", "", "")
		if err = fs.Parse(tail); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		c.Line = line
		c.Pointer = *pointer
		c.Title = *title
		c.Client = *client
		c.Origin = *origin
	case "assign":
		id, tail, err := positional(rest)
		if err != nil {
			return err
		}
		targets := fs.String("streams", "", "")
		if err = fs.Parse(tail); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		c.Target = id
		c.Targets = strings.Split(*targets, ",")
	case "complete", "reopen", "drop":
		if len(rest) != 1 {
			return fmt.Errorf("%s requires task or task#step", verb)
		}
		c.Target = rest[0]
	case "ratify":
		if len(rest) == 0 {
			ps := []model.Proposal{}
			for _, p := range state.Proposals {
				if p.Status == "pending" {
					ps = append(ps, p)
				}
			}
			sort.Slice(ps, func(i, j int) bool { return ps[i].ID < ps[j].ID })
			return enc.Encode(ps)
		}
		if len(rest) != 2 || !model.Contains([]string{"--accept", "--reject"}, rest[1]) {
			return fmt.Errorf("ratify ID --accept|--reject")
		}
		c.Target = rest[0]
		c.Action = strings.TrimPrefix(rest[1], "--")
	case "tick":
		if len(rest) != 0 {
			return fmt.Errorf("tick takes no arguments")
		}
	default:
		return fmt.Errorf("unsupported command %s in the core release", verb)
	}
	// Revision preconditions apply on first execution; an identical logical request
	// with the same ID returns its saved result even when the document has advanced.
	result, err := call(ctx, o, "POST", "/commands", daemon.Request{Command: c, Now: o.now})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}
func positional(xs []string) (string, []string, error) {
	if len(xs) == 0 || strings.HasPrefix(xs[0], "--") {
		return "", nil, fmt.Errorf("positional title/target must come before command-specific flags")
	}
	return xs[0], xs[1:], nil
}
func call(ctx context.Context, o options, method, path string, body any) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(o.dir, "endpoint.json"))
	if err != nil {
		return nil, err
	}
	var ep daemon.Endpoint
	if err = json.Unmarshal(b, &ep); err != nil {
		return nil, err
	}
	return callEndpoint(ctx, ep, method, path, body)
}
func callEndpoint(ctx context.Context, ep daemon.Endpoint, method, path string, body any) ([]byte, error) {
	// A stale or edited rendezvous file must never redirect the bearer token off loopback.
	if !strings.HasPrefix(ep.URL, "http://127.0.0.1:") {
		return nil, fmt.Errorf("invalid daemon endpoint")
	}
	port := strings.TrimPrefix(ep.URL, "http://127.0.0.1:")
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || len(ep.Token) != 64 {
		return nil, fmt.Errorf("invalid daemon endpoint")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, ep.URL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(result)))
	}
	return bytes.TrimSpace(result), nil
}
