# Terminal workstream interface

The terminal interface replaces the old browser GUI. It follows the September 8
reference design: an attention queue, expandable workstream table, selected
context and keyboard-operated dialogs on a dark background with amber focus.
The daemon remains the only database writer.

## Run from this checkout

```sh
./bin/heimdall start --data-dir ./demo-data
```

In another terminal:

```sh
./bin/heimdall tui --data-dir ./demo-data
# Optional task/subtree filter, with a step selected explicitly:
./bin/heimdall tui video-series#storyboard --data-dir ./demo-data
```

For a fresh data directory, run `./bin/heimdall init --data-dir ./demo-data` first.
The binary is built with `go build -trimpath -o bin/heimdall ./cmd/heimdall`; it is
not installed on PATH automatically. On Windows use `bin\heimdall.exe`.

A complete synthetic example is available without setting up user tasks:

```sh
node scripts/tui-demo.cjs
```

This creates a separate directory under `.tools/`, starts its daemon, opens the
TUI, and stops that daemon when the terminal closes. It includes an actual
completion proposal, decision proposal, changed files, retained progress and a
desired terminal surface. `--seed-only` prepares the data and prints its location.

`heimdall ui` is an alias for `tui`. There is no browser URL, sign-in code, web
server frontend or frontend build. The old `/ui/*` and `/ui-bootstrap` routes
are removed. `--progress-review` is accepted as a compatibility option but is
unnecessary in the local terminal interface.

## Navigate and act

| Key | Action |
|---|---|
| Tab / Shift-Tab | Focus needs-you, workstreams or selected context. Compact mode has two panels. |
| Up/Down or j/k | Select a row or scroll the focused context/dialog. Page Up/Down moves further. |
| Space | Expand/collapse steps and child workstreams. |
| Enter | Inspect the selected need, task or step. |
| / | Find tasks and steps by ID/title/next action; Escape clears the filter. |
| c | Open a retained progress draft for the explicit target. |
| f | Inspect current file observations and exact artifact pins. |
| p | Preview desired workspace membership and fresh bound Herdr session checks. |
| b | Bind an explicit desired terminal surface to a local Herdr socket/pane. |
| r | Refresh task state and selected file observations. |
| v | Toggle the compact overview. |
| ? | Show all keys. |
| q / Ctrl-C | Quit and restore terminal settings. |

Workstreams sort by resume-by date, then most recently saved progress. Steps and
child tasks keep their own identities. A checkpoint's next action is marked for
review if its recorded direction is stale. The terminal polls every five seconds;
reads do not change task state. Background refresh cannot replace a dialog's
original review preconditions or edit fields.

The **step/task dialog** shows acceptance criteria, recorded checks, evidence age,
file observations and a pending completion proposal. `a` accepts that proposal
with live backend revalidation; `x` rejects it. `f` rechecks current observations.
`e` opens an evaluator picker: Enter explicitly runs the selected accepted
evaluator, which may execute its configured command. `o` opens a separate reopen
confirmation. A recorded pass alone never completes a task.

The **decision/artifact dialog** shows the frozen proposal, contract, exact
version/digest, last review and current file checks. Tab edits the required note;
Escape returns to actions. `a` accepts, `x` rejects and `m` marks reviewed.
`r` explicitly rechecks without replacing the note. These actions do not complete
work. Proposals are authored through the existing [progress CLI](PROGRESS-SETUP.md).

The **save-progress dialog** has summary, next action, current step and blocker
fields. Tab moves between fields, Ctrl-U clears a field, and Enter/Ctrl-S submits.
Escape leaves a field and then closes the dialog. `e`, from the action area,
opens the draft in `$VISUAL`, `$EDITOR`, or `vi`; the variable must name a single
executable. No shell command is constructed. Only editable checkpoint content
can change; identity, original preconditions and artifact pins are checked on
return. New artifact versions must be recorded and explicitly selected through
the artifact/CLI draft workflow; the TUI never silently repins changed files.

## Retained drafts and exact retries

Drafts are private files under `DATA/tui-drafts/`. Each edit is saved through a
synced temporary file and rename. Reopening `c` for the same target restores the
original request ID, preconditions and entered text. An attempted submission
locks its content for exact retry. Successful drafts are archived as
`REQUEST_ID.saved.json`. `n` preserves a conflicted draft as
`REQUEST_ID.retained.json` and prepares a fresh one against current context.

Before any mutation, the TUI also stores its exact request under
`DATA/tui-requests/`. A transport error keeps that request unchanged. Enter
retries it; closing or quitting leaves the file available for recovery:

```sh
./bin/heimdall tui --data-dir DATA --request DATA/tui-requests/REQUEST_FILE.json
```

Inspect the retained request and press Enter to send it again. Successful request
files gain a `.done.json` suffix. These files contain no daemon credentials. No
request is retried automatically, including on startup. A conflict requires
fresh inspection or a new checkpoint draft; repeatedly retrying a conflict does
not update its original preconditions.

## Compact display, scope and current limits

`--compact` provides the narrow overview shown separately in the design. The
layout also switches automatically below 90 columns or 28 rows. Below 45 × 14,
it asks for a larger terminal. For a read-only text snapshot:

```sh
./bin/heimdall tui --snapshot --width 140 --height 44 --data-dir DATA
./bin/heimdall tui --compact --snapshot --width 60 --height 24 --data-dir DATA
```

Snapshots use the same cell renderer and contain no ANSI sequences. Terminal
controls and Unicode direction controls in task data are escaped. Color follows
terminal support and respects `NO_COLOR`.

The TUI is a local human CLI client. It reads the selected data directory's CLI
endpoint, rediscovers the daemon after restart and uses the same authority as
other CLI commands. A task argument filters the display; it is not an access
grant. Scoped MCP/agent credentials and extension credentials gain no new powers.
Current database schema is 20. Historical UI-v2 reviews still replay with their
original provenance, but there is no live browser-session authority.

The screenshots include later roadmap capabilities. Current binding counts are
recorded identities, not live working/blocked/idle agent telemetry. Workspace
preview compares the current saved point with fresh owned windows and Herdr
readback, including age, autosave policy, layout changes and review requirements.
Press `s` to review an explicit capture/pin request; Enter submits the retained
request and Escape cancels. See [workspace preview](WORKSPACE-PREVIEW.md).
[Reviewed application adapters](APPLICATION-RECOVERY.md) support explicit recovery;
full recovery verification, agent pane jumping and a native Omarchy bar module
remain future stages. No desktop configuration or login hook
is installed. The [reference design notes](design/TUI.md) map these boundaries.

Run `go test ./internal/tui` and `node scripts/tui-smoke.cjs` for simulation and
compiled acceptance. Linux additionally exercises a real PTY with Python 3.

W05/W06 workspace controls: inside `p`, use `o` to review open/focus and `c` to review graceful close. Enter submits the frozen scoped operation; Escape cancels. `r` shows fresh observations and recent outcomes. Reviewed application recipes are shown in the comparison/confirmation; create or retire them through the CLI. Unsupported close policies preserve open views. See [application recovery](APPLICATION-RECOVERY.md) and [workspace operations](WORKSPACE-OPERATIONS.md).
