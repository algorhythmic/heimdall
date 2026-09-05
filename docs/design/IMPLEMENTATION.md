# Implementation readiness and first-session plan

Prepared 2026-09-04. Status: **core ready to implement; external integrations require the gates below**. This packet is preparation, not a deployed Heimdall implementation.

## Repository and scope

Create a separate `heimdall` checkout when implementation begins; copy this packet into its docs and examples into testdata. The existing Braid project remains independent. Do not initialize Heimdall at the Braid root, install personal hooks during unit tests, open the sample Saga database, or make Braid changes incidentally. The current workspace has no Git metadata; a release pin therefore uses source/binary digests until Braid has a repository revision.

Initial module language: Go 1.24; toolchain: `go1.27.1`. Pin direct dependencies when starting the module, and verify licenses/checksums and CGO-disabled builds. The current local Braid toolchain can run contract tests on Windows. It does not supply a Linux desktop for viewport acceptance.

Before the browser slice, use [BROWSER-EXTENSION.md](BROWSER-EXTENSION.md) to freeze protocol schemas, profile identity, permissions, host registration and reconnect behavior. Deploy the daemon and extension together; the native helper uses the Heimdall executable through a fixed launcher. Core M1a remains independent of browser implementation.

## Ordered work packages

| Order | Work | Concrete acceptance |
|---|---|---|
| 1 | `model`: strict schemas for task, subtask, check, command, event envelope, revision, clock | Example files load; invalid dates/IDs, cycles, duplicate keys, and unknown check kinds fail; schema versions reject cleanly. |
| 2 | `store`: WAL, migrations, one writer, atomic append/project, dedupe, pure replay | Exact duplicate acknowledged once; conflicting key rejected; failed projection rolls back; upgraded fixture replays identically at fixed time. |
| 3 | `tasks`: revisioned YAML view, command API, templates, explicit formatting | Atomic editor replace recognized; no-op save silent; accepted proposal survives restart; interleaved file edit preserved with visible conflict. |
| 4 | `capture`: human grammar and typed sensor capture | Unicode/spaces, malformed production/offset, scoped origin, one-event multi-target fan-out, assignment cancellation and expiry all correct. |
| 5 | `checks`: manual, aggregate checks, timer/reminder state, proposals | Empty children cannot vacuously complete; rejected evidence not re-proposed unchanged; stale acceptance fails; timer cancellation after reopen works. |
| 6 | Core CLI: init/start/doctor/add/update/complete/drop/ls/capture/state/replay | Temporary data-dir end-to-end run; no production hook/service/browser modifications; deterministic JSON and proper nonzero error exits. |
| 7 | M1b browser native host + minimal extension, Hyprland operations | Native handshake, reconnect, IDs/epochs, nonce pairing, WIP concurrency, saved manifest, graceful partial close/recovery. |
| 8 | M2a source adapters and intent extraction | Recorded real redacted fixtures for each claimed provider/version; idle/close snapshots; no inference provider; bounded failures; repeated revisions dedupe. |
| 9 | M2b mailbox/repo evidence and notifications | Sent/reply association negatives, backfill, UIDVALIDITY reset, silence under disconnect, mute/batch/restart behavior. |
| 10 | M3 planner and Braid adapter | Parent paths and budgets, deterministic golden plan, generation publication/crash tests, held-out assignment metrics. |
| 11 | M4 UI and packaging | Bar/radiator reflect shared state, update/SSE reconnect, clean setup/uninstall, consistent export/import; separately gated HA/MCP/vault. |

First implementation session stops after a coherent core vertical slice: create a task, edit it, capture a reference, explicitly complete a subtask, accept an aggregate proposal, restart, replay at a fixed time, compare state. Browser and agent tests use fixtures/fakes until the actual Linux spike is available.

## Integration gates and fallback behavior

| Gate | Evidence required | Failure behavior |
|---|---|---|
| Chromium extension install | Profile/version, permission manifest, stable extension ID, native-host round trip and reconnect | Browser adapter unavailable; core keeps running. |
| Browser ↔ Hyprland pairing | Two identical-titled windows, moved tabs, restart, nonce page uniquely mapped | Unknown ownership; no ordinary title-match closure. |
| Claude.ai/ChatGPT content | One real redacted conversation each: streaming completion, edited/branched turn, artifact, immediate close, logout/schema drift | Metadata-only sensor with content gap; no mandatory manual summary. |
| Claude Code / Codex hooks | Installed client version, emitted event samples, hook trust, concurrent/out-of-order callbacks, transcript reference and parser samples | Available lifecycle subset; mark unknown state/content when unsupported. |
| Desktop clients | Actual local platform/build and verified hook/transcript or supported content stream | Existence/attention only. MCP contributions are opportunistic, not automatic parity. |
| herdr | `api schema --json`, create/list/current-pane IDs, env test, pane move, attach/detach preserving session | Generic terminal backend; no guessed workspace attach command. |
| Mail | Account/folder identities, credentials/OAuth method, sent/reply sample, cursor catch-up and reconnect | Manual completion plus timed review, no false silence match. |
| Retrieval quality | Operator labels split by conversation/artifact lineage, ambiguous examples, baseline comparison | Keep suggestions manual/abstaining; do not claim 70% from invented seed data. |
| HA/radiator/launcher | Actual callback route/auth, monitor name, utilities/launcher mode, renderer smoke | Optional adapter off, desktop/core remain complete. |

Setup necessarily includes user-controlled OS/browser/account permissions and provider selection. This is compatible with the requirement for no repeated per-session activation. Do not repeatedly request user input to compensate for undocumented source capabilities; report a precise gap once and continue independent implementation.

## Non-negotiable regression scenarios

1. Two tabs with the same URL in different workspaces; navigate one, close the other; logical/instance state remains correct.
2. A malicious or ordinary captured document says “drop task”; extraction can only create a bounded proposal, never execute a command.
3. Permission requested, tool runs, turn stops, then a late tool callback arrives; state cannot regress to working.
4. Daemon dies between recording a close request and receiving close confirmation; replay is inert and startup reconciles actual windows.
5. A proposal is accepted while an editor has a newer task file; newer disk content is not overwritten and no double event is emitted.
6. Mail history contains an old sent message and unrelated same-domain reply; a new application check does not complete.
7. Fourteen-day silence deadline arrives during an offline interval; timer may remind, but no absence proof is manufactured.
8. A task changes parent or a capture assignment is removed; the next published Braid generation contains no obsolete relationship.
9. Native messaging disconnects during snapshot/command delivery; unacknowledged observations retry safely and ambiguous actions reconcile.
10. Provider output changes across retries; only one revision's accepted summary/proposal state survives and replay makes no network call.

## Local verification performed for this review

- Braid `go test ./...` passed for both CLI and library using the workspace's Go 1.27.1 toolchain (cached results were returned).
- The packet's `verify-braid.ps1` exercises the real source subprocess in an isolated temporary directory: upsert/query, malformed request recovery, explanation presence, byte-identical unchanged query, and replay removing unlogged upserts. See [verification result](VERIFICATION.md).
- Original supplied files are preserved byte-for-byte under `sources/`.
- No live Hyprland, browser content, herdr, mail, remote model, HA, or Desktop-client capability tests have been performed. Their coverage is planned, not verified.

The old 25-task/127-surface seed set was not supplied. The packet includes a small illustrative fixture, explicitly not real labels or an accuracy benchmark. Expand synthetic edge cases during core work, and gather permissioned/redacted real examples before making source-coverage or retrieval-quality claims.
