package main

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/session"
	"io"
	"os"
	"path/filepath"
)

// hookCLI ingests one provider hook payload from stdin.
func hookCLI(ctx context.Context, o options, out io.Writer) error {
	body, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
	if err != nil {
		return err
	}
	var p session.HookPayload
	if err := model.StrictJSON(body, &p); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if _, err := call(ctx, o, "POST", "/session/hook", p); err != nil {
		return err
	}
	if o.json {
		fmt.Fprintln(out, `{"ok":true}`)
	}
	return nil
}

// initHooks merges Heimdall's hook entry into the provider settings file,
// preserving every existing hook entry and unrelated keys. The JSON object
// structure is retained by operating on decoded maps.
func initHooks(ctx context.Context, o options, out io.Writer) error {
	settings := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	data := map[string]any{}
	if raw, err := os.ReadFile(settings); err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("claude settings unreadable: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	hooks, _ := data["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	const cmd = "heimdall hook"
	changed := false
	for _, event := range []string{"SessionStart", "SessionEnd"} {
		list, _ := hooks[event].([]any)
		found := false
		for _, group := range list {
			g, _ := group.(map[string]any)
			entries, _ := g["hooks"].([]any)
			for _, e := range entries {
				if m, _ := e.(map[string]any); m["command"] == cmd {
					found = true
				}
			}
		}
		if !found {
			list = append(list, map[string]any{"matcher": "*", "hooks": []any{
				map[string]any{"type": "command", "command": cmd, "timeout": 10}}})
			hooks[event] = list
			changed = true
		}
	}
	if changed {
		data["hooks"] = hooks
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
			return err
		}
		tmp := settings + ".tmp"
		if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, settings); err != nil {
			return err
		}
	}
	// Hooks need streams to bind to; ensure the Claude transcript root is configured.
	if _, err := call(ctx, o, "POST", "/session/source", map[string]any{"version": 1, "provider": "claude_code",
		"root": filepath.Join(os.Getenv("HOME"), ".claude", "projects")}); err != nil {
		return fmt.Errorf("hooks installed but source root failed: %w", err)
	}
	if changed {
		fmt.Fprintln(out, "Installed heimdall SessionStart/SessionEnd hooks (existing hooks preserved).")
	} else {
		fmt.Fprintln(out, "Heimdall hooks already installed.")
	}
	fmt.Fprintln(out, "Configured claude_code transcript root.")
	return nil
}
