# Heimdall v1.1 — revised implementation specification

Implementation note: this remains the target specification. The core and browser metadata/control slices and their explicit deviations are tracked in [implementation status](../STATUS.md); they do not yet satisfy every milestone below. The current browser contract and installation steps are in [BROWSER-PROTOCOL.md](../BROWSER-PROTOCOL.md) and [BROWSER-SETUP.md](../BROWSER-SETUP.md).

Continuity amendment, 2026-09-04: development build 0.3.0 adds CLI-authored accepted contracts/decisions, bounded resource bindings, immutable checkpoints, mandatory resume context and database snapshots. These records remain outside the task edit view. See [CONTINUITY-SETUP.md](../CONTINUITY-SETUP.md) for the adopted interface and limits. The [seven-improvement plan](../IMPLEMENTATION-PLAN.md) brings scoped MCP and task UI forward; optional execution-host coordination remains a later unimplemented extension to the original boundary below.

Scoped-access amendment: build 0.4.0 adds explicit reviewed resource IDs to accepted contracts and separate expiring/revocable task/subtree read credentials, with bounded history and resource observation permissions. See [SCOPED-ACCESS.md](../SCOPED-ACCESS.md). MCP transport and machine mutation grants remain open; the daemon is still the sole writer.

MCP amendment: build 0.5.0 implements the stdio adapter and explicit checkpoint-write grants. Progress records carry authenticated client/grant provenance; authority is checked before dedupe and commit. Contracts, accepted decisions, bindings and completion remain under the trusted CLI boundary. See [MCP-SETUP.md](../MCP-SETUP.md). Execution-host coordination remains unimplemented.

Revision: 2026-09-04. Supersedes the supplied v1 handoff for the proposed build. This is an implementation-ready core design with explicit integration gates, not a claim that integrations have been tested. Decision rationale and primary sources are in [REVIEW.md](REVIEW.md); Braid's wire contract is in [BRAID-CONTRACT.md](BRAID-CONTRACT.md).

## 1. Purpose and boundaries

Heimdall is a local daemon that records observable work, projects task state, restores task workspaces, proposes evidence-backed updates, and prepares a deterministic daily plan. It is not an agent harness or autonomous executor of task work. It does not send mail, run implementation agents, or infer that task completion occurred merely because an agent stopped.

The primary workflow is browser conversation → artifact → another conversation or coding session. Sensors cover Claude, ChatGPT, Claude Code, and Codex to the extent each installed source exposes evidence. Following initial installation, permissions, and account/provider setup, capture of supported session data and summary generation require no per-session activation or structured user input. Optional manual capture remains available. Unsupported or broken content sensors expose a gap instead of requesting a mandatory typed summary.

Heimdall owns domain events, task/workspace state, evidence matching, proposals, planning, and action authorization. **Braid owns retrieval algorithms and its rebuildable index**, including lexical/dense retrieval, graph ranking, fusion, budget packing, and embedding caches. Heimdall remains useful without Braid or model providers.

Reference platform: Linux/omarchy with Hyprland. Core storage, schemas, planning, and Braid contract tests run on Windows and Linux. Windows/macOS viewport operations return `ErrUnsupported` until separately implemented. Runtime desktop dependencies are explicit; “single binary” does not mean a compositor, terminal, browser, extension, and notification utility are unnecessary.

## 2. Decisions

| Area | v1 decision |
|---|---|
| Implementation | Separate `heimdall` Go module/repository. Go language baseline 1.24; reproducible initial toolchain `go1.27.1`, with release upgrades recorded. |
| Store | SQLite/WAL. One daemon writer. Event append and affected projections commit in the same transaction. Postgres is deferred. |
| Truth | Versioned append-only semantic events. Content blobs have independent retention. Replay never invokes sensors, models, notifications, or desktop actions. |
| Task editing | `tasks.yaml` is a versioned edit view of event state. Accepted commands and proposals also update that view using revision checks. Explicit `fmt`; no reformat on every save. |
| Hierarchy | `parent` links tasks; local subtasks are execution steps. Stream means a routing address: task ID or `unassigned`. |
| Workflows | `types.yaml` contains templates with statuses, subtask defaults, and proposed transition rules. Template versions are materialized when used. |
| Authorization | Observations and configured background processing run automatically. Explicit commands/validated file edits authorize their own mutations; machine suggestions require proposal acceptance. |
| Browser | First-party MV3 extension plus native messaging, used for observation and browser actions. Ship this path first; CDP is a gated fallback experiment. |
| Agents | Prefer authenticated structured hooks. Record source, observed time, and reliability. Heuristic signals are optional, lower-confidence evidence; never completion proof. |
| Retrieval | Optional Braid subprocess via existing newline-delimited JSON. A library integration waits for Braid's publishable module path. No duplicate centroid/clustering engine in Heimdall. |
| Inference | Optional direct providers for intent extraction, assignment suggestions, and plan rationale. Retrieval embeddings are separately configured in Braid. |
| Delivery | Daemon rules and durable notification state; desktop first. Home Assistant, sound, MCP, and vault projection are optional later adapters. |
| API | Private local IPC first; authenticated loopback HTTP where needed. No unauthenticated browser or remote mutation endpoint. |

Direct module allowlist: `modernc.org/sqlite`, `gopkg.in/yaml.v3`, `github.com/BurntSushi/toml`. Add `github.com/emersion/go-imap/v2` when the IMAP adapter lands, pinning a tested release and transitive dependencies. Do not hand-roll IMAP. `github.com/coder/websocket` is allowed only if the CDP spike is adopted. MCP SDK selection is an M4 design gate. Record exact versions and licenses in `go.mod`, `go.sum`, and release notes; the allowlist is for direct dependencies, not an impossible ban on transitives.

## 3. Domain identity and hierarchy

- Task IDs match `^[a-z0-9][a-z0-9-]{1,22}[a-z0-9]$`, immutable, unique. Reserve `unassigned` and `radiator`. Use `heimdall-<id>` as the Hyprland workspace name to avoid unrelated workspace collisions. A herdr label may be a task ID; its actual workspace ID is assigned by herdr and stored separately.
- A task has at most one parent. Validate existence and reject cycles. Parent changes do not change IDs. A task may have child tasks or local subtasks, but not both in v1. Empty tasks can remain leaves with one explicit next action. Completion of descendants is a proposal for parent completion, not an automatic cascade.
- Subtask IDs are scoped to the task: `send`, `follow-up`. External references use `<task-id>#<subtask-id>`. `after` references local subtask IDs and forms an acyclic prerequisite graph. Subtask statuses are `open`, `blocked`, `done`, and `dropped`; imported completed steps are explicit user attestations recorded by the import command.
- Stream addresses route captures/surfaces to tasks; they are not workflow definitions. `job-search` can be a project parent, `jobapp-sensemesh` its application child. The former `stream:` field in task definitions becomes `parent:`.
- A logical surface is content identity, distinct from a live instance. Use full SHA-256 of a versioned canonical identity tuple. `tab/chat` is a classification, not part of a URL identity key; a chat URL must not create duplicate surfaces when its classification changes.
- Canonicalization lowercases scheme/host and removes default ports. Preserve query parameters, fragments, path case, and trailing slashes by default because they can identify content. A versioned site adapter may normalize known tracking parameters and use account-scoped conversation IDs. Keep the raw pointer too. Canonicalization changes require explicit alias/migration events.
- Each open instance uses `sensor_id + sensor_epoch + native_id`: browser profile/epoch/tab ID, compositor epoch/window address, or tool/session ID. One surface can have several instances in different workspaces. Navigation rebinds an instance; it does not globally close the old surface while another instance remains open.
- Artifact ID is SHA-256 of exact bytes; artifact occurrences include source conversation/message/tool ID. Matching bytes support “same content”; they do not prove copying direction. Add an `origin` edge only when an observed export/import establishes direction; otherwise record `same_content`. Partial/fuzzy matches are retrieval candidates, never deterministic lineage.
- Use opaque random 128-bit IDs from `crypto/rand` for commands, captures, sessions created by Heimdall, proposals, plans, and notifications. Log order comes from event sequence, not ID timestamps. Native agent IDs are namespaced by provider/source.

## 4. Event storage, replay, and operational state

Minimum envelope:

```sql
CREATE TABLE events (
  id INTEGER PRIMARY KEY,
  event_version INTEGER NOT NULL,
  ts TEXT NOT NULL,
  observed_at TEXT,
  subject TEXT NOT NULL,
  verb TEXT NOT NULL,
  actor TEXT NOT NULL,
  entity_id TEXT,
  command_id TEXT,
  causation_id INTEGER,
  idempotency_key TEXT UNIQUE,
  payload TEXT NOT NULL CHECK(json_valid(payload))
);
```

`ts` is daemon receipt time in UTC RFC3339Nano. `observed_at` is source time if available. `id` orders projection. Payloads include source identity/version and reliability (`structured`, `inferred`, or `heuristic`), plus evidence references for derived claims. Do not use an arbitrary numerical confidence as calibrated probability. Unknown event versions fail replay before changing live projections. Versioned upcasters are pure functions and preserve the original log.

| Subject | Required verbs |
|---|---|
| command | accepted, rejected |
| task / subtask | created, updated, completed, reopened, dropped |
| workflow / preferences | updated |
| surface | observed, aliased |
| instance | opened, navigated, moved, closed |
| attention | span_recorded |
| workspace | open_requested, opened, diffed, close_requested, closed, focused, operation_failed |
| capture | created, assigned, expired |
| session | started, checkpointed, ended |
| agent | state_changed |
| chat / artifact | summarized / observed |
| evidence | recorded |
| proposal | created, accepted, edited, rejected, superseded |
| timer | scheduled, due, cancelled |
| plan | issued, ratified |
| notification | queued, delivery_started, delivered, failed, snoozed, dismissed |
| mute | set, cleared |
| sensor | degraded, recovered |
| retrieval | published, failed |
| content | purged |

Create JSON payload schemas and golden samples with the first consuming milestone. Do not implement arbitrary verb strings with no validator. Semantic relationships (`parent`, `member_of`, `serves`, `origin`, `same_content`, `blocks`) belong to Heimdall; retrieval gets copies, not a second authority.

Projections: tasks, subtasks, workflows, preferences, surfaces, instances, attention, workspaces, captures, sessions, artifacts, evidence, proposals, timers, plans, notifications, mutes, sensors, domain edges, and action status. All domain changes originate in events. Runtime queues, file hashes, locks, source cursors, and disposable Braid generation metadata may use operational tables; they are not user-state truth. Commit an input cursor atomically with the corresponding events to avoid loss after acknowledgement.

Repeated source messages return the prior event IDs when their idempotency key and content match; the same key with different content is a conflict. File edits emit one `task.updated` per task containing a field patch, not one event per field; all accepted changes in one document commit atomically. No-op edits and daemon echoes emit no task events.

Replay builds temporary projections from the immutable log and switches atomically only after validation. The application pauses mutation or holds its write lock while doing so. Scheduler due events, summaries, provider output, accepted templates/preferences, and action results are already in the log. Replay does not rederive them from current wall clock, live files, or current providers. JSON is canonicalized with stable array order and UTC formatting. Compare `ls`, `state`, and `plan` at an explicit `--now` and log boundary.

Durable actions use an outbox: commit authorization and request, perform the external operation, then record its result. Crash recovery reconciles by operation ID and live ownership before retrying. External exactly-once delivery is not promised. Do not blindly repeat window creation, closure, or notifications after an ambiguous acknowledgement. SIGTERM stops accepting commands, flushes bounded queues, checkpoints cursors, and attempts shutdown within 2 seconds; after 5 seconds the service manager may kill it. Recovery must remain correct.

Export uses a consistent SQLite backup, never copies a live WAL database file alone. Include event schema/version, templates, accepted preferences, workspace definitions, and content digests. Secrets are excluded. Include retained content and Braid vectors only with explicit export options; a fresh restore must disclose missing content and unavailable integrations. Replay reconstructs logical state, not the physical desktop on another machine.

## 5. Configuration and editable files

Paths follow XDG on Linux: config in `~/.config/heimdall/`, state in `~/.local/share/heimdall/`, IPC under `$XDG_RUNTIME_DIR/heimdall/`. Secrets live in restricted files or environment references, never event payloads. Windows core tests use a supplied `--data-dir`.

```toml
version = 1
timezone = "America/Los_Angeles"

[store]
path = "~/.local/share/heimdall/heimdall.db"

[api]
listen = "127.0.0.1:7477"
token_file = "~/.config/heimdall/api.token"

[browser]
transport = "native"
content_sites = ["claude.ai", "chatgpt.com"]
private_browsing = false

[capture]
unassigned_ttl_hours = 72
origin_window_minutes = 30

[wip]
resident_max = 3

[notify]
batch_seconds = 30
agent_sound_owner = "herdr"

[retention]
content_days = 30

# Optional; absent means disabled. No provider calls merely because this exists.
#[retrieval]
#backend = "braid-stdio"
#executable = "/usr/local/bin/braid"
#generation_dir = "~/.local/share/heimdall/retrieval"

# Optional generation provider; separate from Braid's embedding provider.
#[inference]
#provider = "ollama"
#model = "<installed-model>"
#allow_remote_content = false
```

Accepted values of config that affect semantics are recorded with the command/plan/proposal they influence. The daemon never edits configuration autonomously. Secret changes are recorded only as a reference/version, not the secret.

`tasks.yaml` contains `version`, daemon `revision`, and task definitions; [examples/tasks.yaml](examples/tasks.yaml) is the seed shape. `types.yaml` is a local template catalog, with versioned statuses, terminal statuses, subtask defaults, and mappings. `preferences.yaml` defines normalized weights, capacities, time horizon, staleness, and budget; defaults appear in §10.

Single-writer reconciliation:

1. CLI and UI submit commands to the daemon; they never both append events and independently write task files.
2. Poll the directory and content hash (initially every 500 ms), handling atomic replacement by editors. Debounce until two reads agree. Parse strict YAML, reject duplicate/unknown keys, invalid dates, bad references, and malformed checks; retain the last valid state.
3. A complete saved document must carry the current revision. Validate and apply its semantic diff atomically, recording `command.accepted` with source `file` and the before/after revision. Missing IDs are allocated in this transaction.
4. Before writing back new revision/IDs or accepted proposal changes, compare the disk content with the hash read. If it changed, preserve the file, save a pending view, and expose a conflict for merge. Never overwrite an intervening edit. Daemon-origin writes are recognized by revision/hash.
5. Removing a task from the file is not silent deletion. Require a terminal/drop transition first; otherwise reject the omission. IDs and audit history survive drops. `fmt` is explicit and revision-checked.

This deliberately replaces the original impossible combination of “events are truth,” “ratify changes fields,” and “daemon may never update field values.” File edits, CLI updates, and accepted proposals now share one mutation path. Preferences/template edits use the same revision discipline; they do not retroactively alter existing instantiated tasks.

Workspace manifests are daemon-generated views containing task ID, logical surface IDs, raw open pointers, restore policy, branch/worktree hints, and observed herdr workspace ID. Browser/native instance IDs are valid only in their recorded epoch. Use an explicit import command for hand-edited manifests. Closed membership history is separate from desired restore membership; closing a workspace must not remove every saved tab from the next restore.

## 6. Capture

Human convenience grammar:

```text
line    = ["^"] streams "/" kind ":" SP why
streams = stream *( "," stream )
kind    = "candidate" / "reference" / "task" / "study"
```

`why` is 1–200 Unicode scalar values after trimming; spaces are allowed, control characters are rejected, and whitespace-only input fails. Names are exact, streams unique. `unassigned` cannot be combined with task IDs. Return a production name and byte offset on parse errors. Unknown task IDs fail validation.

`POST /capture` accepts `{pointer,title,line,client_id,idempotency_key,origin_id?}`. One capture event contains the deduplicated target array; fan-out is projected membership, not duplicate capture events. `^` refers only to the same authenticated client/actor's last capture within 30 minutes; ambiguity/no candidate is an error. Prefer explicit `origin_id` in sensor protocols.

Unassigned expires after 72 hours; assignment cancels that timer. Unevaluated candidates expire after 14 days, references have no kind expiry, and study captures use the addressed task's explicit due date only if present. For multiple addresses use the earliest applicable kind deadline. Record deadlines at creation/assignment rather than recomputing during replay. Expiry hides the item from the active queue; it does not erase its event history. `task` captures propose a subtask/next action and cannot mutate task state before acceptance.

Automatic session/evidence capture does not use this human grammar and does not require a fabricated why-line. It records source/context directly; inferred rationale is labeled as such. The extension handles explicit capture UI; a bookmarklet or URL scheme is optional convenience, not an installation acceptance dependency.

## 7. Sensors and observation granularity

Every sensor reports capabilities, version, health, source epoch, sequence/cursor, and bindings. Tiers: 0 existence; 1 attention; 2 agent lifecycle; 3 content. A tier is reported per instance/session, not promised globally for a product.

| Source | Baseline | Content and boundaries |
|---|---|---|
| Hyprland | Window/workspace existence and active window | No chat or mail semantics. Bootstrap snapshot plus buffered IPC, reconcile on reconnect. |
| Chromium extension | Tab/window IDs, URLs, navigation, focus, moves | Enabled site adapters capture message/artifact snapshots automatically. Unsupported sites remain metadata-only. |
| Claude Code | Installed-version hooks and transcript reference | Bounded incremental transcript adapter after setup; verify Desktop Code separately. |
| Codex CLI/app | Installed-version hooks and transcript reference | Probe hooks and parser compatibility per installed client; do not assume rollout JSONL is stable. |
| herdr | Workspace/pane inventory and live binding | Structured agent reports retain producer provenance; optional heuristic state only if hook coverage is absent. |
| Claude Desktop Chat / ChatGPT native app | Window metadata on supported compositor | Full passive content is unverified. Optional model-initiated MCP summaries do not satisfy automatic reliable coverage. |
| Maildir / IMAP | Mail evidence from configured account/folders | Shared mailbox source works with native and web clients. No inference from Gmail page titles or clicks. |
| Repo | Explicitly registered repo/worktree hooks or polling | Commit identity and changed paths, not proof of task success. |

Focus: combine tab activation with browser window focus, compositor focus, and session lock/idle state. An active background tab is not attention. Record spans on blur, navigation, idle/lock, or a 60-second checkpoint. Durable spans use unique span/segment IDs; adjacent segments do not overlap. Expose a live start timestamp for the bar, but durable `last_touch` advances only from evidence. Missing shutdown gives an explicitly truncated span, never an invented long duration.

Hooks: emit observational messages only; no permission decisions, stdout instructions, or task state mutation. `UserPromptSubmit` and observed tool activity imply working, `Stop` implies idle, permission-request evidence implies blocked, and `SessionEnd` implies released. Filter Claude notification types. `PostToolUse` clears a prior blocked state when later in the same turn. Track per-session/turn ordering; late callbacks cannot overwrite a later stop. Conflicting or expired signals become unknown. An idle timeout is `session.checkpointed{reason: idle}`, not proof of process exit.

For Codex, current official hooks include permission and end signals; the older discussion's blanket exclusions are not carried forward. Install only hooks supported by the actual version and complete its trust/setup flow. Preserve existing hooks and notify settings. Use a legacy notify wrapper only if necessary and demonstrably compatible; never overwrite another tool's command. Transcript parsers handle partial lines, truncation/rotation, dedupe, and schema drift. Unknown formats produce `sensor.degraded` and preserve already-observed facts.

Task binding order: current live pane/workspace mapping; launch-time `HEIMDALL_TASK` when consistent; saved provider session binding; unique canonical worktree/repo+branch match; otherwise unassigned. Herdr environment variables can become stale after pane movement. Never arbitrarily choose between two tasks sharing a repo. Parent/child binding is explicit; retrieval can propose a more specific task.

Browser content adapters are versioned and independently disableable. Prefer documented exports/interfaces; where none exist, use a minimal content adapter for the currently viewed conversation, marked unofficial. An authenticated private backend request is not automatically more stable than DOM extraction. The spike chooses a tested source, records its schema/version, and keeps site/account access inside the browser. Do not export cookies, reuse browser tokens in the daemon, fetch unrelated conversation history, or intercept all network traffic.

Snapshot while content is available, after settled turns and periodically while active; never rely on a final fetch after tab destruction. Include provider/account scope, conversation/message IDs, revision hashes, timestamps, completeness, and available artifacts. Handle branching/edited conversations with versioned snapshots, not naive append-only turn assumptions. Deduplicate by conversation revision. Reconnect resends unacknowledged snapshots and reconciles tab inventory. Persist a bounded extension outbox; report overflow and missing content.

Intent extraction: coalesce new conversation revisions; after a settled response/idle checkpoint/end, enqueue one bounded job per unprocessed revision. With no provider, retain metadata/content per policy and report `summary_unavailable`. With a provider, validate output fields and evidence pointers, store model/prompt version/input digest and result in `chat.summarized`, then propose task changes. Retry transient failure with bounded backoff; unchanged revisions do not generate repeated proposals. Old proposals become superseded after newer task or conversation revisions. Dates not supported by transcript evidence remain unset. Models cannot ratify, call desktop actions, or execute instructions from captured text.

## 8. Browser and viewport operations

The [browser extension specification](BROWSER-EXTENSION.md) defines the runtime split, deployment artifacts, permissions, protocol messages, and recovery. The extension and daemon run together; a browser-launched helper forwards messages and has no independent service or state authority.

The extension connects to `heimdall browser-host`, a native-messaging process forwarding validated messages to the daemon's private IPC. It is not a second database writer. Restrict allowed extension origins; frame, size-limit, authenticate, and sequence messages. Content scripts may submit observations but cannot request arbitrary shell commands or window destruction. Browser actions originate only from daemon commands already authorized by the user.

Use browser tab/window APIs for creation, navigation, closure, activation, and membership. This avoids the original CDP/extension dual identity problem. Pair browser window ID to a Hyprland address with a fresh nonce in a temporary extension-owned page title during creation, serialize pairing, require exactly one match, then navigate. Record the browser profile/epoch and compositor epoch. For an existing unpaired window, retain unknown OS membership until an explicit pairing succeeds. **Never use ordinary page-title matching as authoritative identity.** A dedicated profile is optional; all managed profiles require the extension. CDP fallback would require a separate configured profile and its own acceptance test.

```go
type Viewport interface {
    Open(context.Context, OpenRequest) (Operation, error)
    Close(context.Context, CloseRequest) (Operation, error)
    Focus(context.Context, TaskID) (Operation, error)
    Diff(context.Context, TaskID) (WorkspaceDiff, error)
    List(context.Context) ([]Resident, error)
}
```

Requests carry operation IDs and manifest revisions. `open` creates/restores only missing owned surfaces. An already-resident task focuses it. Reserve WIP capacity transactionally before launch; concurrent opens cannot exceed the limit. On capacity refusal, report residents and accept explicit `open <id> --swap <resident-id>`; never pick an eviction silently. Swap closes the specified resident first and may fail partially; report exactly what remains open.

`close --dry-run` provides a current diff and affected instances. Normal explicit `close` authorizes saving verified workspace membership and requesting graceful closure of Heimdall-owned instances. Moved-in or ownership-ambiguous windows are listed and left open unless explicitly adopted. Blocking agents, unsaved prompts, shared terminal sessions, and browser before-unload refusal leave the workspace partially resident; no process kill fallback. Default terminal closure detaches a view without destroying herdr work or worktrees. Generic terminal support is required when herdr is absent; reliable reattachment then remains unavailable.

Persist `workspace.diffed` and desired manifest changes before closure requests; emit `workspace.closed` only after observing that affected owned instances closed. Use one diff event, referenced by the result, rather than duplicating it in closed payloads. If the daemon crashes midway, restart reconciles actual instances against the operation. `replay` never recreates or closes them. Exclude radiator, pairing pages, and the daemon's own UI from capture/planning.

## 9. Completion checks and ratification

`done: {text, mode: any|all, checks: [...]}` exists for tasks and subtasks from M1a. Each check has a stable local ID, kind, and typed parameters. Checks evaluate to `matched`, `not_matched`, `unknown`, or `unsupported`; unknown/unsupported is never success. Omitted checks mean manual completion. Empty automatic check lists are invalid. `any`/`all` must be explicit when multiple checks exist.

| Kind | Evaluator |
|---|---|
| manual | Explicit user completion command; records user attestation. |
| subtasks_done / children_done | All non-dropped members terminal-success, at least one member; proposes parent completion. |
| silence | Configured interval since a ratified anchor, with no matching response and sufficient source coverage. |
| mail.sent / mail.received | Account, direction, correspondent, time window, and optional thread/reference match. |
| agent.released | Bound structured session release only; appropriate for a “run ended” check, not general task correctness. |
| repo.commit | Explicit canonical repo/worktree and commit/ref predicate. |
| gh.pr_merged | Registered schema in v1, evaluator deferred; reports unsupported. |

Mail metadata includes account ID, folder role, Message-ID where present, mailbox UIDVALIDITY/UID or local stable identity, sender/recipients, subject, In-Reply-To/References, and source/receipt timestamps. Account + mailbox epoch + UID deduplicates IMAP delivery, not Message-ID alone. A sent-folder copy is evidence of a recorded send, not guaranteed delivery; historical messages imported later cannot satisfy a newly opened check without its explicit anchor/window. A domain match alone is broad candidate evidence and must not auto-complete multiple tasks.

Silence is **absence under coverage**, not just a wall-clock timeout. Anchor its deadline to a ratified send/subtask fulfillment event, using the accepted send's evidence time when available, otherwise the user-attested time recorded in the command; reopening/dropping the anchor cancels the timer. Record a named response check. At the deadline, require catch-up through the interval and no matching reply. During disconnect, auth failure, unindexed folders, or ambiguous retention, the result is unknown; offer an explicit manual “close anyway” command, not an automatic silence claim. Without mail configured, the timer produces a review reminder only. Backfill a late response before acceptance and supersede stale proposals.

Evidence matches produce `proposal.created{kind: fulfill, target, target_revision, check_ids, evidence_ids}`. Deduplicate by target revision/check/evidence set. Acceptance revalidates target revision, dependency state, evidence coverage, and conflicts, then records acceptance and completion in one transaction. Rejection suppresses the same evidence-set proposal until evidence or target changes. Template status mappings (e.g. accepted send → submitted) are presented as part of the same proposed patch.

Explicit `complete`, `drop`, edits, captures, and workspace commands are user-authorized and record `command.accepted`; they do not require a redundant proposal round trip. An inferred summary, Braid suggestion, model-emitted MCP call, or unattended HA webhook is not user ratification. `ratify` offers accept/edit/reject with evidence and revision. Plans are recommendations; plan ratification records intent and never executes their next actions.

## 10. Deterministic planner

Rank actionable steps and display `parent › task › subtask`. A parent has no independent budget entry when it has children. Its display score is the maximum eligible descendant score. Local subtasks inherit task importance/impact, use their own due/estimate first, then task due. Eligible subtasks have fulfilled `after` prerequisites; by default only the first eligible open subtask in authored order is offered per task. Already-blocked/terminal tasks and blocked ancestors exclude their descendants. Leaf tasks without subtasks use explicit `next_action`.

Defaults: importance 3/5, no due → urgency 0, horizon 14 days, staleness 5 days, missing estimate → 30 minutes with `estimate_assumed:true`, daily budget 240 minutes. Explicit estimates must be positive. Date-only deadlines resolve to 23:59:59 in the configured timezone; calendar-day calculations and DST tests are required.

```text
urgency = clamp((horizon_days - days_until_due) / horizon_days, 0, 1)
impact  = sum(capacity[c] * impact[c] / 5)
score   = sum(weight[k] * component[k]) / sum(active_weights)
```

Weights default to importance .35, urgency .30, impact .35. Capacities default to income .4, skill .2, portfolio .3, relationships .1; validate finite nonnegative entries and positive totals, then normalize. Importance and urgency are always active; an absent entire impact object removes impact's weight. If the object is present, unspecified capacities contribute zero. Impact/importance values must be 0–5/1–5 respectively. No active positive weight is a configuration error.

Sort by score descending, then due ascending (missing last), then stale-before-fresh only on exact score/due ties, then stable task/subtask ID. Staleness is measured from the last qualifying attention/work event, falling back to creation. It does not add an undisclosed .05 that can reverse the ranking. Greedily include eligible steps that fit the remaining budget; skip oversize steps and list them separately. This is deterministic packing, not an optimal knapsack claim.

Plan JSON includes `as_of`, source event boundary, preference/template versions, component values, full paths, estimates, exclusions, selected items, and the highest-score/highest-urgency leaders plus impact leaders by capacity. Zero budget yields an empty selection. Inject `--now` in all time-dependent commands and tests. Persist an issued plan's exact output. Optional model rationale is a separate annotation/proposed wording change; it cannot reorder or mutate the deterministic result.

## 11. Retrieval integration

Use the current Braid subprocess contract described in [BRAID-CONTRACT.md](BRAID-CONTRACT.md), not the earlier discussion's hypothetical fuse API. No raw Heimdall events enter Braid's generic projector in v1. Heimdall materializes active searchable snapshots into an isolated, disposable Braid database generation using `upsert`; it rebuilds the whole generation on semantic changes that alter membership or relationships. This accommodates the current lack of node/edge deletion and avoids treating direct upserts as replayable truth.

Publish a generation only after all batches and validation succeed. Record the Heimdall source boundary, mapping version, Braid build fingerprint, configuration, and input digest. A generation must include exact accepted relationships; stale deleted edges cannot remain queryable after publication. Query results carry that boundary. For operations requiring current membership, wait for a matching generation or return `index_pending`; don't silently use a stale graph. Stop or failure of Braid disables suggestions without blocking capture, planning, or ratification.

Heimdall owns task-assignment policy: query Braid with capture text/anchors, collect evidence-backed task candidates, recheck active task membership, and create a proposal. A Braid fused score is not cosine similarity or a confidence probability; remove the fixed .62 threshold. Calibrate abstention and suggestion acceptance using held-out real task labels. Measure precision, recall@k, coverage, and rejection rate against lexical-only/manual baselines. Never train and evaluate on the same labeled surfaces. Synthetic fixtures validate correctness, not retrieval quality.

Defer new-initiative clustering; Braid currently does not provide it. Retrieval works lexically without a model. Dense embedding uses Braid's explicitly configured provider and reindex scheduling, independent of the summary provider. Reindex may send content remotely only under the configured policy. Query-time embeddings can also make provider calls when dense is enabled; provider absence means no such calls.

## 12. Notifications, interfaces, and data boundaries

Notifications: needs-you (credible blocked agent, due review, pending proposal threshold), info (issued plan/completion), drift (staleness threshold). Notification keys combine rule version, target, evidence episode, and local date when applicable. Repeated sensor refreshes do not resend. Persist queue, attempts, delivery outcome, snooze/dismiss state, and mute intervals. `mute --deep <duration>` records an expiring mute. Batch for 30 seconds, then recheck whether the need still exists. Focus suppression applies to info/drift; a focused agent waiting for permission still appears as needs-you. Mute suppresses delivery, not recording.

One sound owner per agent source; default herdr where installed, daemon otherwise. External adapters have at-least-once/ambiguous delivery semantics; UI should not promise exactly once. An unrecoverable ambiguous attempt is visible without blindly replaying a destructive action.

CLI: `init`, `start`, `doctor`, `fmt`, `ls`, `add`, `update`, `complete`, `drop`, `capture`, `state`, `open`, `close`, `focus`, `plan`, `ratify`, `mute`, `watch`, `export`, `import`, `replay`, `retrieval rebuild`, `browser-host`. Machine commands support `--json`; interactive `watch` and `ratify` require separate noninteractive flags for automation. `init` writes local defaults; `init --hooks`, `init --browser`, and `start --install-service` are explicit setup commands that merge existing settings safely and report remaining platform permission/trust steps. `init` alone does not claim to start a daemon.

Loopback endpoints: `/capture`, `/hook`, `/observations`, `/summary` (sensor ingestion, not required user forms), `/commands`, `/state`, `/plan`, `/events`, `/radiator`, `/health`. Enforce authentication, strict schemas, origin/Host checks, body/rate limits, and per-client roles. Sensor credentials cannot authorize `/commands`. Local CLI prefers restricted IPC. Browser-native content messages receive only the observation role. SSE supports Last-Event-ID and reconnect; serve UI and API from the same origin, keep token material out of URLs, and escape all captured text.

Radiator: needs-you, resident lanes with next action/attention/agent state, today's plan, drift and sensor gaps. Embedded HTML/JS uses a restrictive content policy. The bar and launcher read the same state API. Generate Hyprland rules for the tested installed release rather than freezing outdated rule syntax. Quickshell, walker, output name, and sound utilities are optional setup capabilities.

HA is M4 opt-in. Outbound notifications can work with a configured URL; phone callbacks require an explicitly configured authenticated TLS ingress/reverse proxy or a local bridge. Default loopback binding remains unchanged. Action buttons use expiring single-use tokens scoped to one action/target/revision, and create a user-action command only when invoked. Ordinary webhooks are observations, not authorization to drop tasks. No LAN listener is opened automatically.

Raw transcript/mail content is sensitive and also untrusted model input. Store content in separate blobs with digest/provenance in the event log; default local retention is 30 days after opted-in content setup, configurable per source. Metadata collection, transcript access, site permissions, and remote provider disclosure are distinct setup choices. Exclude private browsing, password fields, tokens, and unrelated files. Restrict blob paths to registered roots and avoid following untrusted path references. Purging content records an event, deletes blobs and disposable retrieval copies, and rebuilds the index; summaries retained in events may still contain sensitive information, so show that limitation in retention/export UI. Full account-history erasure is a separately designed destructive operation, not an append-only-log promise.

No credential copying, arbitrary command execution from captured text, or silent cloud transmission. Bounded queues, processing limits, and per-source health make partial coverage visible. No requirement for zero CPU or universal observation is made.

## 13. Repository layout

```text
heimdall/
  cmd/heimdall/
  internal/model/         typed commands/events/checks
  internal/store/         migrations, transactions, replay, outbox
  internal/tasks/         edit view, hierarchy, templates
  internal/capture/       human grammar and capture commands
  internal/checks/        evidence matching and timers
  internal/observe/       hyprland, browser, hooks, herdr, mail, repo
  internal/session/      content snapshots, transcripts, summary jobs
  internal/viewport/     operations, ownership, platform adapters
  internal/retrieval/    Braid subprocess and snapshot mapping
  internal/plan/         deterministic scoring and packing
  internal/notify/       durable rules/delivery
  internal/infer/        optional provider adapters
  internal/api/          local command and observation interfaces
  web/radiator/
  extension/             MV3 sensor, native bridge, site adapters
  contrib/               systemd, hooks, desktop setup, bar/launcher
  testdata/              synthetic fixtures, redacted source samples
  docs/                  spec, decisions, STATUS.md, compatibility ledger
```

## 14. Milestones and release gates

No fixed four-weekend promise. Ship vertical slices with evidence before expanding adapters.

| Milestone | Deliverable and acceptance gate |
|---|---|
| M0: contracts/spikes | Core schemas/examples; Braid protocol smoke; browser-native round trip and unique window pairing; hook capability samples. Failures narrow advertised coverage rather than blocking pure core work. |
| M1a: local core | Store/replay, command authorization, tasks/templates/check schema, parser, CLI, manual completion, parent proposals, timer review reminders. Replay byte-identical at pinned time; conflicting edits and crash/retry tests pass. |
| M1b: workspace | Browser native actions/inventory, Hyprland observer, generic terminal, optional herdr. Round-trip restore/diff; duplicate URLs/titles, moved tabs, WIP race, partial close, and crash recovery pass on Linux. |
| M2a: automatic conversation evidence | Claude.ai/ChatGPT browser adapters plus Claude Code/Codex capability-based hooks/transcripts; automatic summaries and artifact occurrences. No per-session manual activation; parser/site failure reported; inference-off path works. |
| M2b: fulfillment/notifications | Maildir then IMAP, send/reply matching and coverage-aware silence, repository evidence, notification dedupe/mute/recovery. Negative evidence cases and provider gaps do not complete tasks. |
| M3: planning/retrieval | Hierarchical planner, Braid generations, assignment proposals, held-out evaluation. No invented accuracy target or clustering delivery; report baseline metrics and coverage. |
| M4: packaging/adapters | Radiator/bar/launcher, export/restore, optional HA/MCP/vault, installation/upgrade/uninstall docs. Display gaps; tested source compatibility matrix accompanies release. |

Core M1a can begin from this specification. Production automatic browser content coverage, compositor closure, mail authentication, and native-app parity remain empirical integration gates. Claim no native-app content support until a tested data source exists.

## 15. Deferred work

Postgres/shared multi-device state, event buses, clustering/initiative generation, rich retrieval filters, incremental deletion in Braid, vector indexing at scale, historical Braid snapshots, context-injection proxies, general mail OAuth product distribution, and Windows/macOS viewport parity are separate work. Do not implement them as part of the core. Keep module/binary merging with Braid optional and later; repository independence remains explicit.
