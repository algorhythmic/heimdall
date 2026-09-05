# Heimdall v1 — locked handoff

Locked 2026-09-04. Scope: Go v1 on omarchy (Hyprland). This document is the v1 specification; each
weekend gets a one-page session handoff derived from §11–§12. Any deviation edits this file first.

## 1. What Heimdall is

A local daemon that observes every surface you work in (browser tabs, terminals, agent sessions,
repos), keeps an append-only event log of what it sees, projects that log into task state, and acts on
the desktop: it opens a task's surfaces as an OS workspace, writes back what you opened, notifies you
when something needs you or is rotting, and issues a ranked session plan each morning. It is
deterministic by default. Inference is optional and confined to three functions: classification,
intent extraction, plan rationale. It is not a harness, not an orchestrator, and never acts without a
ratification event.

## 2. Locked decisions

| Decision | Choice | Rationale | Rejected |
|---|---|---|---|
| Language | Go 1.22+, single static binary | one-command install, cross-compile, embed assets, zero idle | Python (runtime, size, drift); Rust (velocity cost for v1) |
| Store | SQLite (WAL) at `~/.local/share/heimdall/heimdall.db` | whole state is one file; export/replay trivial | Postgres default (a service is a dependency); Postgres remains an optional backend |
| Truth | append-only `events`; everything else is a projection rebuilt by `replay` | durable, auditable, replayable | mutable state tables as truth |
| Task edit surface | `tasks.yaml`, daemon diffs saves into events, daemon owns formatting and id assignment | structured diff = unambiguous events; lintable | markdown-canonical (identity and diff ambiguity); two-way sync |
| Unit of work | the OS workspace; manifest `workspaces/<task-id>.yaml` | membership by placement, write-back on close | per-tab filing |
| Capture | built-in `POST /capture`, one-line grammar, `unassigned` valve with 72 h expiry, why-line mandatory | zero-friction, classifier input queue | Obsidian Web Clipper (no origin, second writer); hard no-stream rule (drives lying) |
| Agent state | Claude Code hook reporter is authoritative | screen-scraping (tmux regex, herdr pane read) is one fragility class | herdr as state source |
| Reference platform | Hyprland via IPC socket; Windows and macOS are degraded backends behind the `Viewport` interface | only OS where workspace is scriptable and observable | building for three OSes at once |
| Event bus | none; in-process dispatch + SSE | one event path; all v1 consumers are local | MQTT (a bus, not a log; second path) |
| Judgment runtime | direct model calls from the daemon, provider optional | "occasionally uses inference" is a function call | Saga coupling; a long-running overseer agent |
| Notifications | daemon owns rules; delivery adapters: desktop notifier, Home Assistant webhook, sound player | one rule set, one mute | herdr as second notifier long-term; a chatbot |
| Dependencies | none required at runtime; vault projection, Huginn upstream, HA, inference are optional adapters | Heimdall must be complete alone | Muninn/Huginn as required services |
| Repo | new repo `heimdall` | new system with its own process, schema, OS integrations | folding into Muninn |
| Names | project/binary/daemon `heimdall`; every component by its technical name | one proper noun | Norse names for modules |

## 3. Repository layout

```
heimdall/
  cmd/heimdall/          main; subcommand dispatch (stdlib flag)
  internal/store/        sqlite open, migrations, event append, replay, projections
  internal/model/        types: Event, Task, Surface, Workspace, Capture, Plan, Proposal, Commitment
  internal/tasks/        tasks.yaml load, format, diff → events; preferences.yaml
  internal/capture/      grammar parser; HTTP endpoint; expiry
  internal/observe/      hyprland/, chrome/, hooks/, herdr/ (each an Observer)
  internal/viewport/     interface + hyprland backend; stubs for windows, darwin
  internal/classify/     embeddings, centroids, clustering, proposals
  internal/plan/         scoring, plan assembly, commitment ledger, optional rationale
  internal/notify/       rules; adapters: desktop, homeassistant, sound
  internal/infer/        Provider interface; ollama, anthropic
  internal/vault/        markdown projection adapter (managed regions, commit prefix)
  internal/api/          HTTP: /capture /hook /summary /events (SSE) /radiator /state
  web/radiator/          static HTML+JS, embedded via embed.FS
  contrib/               quickshell bar module, walker script, hyprland rules, bookmarklet, hook scripts
  seed/                  tasks.yaml, surfaces.jsonl, orphans.jsonl (fixtures and classifier labels)
  docs/                  this file; STATUS.md
```

Allowed third-party modules (add nothing else without editing this list): `modernc.org/sqlite`,
`gopkg.in/yaml.v3`, `github.com/BurntSushi/toml`, `nhooyr.io/websocket` (CDP only). Everything else
is stdlib.

## 4. Data model

### 4.1 Event log

```sql
CREATE TABLE events (
  id              INTEGER PRIMARY KEY,
  ts              TEXT NOT NULL,          -- RFC 3339, UTC
  subject         TEXT NOT NULL,          -- surface|workspace|capture|chat|agent|task|plan|proposal|commitment
  verb            TEXT NOT NULL,          -- past tense
  actor           TEXT NOT NULL,          -- user|cli|capture|observer:<name>|classifier|planner|notifier
  entity_id       TEXT,
  payload         TEXT NOT NULL,          -- JSON
  idempotency_key TEXT UNIQUE             -- observers dedupe on this
);
```

v1 event set:

| subject | verbs |
|---|---|
| surface | observed, opened, closed, touched |
| workspace | opened, closed, focused, diffed |
| capture | created, classified, expired |
| chat | summarized |
| agent | working, idle, blocked, released |
| task | created, updated, completed, dropped |
| plan | issued, ratified |
| proposal | created, accepted, edited, rejected |
| commitment | created, due, fulfilled, dropped |

Rule: nothing writes a projection directly. Every state change is an event first; projections are
functions of the log. `heimdall replay` truncates projections and rebuilds them; a golden test
asserts `ls --json` is byte-identical before and after.

### 4.2 Projections

`tasks`, `subtasks`, `surfaces`, `surface_state` (open, last_touch, agent_state), `workspaces`
(task_id, resident, hyprland_ws, opened_at), `captures`, `plans`, `proposals`, `commitments`,
`edges` (src_type, src_id, dst_type, dst_id, kind ∈ serves|origin|blocks|owes|member_of),
`embeddings` (entity_type, entity_id, vector BLOB, model).

### 4.3 Identifiers

- task id: `^[a-z0-9-]{3,24}$`, immutable; doubles as Hyprland workspace name, herdr workspace name,
  manifest filename, `open` argument.
- surface id: first 12 hex of sha256(`kind|pointer`).
- capture id, plan id, proposal id: ULID.
- stream: any task id or `unassigned`. Streams are not a separate table; they are the set of task ids
  plus that one literal.
- capture kind: `candidate | reference | task | study`.
- surface kind: `tab | chat | artifact | terminal | agent | repo | note | mail`.
- task type and status enums:
  - application: `researching → drafting → submitted → followed_up → replied → closed`
  - project: `backlog → active → blocked → review → done`
  - research, study: `open → summarized → filed`
  - commitment: `open → fulfilled | dropped`

### 4.4 Files

`~/.config/heimdall/config.toml`

```toml
[store]
backend = "sqlite"                        # "postgres" optional, see STATUS.md
path = "~/.local/share/heimdall/heimdall.db"

[surfaces]
hyprland = true
herdr = true                              # list/attach only; never state
chrome_cdp_port = 9222
claude_code_hooks = true                  # `heimdall init --hooks` installs them

[capture]
listen = "127.0.0.1:7477"
unassigned_ttl_hours = 72

[wip]
resident_max = 3

[notify]
desktop = "notify-send"
batch_seconds = 30
deep_session_mute = true
sounds_dir = ""                           # drift.mp3, request.mp3
home_assistant_webhook = ""               # absent = off

[radiator]
listen = "127.0.0.1:7478"
output = ""                               # Hyprland monitor name, e.g. DP-2

[inference]                               # absent = off; classifier and rationale disabled
provider = "ollama"                       # or "anthropic"
model = ""
embed_model = "nomic-embed-text"

[vault]                                   # absent = off
path = ""
commit_prefix = "skald(heimdall):"
```

`tasks.yaml` (human edit surface; daemon reformats on save; ids assigned if missing)

```yaml
version: 1
tasks:
  - id: jobapp-sensemesh
    title: SenseMesh follow-up
    stream: job-search
    type: application
    status: drafting
    importance: 4                  # 1–5, operator-set
    resume_by: 2026-09-04
    estimate_minutes: 15
    next_action: Send the follow-up email
    done: Reply received or 14 days of silence
    impact: {income: 4, skill: 1, portfolio: 1, relationships: 2}   # optional
    subtasks:
      - {id: jobapp-sensemesh-email, title: send email, done: false}
```

`preferences.yaml`

```yaml
version: 1
weights: {importance: 0.35, urgency: 0.30, impact: 0.35}
capacities: {income: 0.4, skill: 0.2, portfolio: 0.3, relationships: 0.1}
urgency_horizon_days: 14
staleness_days: 5
daily_budget_minutes: 240
```

`workspaces/<task-id>.yaml`

```yaml
task: video-series
surfaces:
  - {kind: tab, pointer: https://..., title: ..., added_at: 2026-09-04T18:02:00Z}
  - {kind: terminal, pointer: herdr:video-series}
  - {kind: repo, pointer: /opt/reelforge, branch: main}
```

## 5. Capture grammar

```
line    = ["^"] streams "/" kind ":" SP why
streams = stream *("," stream)          ; each a task id or "unassigned"
kind    = "candidate" / "reference" / "task" / "study"
why     = 1*200VCHAR                    ; non-empty; parser rejects empty
```

`^` sets `origin` to the most recent capture. `POST /capture` body:
`{"pointer": url, "title": string, "line": string}`. Response 400 on grammar failure, with the
failing production named. Expiry: `unassigned` captures get `expires_at = created + ttl`; a daily job
emits `capture.expired`, never deletes; expired why-lines are exported for the weekly brief.
Kind → default lifecycle: candidate expires in 14 d unevaluated, reference never, task binds to its
stream's `next_action` on ratification, study expires at the stream's `resume_by`.

## 6. Observers

Each implements `Observer{ Start(ctx, emit) error }` and emits events with an idempotency key.

| Observer | Source | Emits |
|---|---|---|
| hyprland | `.socket2.sock` lines: `openwindow`, `closewindow`, `activewindow`, `workspace`, `movewindow`; initial state from `hyprctl clients -j` | surface.observed/opened/closed/touched, workspace.focused |
| chrome | CDP `GET /json` on start; `Target.setDiscoverTargets` for created/destroyed/infoChanged | surface.observed/opened/closed/touched (tab) |
| hooks | shell hooks POST `{event, session_id, cwd}` to `/hook`; `UserPromptSubmit→working`, `Stop→idle`, `Notification→blocked`, `SessionEnd→released`; cwd→repo→task via manifest `repo` pointer | agent.* |
| herdr | socket API: list workspaces and panes; attach. Not state. First session reads herdr docs for the wire format and feature-flags this observer. | surface.observed (terminal) |
| summary | `POST /summary` or `heimdall close --summary`; parses `decided`, `next_action`, `resume_by`, `drop_if` | chat.summarized |

Known gap, v1: CDP window ids are not linked to Hyprland window addresses. Membership of Chrome
windows is tracked from what the viewport controller created; a tab moved by hand between windows is
reconciled on `close` via title match, and logged as `workspace.diffed` with `reconciled: title`.

## 7. Viewport controller

```go
type Viewport interface {
    Open(ctx, task Task, m Manifest) error   // refuses if resident >= wip.resident_max
    Close(ctx, task) (Diff, error)           // diff then tear down
    Focus(ctx, task) error
    Diff(ctx, task) (Diff, error)
    List(ctx) ([]Resident, error)
}
```

Hyprland backend: `hyprctl dispatch workspace name:<id>`; tabs via CDP `Target.createTarget` (first
with `newWindow:true`), then `movetoworkspacesilent name:<id>,address:<addr>` once the window
appears; terminals via `hyprctl dispatch exec [workspace name:<id>] <terminal> -e herdr attach <id>`;
chats are tabs. `Close` computes the diff against the manifest, appends additions with `added_at`,
marks removals `closed_at`, emits `workspace.closed{diff}`, then `closewindow` per address.
`windows` and `darwin` packages compile and return `ErrUnsupported`.

## 8. Classifier, planner, ratification

Classifier (requires `[inference]`): embed `title + why` per capture and surface; per-task centroid
from its labeled surfaces; cosine ≥ 0.62 → `proposal.created{kind: assign}`; below threshold,
agglomerate unassigned items and propose an initiative when a cluster reaches 3. Without a provider
the classifier is off and captures require explicit streams; the valve and expiry still run.

Planner (no inference required):

```
urgency  = overdue ? 1 : clamp((horizon - days_until_resume_by) / horizon, 0, 1)
impact   = Σ_c capacities[c] * task.impact[c] / 5           (0 if unset)
score    = w.importance * importance/5 + w.urgency * urgency + w.impact * impact
```

Blocked tasks are excluded; staleness beyond `staleness_days` adds 0.05 as a tiebreaker. The plan is
the top tasks by score whose `estimate_minutes` fit `daily_budget_minutes`, with the three named
answers: highest score, highest urgency, highest impact per capacity. With a provider, the model writes
a two-sentence rationale and phrases `next_action` and `done`; it may argue against the order once,
with a stated reason, and the operator ratifies. Golden test: fixed `seed/` input produces a fixed
plan JSON.

Ratification: `heimdall ratify` walks `proposals` newest-first, one at a time, keys `y` `e` `n`;
each answer is an event; edits to weights or assignments are events too. Nothing is applied without
ratification.

## 9. Notifier

Classes: `needs-you` (agent.blocked, commitment.due, proposals ≥ 5), `info` (plan.issued,
task.completed), `drift` (staleness crossed). Rules, in order: suppress if the event's task is the
focused workspace; suppress all while `deep_session` is set; batch anything within `batch_seconds` of
the last delivery. Delivery adapters: desktop (`notify-send`), Home Assistant webhook (payload
carries `snooze`, `open`, `drop` actions that POST back to `/action`), sound (`request.mp3` for
needs-you, `drift.mp3` once per task per day). herdr owns its own done and needs-input sounds for
agent events in v1; the daemon does not play for `agent.*`.

## 10. Interfaces

CLI (all support `--json`):

| verb | semantics |
|---|---|
| `init [--hooks]` | write config, empty `tasks.yaml`, `preferences.yaml` defaults; install hook scripts by merging `~/.claude/settings.json` |
| `start` | run daemon; `--systemd` writes the user unit |
| `ls` | tasks ranked by planner score; resident/swapped; staleness; due |
| `open <id>` / `close [<id>]` / `focus <id>` | viewport controller |
| `capture "<line>" --pointer <url>` | same as the endpoint |
| `add "<title>" --stream <id> --type <t>` | append to `tasks.yaml`, emit `task.created` |
| `state [<id>] [--active]` | task's surfaces with open/last_touch/agent_state |
| `plan [--ratify]` | issue today's plan; optionally walk ratification |
| `ratify` | proposals queue |
| `watch` | redraw-on-event text view for a terminal pane |
| `export` / `replay` | tar of db+config; rebuild projections |

Status bar: `contrib/quickshell/heimdall.qml` reads `heimdall state --active --json` every 5 s and
renders `task · next_action · last_touch · due · wip n/max`; landscape output only.

Launcher: `contrib/walker/heimdall.sh` pipes `ls --json` into the launcher's dynamic list; Enter
runs `open`, Tab prompts a capture line.

Radiator: `/radiator` serves embedded HTML; `/events` is SSE. Sections top to bottom: needs-you,
resident lanes (task, next_action, elapsed, agent states), today's plan with completion, drift.
`contrib/hyprland/heimdall.conf`:

```
workspace = name:radiator, monitor:<output>, default:true
windowrulev2 = workspace name:radiator silent, title:^(Heimdall radiator)$
windowrulev2 = fullscreen, title:^(Heimdall radiator)$
exec-once = chromium --app=http://127.0.0.1:7478/radiator --window-name="Heimdall radiator"
```

## 11. Milestones and what done looks like

**M1 (weekend 1): store, tasks, capture, viewport, CLI, bar.** Done when: `heimdall init` on a
clean box produces a running daemon; editing `tasks.yaml` and saving yields exactly one `task.updated`
event per changed field; `open video-series` creates a Hyprland workspace with its tabs and terminal;
opening one extra tab and running `close` appends it to the manifest; a bookmarklet capture lands as
`capture.created`; the bar shows the focused task's `next_action`; `replay` yields byte-identical
`ls --json`.

**M2 (weekend 2): observers and notifier.** Done when: `agent.blocked` from a real Claude Code
session appears in `state` within 2 s; `last_touch` updates on tab switch; a task past
`staleness_days` (test with `--now` override) produces one drift notification and one sound; herdr
`List` returns its workspaces or the observer is cleanly flagged off.

**M3 (weekend 3): planner, ratification, classifier.** Done when: `plan` on `seed/` matches the
golden JSON; with a provider, rationale is present and `ratify` walks it; the classifier assigns at
least 70 % of `seed/surfaces.jsonl` to their labeled task ids; `seed/orphans.jsonl` yields at least
two initiative proposals.

**M4 (weekend 4): radiator, phone, reproducibility.** Done when: the portrait output shows the
radiator updating within 1 s of an event; HA actions write back as events; `export` on one box and
`replay` on a fresh install reproduce `ls --json`.

## 12. First-session checklist (M1)

1. `go mod init`; layout from §3; `Makefile` with `build`, `test`, `lint`, `install`.
2. `internal/store`: open with WAL, migrations, `Append(Event)`, `Replay()`, projection builders.
3. `internal/tasks`: load, validate against §4.3 enums, format, diff → events; assign ids.
4. Import `seed/tasks.yaml` (25 tasks) and `seed/surfaces.jsonl` (127 surfaces) as fixtures.
5. `internal/capture`: parser with table tests for every production and every failure.
6. `internal/observe/chrome`: `ListTargets`, `CreateTarget(newWindow)`, `CloseTarget`.
7. `internal/observe/hyprland`: `Dispatch`, `Clients()`, `Workspaces()` (socket2 subscribe is M2).
8. `internal/viewport/hyprland`: `Open`, `Close` with diff, `Focus`, `List`.
9. `cmd/heimdall`: `init`, `start`, `ls`, `add`, `capture`, `state`, `open`, `close`, `focus`,
   `replay`, all with `--json`.
10. `contrib/quickshell/heimdall.qml`; `contrib/bookmarklet.js`; `contrib/systemd/heimdall.service`.
11. Run the M1 done list; record results in `STATUS.md`.

## 13. Constraints for the implementing agent

- No self-configuration: the daemon never edits `config.toml`, never changes weights or thresholds
  without a ratification event, never adds hooks without `init --hooks`.
- `tasks.yaml` is rewritten only to format and to assign ids; field values are never changed by the
  daemon.
- No model call unless `[inference]` is present; no network call except configured adapters and
  `127.0.0.1` sockets.
- No goroutine without a context; the daemon must exit cleanly on SIGTERM in under 1 s.
- One milestone item per PR; each PR adds or updates a test; the replay golden test runs on every PR.
- Report deviations from this document instead of resolving them silently.

## 14. Scale path (document in STATUS.md, do not build)

| Item | Trigger |
|---|---|
| Postgres backend | second device needs shared state, or a second user |
| Event bus (MQTT or NATS) | a second observed machine, or a hardware indicator on the desk |
| sqlite-vec index | more than 20k embeddings |
| Windows, macOS backends | two or more working days a week on those machines |
| Tracker web page | after M4 review, if `state` plus the bar is insufficient |
| ActivityWatch importer | `last_touch` from Hyprland plus CDP proves insufficient |
| Synchronous classifier pre-fill replacing the `unassigned` valve | proposals accepted above 80 % for a month |
| Context-injection proxy for harnesses | more than three harnesses need task-scoped context |
| Daemon-owned agent sounds | when the daemon's notifier is trusted; then `herdr` sound off |

## 15. Operator fills before M3

- `preferences.yaml`: the ruling on the eight application tasks versus the consulting-and-product
  direction; this is a weight and an `importance` value, not code.
- herdr socket wire format for list and attach (from its docs); the observer stays flagged off until
  confirmed.
- Launcher: whether walker accepts a dynamic list from stdin or needs a mode plugin.
- Hyprland output name for the portrait monitor; Chromium launch flag for CDP in the omarchy config.
- Three sound files.
