# Heimdall

Heimdall is a local task and continuity system for work shared between people and assistants. It records changes as events, preserves accepted decisions and progress checkpoints, and supplies scoped resume context. Retrieval belongs to the separate Braid project.

**Current build: local task and evidence GUI (0.7.0).** The daemon and CLI provide contracts/decisions, reviewed resource scopes, checkpoints, resume context, scoped MCP and independently observed artifact/repository/test evidence. The browser GUI shows scoped tasks, saved progress and evidence, and lets you explicitly review completion proposals with live revalidation. The extension remains at 0.2.0; Braid integration and automatic continuation remain future work. Start with [GUI setup](docs/GUI-SETUP.md), [evidence setup](docs/EVIDENCE-SETUP.md) or [MCP setup](docs/MCP-SETUP.md); see [STATUS.md](docs/STATUS.md) for boundaries.

## Progress

| Improvement | Status in 0.7.0 |
|---|---|
| Durable checkpoints and context | Initial implementation delivered: immutable checkpoints, contracts, decisions, resource drift and resume context. Decision review, Git identity and evidence/run links remain open. |
| Assistant access through MCP | Initial implementation delivered: four tools, scoped credentials and explicitly delegated checkpoint writes. Host registration remains a deployment step. |
| Verified computer actions | Planned. Browser tab controls exist; API success does not yet verify the intended outcome. |
| Evidence-based completion | Initial CLI implementation delivered: artifact/repo/test evaluators, durable attempts, invalidation and live revalidation of task/step proposals. Raw-output retention, broader machine tools and review notices remain open. |
| Persistent task continuation | Planned. Saved context supports resuming work; dispatch, leases, recovery and an execution-host adapter remain open. |
| Project-aware Braid memory | Planned; Braid is not integrated. Current mandatory context works without retrieval. |
| Task GUI | Initial implementation delivered: scoped task/step navigation, checkpoints, accepted direction, evidence and explicit completion review. Task editing, decision review and run controls remain open. |

See [implementation status](docs/STATUS.md), the [ordered backlog](docs/BACKLOG.md), and [development milestones](CHANGELOG.md). These are development milestones, not a complete v1 release.

## Runtime

The Go daemon owns the database, task state and authorization, and serves the embedded TypeScript GUI. CLI commands and each MCP stdio adapter connect to it over authenticated loopback HTTP. The GUI uses a separate scoped browser session. The browser extension runs alongside the daemon and communicates through a browser-launched native helper. The extension does not contain the daemon or database; the GUI works without the extension.

```text
CLI ----------------------------> Go daemon ------> SQLite event log
Assistant host --> MCP adapter ->     ^
Browser extension --> native helper --+
Browser GUI --> scoped UI session ----+
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

Windows is the locally tested runtime. Ubuntu CI also passes Go tests, vet, build and extension unit tests; Linux desktop/native-browser and macOS runtime acceptance remain open. For the examples below, use `./bin/heimdall` on Unix.

Built executables do not require Go at runtime. The local `bin/heimdall-linux-amd64` artifact is a CGO-disabled cross-build, not a Linux-tested release.

Use a separate data directory for the example:

```powershell
.\bin\heimdall.exe init --data-dir .\demo-data
.\bin\heimdall.exe start --data-dir .\demo-data
```

Leave `start` running. In another terminal:

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

## Commands

For extension installation, use [Browser setup](docs/BROWSER-SETUP.md). The extension and daemon run together; the browser launches the native helper. Load `extension/` unpacked, or extract `bin/heimdall-extension-0.2.0.zip`. Native-host registration is a separate local installation step.

All commands accept `--data-dir PATH`, `--json`, and `--now RFC3339`. JSON is the default output except `export-tasks`, which emits YAML. Global flags can occur anywhere; command-specific flags follow the title/target.

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
| `checkpoint create\|show\|list TARGET` | Immutable progress checkpoints; create requires explicit contract and previous head; show supports `--id` |
| `context TARGET --budget N` | Mandatory task/ancestor context and checkpoint drift checks; explicit budget error |
| `backup --output FILE` | Consistent database-only snapshot with no-overwrite publication |
| `grant issue TARGET --name NAME --expires TIME --output FILE` | New private read credential; optional `--subtree` and `--resources ID1,ID2` |
| `grant activate --credential FILE` / `grant list` / `grant revoke ID` | Retry issuance exactly, inspect or revoke grants |
| `client task\|history\|context TARGET --credential FILE` | Scoped reads using the separate client credential; history supports `--kind`, `--limit`, `--cursor` |
| `grant issue ... --checkpoint-write` | Explicitly delegate checkpoint progress writes; old/read grants stay read-only |
| `client checkpoint TARGET --credential FILE --file FILE --expected-task-revision N --request-id ID` | Grant-authorized checkpoint with explicit retry identity and preconditions |
| `mcp --credential FILE` | Official-SDK stdio adapter; daemon must already be running |
| `ui ROOT_TASK` | Print a token-free local GUI URL and single-use sign-in code for a root task's subtree; daemon must already be running |
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

After building `bin/heimdall.exe`, Windows integration checks exercise the compiled daemon and MCP adapter against synthetic data:

```powershell
.\scripts\smoke.ps1
node scripts/native-smoke.cjs
node scripts/continuity-smoke.cjs
node scripts/mcp-smoke.cjs
node scripts/evidence-smoke.cjs
node scripts/gui-smoke.cjs
```

GUI development uses `npm ci --ignore-scripts --no-audit --no-fund` and `npm run build` in `web/`. Commit the generated `internal/webui/assets/app.js`; a fresh Go-only build embeds it without requiring Node at runtime. Install the test browser with `npx playwright install chromium` in `web/` before running the GUI smoke.

CI runs Go tests/vet/build, TypeScript build/generated-file checks and extension unit tests on Windows and Ubuntu, plus compiled continuity/MCP/evidence and Chromium GUI checks on Windows. See [verification notes](docs/VERIFICATION.md) for their limits and historical results.

On this Windows workspace, `scripts/dev.ps1` can use `HEIMDALL_GO`, Go on PATH, a local `.tools/go`, or the already-installed sibling Braid toolchain. This is a development convenience, not a runtime dependency or import from Braid.

```powershell
.\scripts\dev.ps1 test ./...
.\scripts\dev.ps1 vet ./...
.\scripts\dev.ps1 build -o bin/heimdall.exe ./cmd/heimdall
```

[Implementation specification](docs/design/HANDOFF-heimdall-v1.1.md) · [Browser runtime design](docs/design/BROWSER-EXTENSION.md) · [Verification](docs/VERIFICATION.md).

C08/C09 and C10/C11 now have initial evidence and GUI implementations. Development is stopped at the requested documentation/commit/push checkpoint. Remaining work is recorded in the [seven-improvement implementation plan](docs/IMPLEMENTATION-PLAN.md) and [dependency-ordered backlog](docs/BACKLOG.md).
