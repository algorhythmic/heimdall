# Heimdall

Heimdall is a local task and continuity system for work shared between people and assistants. It records changes as events, preserves accepted decisions and progress checkpoints, and supplies scoped resume context. Retrieval belongs to the separate Braid project.

**Current development: terminal/editor continuity, reviewed application recovery, artifact versions and CLI/TUI progress review, extending 0.7.0 with database schema 20.** Resume an explicit task with its accepted direction, saved progress, blockers and file drift. Save progress through editable checkpoint drafts that preserve their original task, revision and retry identity.

The [Neovim integration](docs/NEOVIM-SETUP.md) brings that workflow into the editor. [Workspace/session records](docs/WORKSPACE-SETUP.md) keep tasks distinct even when they share a repository. The [Linux Herdr adapter](docs/HERDR-SETUP.md) verifies selected panes and publishes expiring task metadata. [Artifact versions](docs/ARTIFACT-SETUP.md) pin exact file identity, with optional Git metadata, to checkpoints; they detect changed or missing files and explicitly recorded relocations. They retain metadata and digests, not file contents.

Scoped MCP clients can read context and, with an explicit write grant, save progress. The terminal interface shows needs-you, workstreams, saved context and explicit review dialogs. Start with [terminal setup](docs/CONTINUITY-SETUP.md), [TUI setup](docs/TUI-SETUP.md), [evidence setup](docs/EVIDENCE-SETUP.md) or [MCP setup](docs/MCP-SETUP.md). Extension 0.6.0 adds recovery deduplication alongside challenged browser readback and shared action/attempt references. Initial [P02 progress/decision review](docs/PROGRESS-SETUP.md) is available through the CLI and the terminal interface, with Neovim inspection and terminal handoff; [P03 manual preservation](docs/PRESERVATION-SETUP.md) adds checkpoint-linked previews and independently observed copy/commit/remote receipts. [P04 task dependencies and scoped summaries](docs/DEPENDENCY-SETUP.md) expose cross-project prerequisites and saved progress. [W02 Hyprland observation](docs/HYPRLAND-SETUP.md) adds explicitly selected desktop inventories and task-owned window bindings. [W03 durable snapshots](docs/SNAPSHOT-SETUP.md) add manual restore points, explicit autosave policies and bounded payload retention. [W04 workspace previews](docs/WORKSPACE-PREVIEW.md) compare saved points with fresh owned observations and reject stale plans. [C12 shared actions](docs/ACTIONS-SETUP.md) preserve task-bound intent, attempts and uncertain outcomes before browser dispatch. [C13 browser verification](docs/BROWSER-VERIFICATION.md) checks independent outcomes and recovers retained results through a real native-host connection. [C13 browser pairing](docs/BROWSER-PAIRING.md) joins an explicit browser window to Hyprland before continuing navigation. [W05 workspace operations](docs/WORKSPACE-OPERATIONS.md) add journaled native focus and graceful close, explicit swaps and interruption reconciliation. [W06 application recovery](docs/APPLICATION-RECOVERY.md) adds reviewed Foot/Herdr, paired-browser and saved-file Neovim adapters. [W07 recovery verification](docs/RECOVERY-VERIFICATION.md) reports fresh ownership, membership, placement and supported application state with explicit uncertainty. Startup/interruption gates, unsupported attachment/editor readback, programmatic preservation, Braid integration, sensors, planning and notifications remain planned. See [STATUS.md](docs/STATUS.md) for the supported boundaries.

## Progress

| Improvement | Current development status |
|---|---|
| Durable checkpoints and context | Delivered: immutable checkpoints, contracts, decisions, resource drift, readable resume and draft/submit helpers. [Linux artifact/version pins](docs/ARTIFACT-SETUP.md) include optional Git identity. [CLI/TUI progress and decision review](docs/PROGRESS-SETUP.md) is delivered; evidence/run links remain open. |
| Workspace/session identity | Initial W01/T02 delivered: explicit manifests, generic declarations and verified local Herdr bindings with refresh/metadata. W02 adds observation/binding, W03 durable points/autosnapshots, W04 scoped previews, W05 journaled operations/capacity and W06 application adapters. W07 adds fresh recovery reports with explicit capability limits; W08 startup/interruption gates remain open. |
| Editor continuity | Initial T03 delivered: [Neovim commands and LazyVim example](docs/NEOVIM-SETUP.md) for task selection, resume, checkpoint drafts, artifacts, session checks and TUI review. Isolated Linux acceptance; no global configuration installed. |
| Assistant access through MCP | Initial implementation delivered: four tools, scoped credentials and explicitly delegated checkpoint writes. Host registration remains a deployment step. |
| Verified computer actions | C12/C13 and W05–W07 delivered: journaled actions, independent browser/Hyprland verification and scoped recovery reports. WCU records follow at S2a. |
| Evidence-based completion | Initial CLI implementation delivered: artifact/repo/test evaluators, durable attempts, invalidation and live revalidation of task/step proposals. Raw-output retention, broader machine tools and review notices remain open. |
| Agent continuity | Saved context and checkpoint handoff are delivered. Agents execute in external harnesses; scoped computer-use records follow at S2a. |
| Project-aware Braid memory | Planned; Braid is not integrated. Current mandatory context works without retrieval. |
| Task interface | Delivered: [terminal dashboard and review dialogs](docs/TUI-SETUP.md), replacing the browser GUI. Needs-you, expandable workstreams, retained progress drafts, file checks and workspace preview. |

See [implementation status](docs/STATUS.md), the [ordered backlog](docs/BACKLOG.md), and [development milestones](CHANGELOG.md). These are development milestones, not a complete v1 release.

## Runtime

The Go daemon owns the database, task state and authorization. The TUI, CLI commands and each MCP stdio adapter connect to it over authenticated loopback HTTP. The TUI uses local CLI authority. The optional browser extension communicates through a browser-launched native helper; it does not contain the daemon or database.

```text
CLI ----------------------------> Go daemon ------> SQLite event log
Assistant host --> MCP adapter ->     ^
Browser extension --> native helper --+
Terminal UI --> local CLI client -----+
```

Braid will remain a separate retrieval component when its adapter is implemented. No model runner or automatic task dispatcher is included yet.

## Build and run

From a fresh clone, install Go (language baseline 1.25; the module selects toolchain 1.27.1), then build from the repository root. The first build downloads the pinned modules and, if needed, the selected toolchain. Generated binaries are ignored by Git and are not included in a clone.

Windows PowerShell:

```powershell
go build -trimpath -o bin/heimdall.exe ./cmd/heimdall
```

Linux/macOS build command:

```sh
go build -trimpath -o bin/heimdall ./cmd/heimdall
```

Windows has historical acceptance; Linux core, continuity, MCP, evidence, TUI and isolated browser/native-worker checks pass locally. WCU also inspected terminal rendering on Hyprland. Actual native-host installation, workspace recovery and macOS acceptance remain open. Use `./bin/heimdall` on Unix.

Built executables do not require Go at runtime. `bin/heimdall` is the locally tested Linux build; generated binaries are ignored by Git.

Use a separate data directory for the example. On Linux:

```sh
./bin/heimdall init --data-dir ./demo-data
./bin/heimdall start --data-dir ./demo-data
```

On Windows:

```powershell
.\bin\heimdall.exe init --data-dir .\demo-data
.\bin\heimdall.exe start --data-dir .\demo-data
```

Leave `start` running. The following PowerShell example uses the Windows executable; on Linux, use `./bin/heimdall` and forward-slash paths. For the resume/draft workflow, follow [terminal continuity setup](docs/CONTINUITY-SETUP.md).

```powershell
.\bin\heimdall.exe import-tasks .\testdata\tasks.yaml --data-dir .\demo-data
.\bin\heimdall.exe add "Review the core" --id review-core --status active --data-dir .\demo-data
.\bin\heimdall.exe update review-core --title "Review the replay checks" --data-dir .\demo-data
.\bin\heimdall.exe capture "heimdall-core/reference: design notes" --pointer https://example.test/design --data-dir .\demo-data
.\bin\heimdall.exe complete "heimdall-core#store" --data-dir .\demo-data
.\bin\heimdall.exe complete "heimdall-core#tasks" --data-dir .\demo-data
.\bin\heimdall.exe ratify --data-dir .\demo-data
# Copy a returned proposal ID:
.\bin\heimdall.exe ratify <proposal-id> --accept --data-dir .\demo-data
.\bin\heimdall.exe replay --data-dir .\demo-data
```

The task fixture imports only at document revision 0 into a fresh store. For later imports, start with `export-tasks` and retain its current revision. Do not reset a document's revision just to bypass a conflict; merge against the current view.

Stop with Ctrl+C. No hooks, browser extension, system service, remote provider, or Braid process is installed or started by this build. Running the example uses synthetic tasks only.

## Terminal interface

Run `./bin/heimdall tui --data-dir ./demo-data` with the daemon running. Use
`node scripts/tui-demo.cjs` for an isolated working example of the new design.
[Controls and recovery](docs/TUI-SETUP.md) cover review, drafts and exact retries.

## Commands

For extension installation, use [Browser setup](docs/BROWSER-SETUP.md). The extension and daemon run together; the browser launches the native helper. Load `extension/` unpacked, or extract `bin/heimdall-extension-0.6.0.zip`. Native-host registration is a separate local installation step.

All commands accept `--data-dir PATH`, `--json`, and `--now RFC3339`. JSON is the default output except `export-tasks` (YAML) and `resume` (readable text unless `--json` is supplied). Global flags can occur anywhere; command-specific flags follow the title/target.

| Command | Behavior |
|---|---|
| `init` / `start` / `doctor` | Initialize without overwrite; run foreground daemon; report health and task-file errors |
| `ls` / `state [id]` / `events` | Task edit document in ID order; complete state or one task; event log |
| `add TITLE --id ID --type TYPE --parent ID --status STATUS` | Add a task; missing ID is allocated; workflow subtask defaults materialize |
| `update ID --title TITLE --status STATUS --next-action TEXT --resume-by DATE --parent ID --importance N` | Apply validated task changes at the current revision |
| `import-tasks FILE` / `export-tasks` | Apply a strict revisioned YAML document; emit current document |
| `capture LINE --pointer POINTER [--title TITLE] [--client ID] [--origin-id ID]` | Record one capture with task membership, optional scoped origin, and expiry |
| `assign CAPTURE --streams ID,ID` | Explicitly reassign; cancel obsolete expiry timers |
| `complete TARGET` / `reopen TARGET` / `drop TARGET` | User attestation; TARGET is task ID or `task#step` |
| `checks TASK` | Show matched/not_matched/unknown/unsupported check outcomes |
| `ratify [PROPOSAL --accept\|--reject]` | List pending proposals or explicitly decide one |
| `tick --now TIMESTAMP` | Process due timers; unavailable mail coverage produces a reminder, not fulfillment |
| `sync` / `fmt` | Ingest current task file explicitly; format an already accepted view |
| `replay` | Rebuild state and command dedupe from events; no external side effects |
| `contract accept\|show\|list TARGET` / `decision accept\|list TARGET` | Accepted continuity records; mutations require JSON `--file` and explicit `--expected-task-revision` |
| `resource bind\|unbind\|list TARGET` | Register or deactivate bounded file/tree observations |
| `progress summary TASK` / `progress summary --all` | Recorded checkpoints, next actions, unresolved decisions and prerequisites with explicit ordering and bounded pages; see [dependencies and summaries](docs/DEPENDENCY-SETUP.md) |
| `dependency add\|remove\|list\|show TARGET` | Explicit task prerequisites, exact revision/head checks and immutable relation history |
| `preservation preview\|request\|observe\|show\|list\|export TARGET` | Manual checkpoint-linked preservation and independently observed receipts; see [preservation setup](docs/PRESERVATION-SETUP.md) |
| `artifact record\|list\|show\|check TARGET` | Explicit local Linux file identity, immutable versions, optional Git metadata and fresh checks; see [artifact setup](docs/ARTIFACT-SETUP.md) |
| `checkpoint create\|show\|list TARGET` | Immutable progress checkpoints; create requires explicit contract and previous head; show supports `--id` |
| `checkpoint draft TARGET --output FILE` / `checkpoint submit TARGET --file FILE` | Prepare a new editable request with fixed revision/head/ID; submit after editing; preserve the file on retry or conflict |
| `resume TARGET [--budget N] [--json]` | Readable accepted direction, saved progress, drift and recorded review needs; does not execute work |
| `context TARGET --budget N` | Mandatory task/ancestor context and checkpoint drift checks; explicit budget error |
| `action context\|queue\|show\|list\|history\|cancel\|reconcile TASK` | Shared task-bound intent, attempt history, cancellation and observation-only reconciliation; API execution and verification remain separate |
| `workspace accept\|show TASK` | Accept a desired manifest or inspect its task-owned surfaces and bindings; acceptance requires explicit revision and JSON input |
| `workspace list\|diff\|preview\|validate TASK` | Scoped observations, selected manifest/snapshot comparison and stale-plan validation; see [workspace preview](docs/WORKSPACE-PREVIEW.md) |
| `session bind\|unbind\|show TASK` | Explicit generic session declarations with immutable history; declarations are unverified |
| `session bind-herdr\|refresh\|publish TASK` | Bind/check an actual local Herdr pane or publish expiring task metadata; see [Herdr setup](docs/HERDR-SETUP.md) for required identities |
| `backup --output FILE` | Consistent database-only snapshot with no-overwrite publication |
| `grant issue TARGET --name NAME --expires TIME --output FILE` | New private read credential; optional `--subtree` and `--resources ID1,ID2` |
| `grant activate --credential FILE` / `grant list` / `grant revoke ID` | Retry issuance exactly, inspect or revoke grants |
| `client task\|history\|context TARGET --credential FILE` | Scoped reads using the separate client credential; history supports `--kind`, `--limit`, `--cursor` |
| `grant issue ... --checkpoint-write` | Explicitly delegate checkpoint progress writes; old/read grants stay read-only |
| `client checkpoint TARGET --credential FILE --file FILE --expected-task-revision N --request-id ID` | Grant-authorized checkpoint with explicit retry identity and preconditions |
| `mcp --credential FILE` | Official-SDK stdio adapter; daemon must already be running |
| `tui [TARGET]` / `ui [TARGET]` | Open the terminal dashboard, optionally selecting/filtering a task or step; `--snapshot` prints it once |
| `evidence configure TARGET --file FILE --expected-task-revision N` | Accept a version-bound evaluator definition through the CLI |
| `evidence evaluate TARGET --evaluator ID --expected-task-revision N` | Commit an evaluation attempt, then run the configured observer/test asynchronously; use `--request-id` for exact retries |
| `evidence list TARGET` / `evidence refresh TARGET` | Inspect bounded result history; record invalidations for changed evidence inputs |
| `browser status` / `browser pair PROFILE` / `browser unpair PROFILE` | Inspect browser state; explicitly authorize/revoke a connected profile |
| `browser setup --extension-id ID --output DIR` | Prepare native host, exact-origin manifest and registration files in a new final installation directory |
| `browser open\|navigate\|focus\|move\|close --profile ID --epoch ID ...` | Queue a guarded operation; inspect its eventual result with `browser status` |

For API/CLI logical retries, `--request-id ID` returns the saved result for the same operation/body. The initial revision precondition is checked only on first execution. Reusing an ID with a different logical request fails. An imported document's revision and continuity commands' explicit revision/head preconditions remain part of their logical content.

`--now` is primarily for deterministic tests. A daemon started with `--now` freezes the scheduler clock too; do not use that flag for normal operation.

## Task files and recovery

The daemon is the sole database writer. A platform file lock prevents another daemon or `init` from taking the same directory. CLI commands use an authenticated loopback endpoint recorded in `endpoint.json`; the token stays out of normal output. The endpoint is local-only and has no browser CORS or remote access support.

Edit `tasks.yaml` and save. The watcher checks two stable reads, validates the entire document, and commits changes atomically. Missing IDs and new revision numbers are written back. Comments/formatting survive a semantic no-op save; explicit `fmt` reformats. Removing an active task is rejected; omitted terminal tasks stay in the archived edit view.

If a command races with an editor save, the command's event remains durable while the disk edit is preserved. `doctor` reports the conflict; `tasks.pending.yaml` contains the current accepted view for a manual merge. Publication uses atomic no-clobber hard links and retains detached originals in `task-file-history/`. On filesystems without hard-link support, publication fails visibly with recoverable files instead of overwriting. History is not pruned automatically in this slice; review it before sharing a data directory.

The prototype keeps `tasks.yaml`, `types.yaml`, SQLite and endpoint metadata together under `--data-dir`. It defaults to `$XDG_DATA_HOME/heimdall`, otherwise `~/.local/share/heimdall`. The full XDG config/state split and `config.toml` are future work. On Windows, access control follows the chosen directory's ACL; Unix file mode bits are not a substitute for Windows ACL hardening.

The current binary upgrades database markers 1–19 to **20**, publishing a consistent `backups/pre-schema-20-*.db` before migration. Older binaries refuse marker 20. Database backups preserve recorded state and receipts; external working files require separate preservation. Follow [backup, upgrade and restore](docs/CONTINUITY-SETUP.md#backup-upgrade-and-restore) for fresh-directory recovery or rollback.

## Develop

Go language baseline 1.25, tested toolchain 1.27.1. Direct libraries are the pinned official Go MCP SDK v1.7.0, YAML parser and pure-Go SQLite driver. Retained dependency notices are in [docs/licenses](docs/licenses/INDEX.txt).

```sh
go test ./...
go vet ./...
go build -o bin/heimdall ./cmd/heimdall
```

Extension unit tests require Node.js 24 and no npm packages:

```sh
node --test extension/test/*.test.js
```

After building `bin/heimdall` on Unix or `bin/heimdall.exe` on Windows, compiled integration checks use isolated synthetic data (Node 24):

```sh
node scripts/core-smoke.cjs
node scripts/native-smoke.cjs
node scripts/continuity-smoke.cjs
node scripts/resume-smoke.cjs
node scripts/workspace-smoke.cjs
node scripts/mcp-smoke.cjs
node scripts/evidence-smoke.cjs
node scripts/tui-smoke.cjs
```

Additional Linux gates cover artifact observations and installed integrations:

```sh
node scripts/artifact-smoke.cjs
node scripts/herdr-smoke.cjs
node scripts/neovim-smoke.cjs
```

The installed targets tested locally are Herdr **0.8.2 / protocol 20** and Neovim **0.12.5**, with an optional lazy.nvim loader gate. These checks use isolated data and configuration. See [Herdr setup](docs/HERDR-SETUP.md#compatibility-and-acceptance) and [Neovim setup](docs/NEOVIM-SETUP.md#reproduce-acceptance) for prerequisites and limits. Fresh artifact and live Herdr observations currently require Linux; Windows cross-build success does not establish those runtime capabilities.

Set `HEIMDALL_BIN` to an explicit executable and `HEIMDALL_TEST_TMP` to a scratch parent when needed. The harness closes child stdin and bounds shutdown on Unix; test data remains for inspection. The old PowerShell core smoke remains available.

TUI development uses Go and tcell v2. Run `go test ./internal/tui` and `node scripts/tui-smoke.cjs`; Linux runs the compiled interface in a real PTY. Browser-extension test dependencies live separately under `scripts/browser-test/`; the task interface has no frontend build.

CI runs Go tests/vet/build, extension checks and compiled core/native/continuity/resume/workspace/progress/MCP/evidence/TUI checks on Windows and Ubuntu. Linux additionally runs artifact and real-PTY checks. See [verification notes](docs/VERIFICATION.md) for local and historical acceptance.

On Windows, `scripts/dev.ps1` can use `HEIMDALL_GO`, Go on PATH, a local `.tools/go`, or the already-installed sibling Braid toolchain. This is a development convenience, not a runtime dependency or import from Braid.

```powershell
.\scripts\dev.ps1 test ./...
.\scripts\dev.ps1 vet ./...
.\scripts\dev.ps1 build -o bin/heimdall.exe ./cmd/heimdall
```

[Implementation specification](docs/design/HANDOFF-heimdall-v1-r4.md) · [Browser runtime design](docs/design/BROWSER-EXTENSION.md) · [Verification](docs/VERIFICATION.md).

## Roadmap

| Slice | Scope | Planned schema |
|---|---|---|
| P0 | W07 clock repair, r4 adoption, evaluator environment, YAML check materialization, browser deltas and daily-profile pairing | 20 |
| S2a (R5/A01/A02) | Scoped computer-use intents, action grant, reports and fresh reconciliation | 21 |
| S1 | Observed surfaces, hooks, conversations, herdr sensors, focus spans; S1b adapter spike | 22 |
| S2b (R6/W08) | Startup readiness, interruption/reboot recovery, optional login restore | no reserved bump |
| S3 (C14–C16) | Braid assignment and continuity retrieval, typed checkpoint MCP records, optional intent extraction | 23 |
| S4 | Planner, notifier, configuration/preferences and TUI views | 24 |
| S5 (C21) | Mail, Codex/Desktop adapters, packaging and fresh-install replay | 25 |

Delivery order: **P0 → S2a → S1 → S2b → S3 → S4 → S5**. S-numbers are
stable labels, not numeric execution order. S2a and all later slices remain unstarted.

**Agent execution is out of scope for Heimdall.** Agents run in Claude Code,
Codex and herdr; desktop input runs through WCU under its MCP host's approval.
Heimdall supplies accepted context, records scoped actions and reports, and
verifies outcomes from its sensors. No run state machine, dispatch outbox,
leases, fencing, resource locks, execution limits or execution-host adapter will
be added. Checkpoint handoff/resume is built; wake conditions and deduplicated
attention belong to the S4 notifier.

Historical C/T/P/W/R identifiers and their dispositions are preserved in [the backlog](docs/BACKLOG.md).
