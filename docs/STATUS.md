# Implementation status

2026-09-04 — MCP and scoped checkpoint writes (0.5.0 daemon; 0.2.0 extension). This is a development build, not the complete Heimdall v1 release.

## Implemented and tested

- Official Go MCP SDK v1.7.0 stdio adapter with task/context/history/checkpoint tools. Official SDK protocol 2026-07-28 and compiled legacy 2025-11-25 checks pass. The adapter rediscovers the daemon port and never opens the database. See [MCP-SETUP.md](MCP-SETUP.md).
- Explicit checkpoint-write grants, authenticated client provenance, authorization before retry lookup and commit, rollback on lost authorization, and revoked/expired/cross-grant retry denial. Progress is saved in immutable checkpoints and cannot change task completion or accepted contracts.
- Separate expiring/revocable credentials, exact-task/subtree access, explicit binding permissions for live observations, bounded history/response sizes, cursor scope checks and public endpoint discovery. Default/read credentials cannot write; scoped credentials cannot reach CLI/browser routes.
- Version-2 contracts freeze explicitly reviewed resource IDs and reject scope changes. Version-1 contracts remain replayable but require review before new checkpoints. Golden event fixtures exercise both versions plus grant issue/revoke.
- CLI-authored accepted task/step contracts and decisions, explicit supersession, canonical file/tree bindings, immutable checkpoints with atomic head preconditions, and deterministic mandatory context without retrieval. Task/ancestor changes, working-file drift, blockers and unavailable resources are visible. See [CONTINUITY-SETUP.md](CONTINUITY-SETUP.md).
- Versioned continuity events and request fixtures, exact-request retry, competing-head rejection, replay/restart equality, bounded resource reads and explicit small-budget errors. Replay performs no filesystem observations.
- Database marker 5, consistent pre-upgrade backups, exclusive live database backups, and fresh-directory recovery. A compiled 0.4.0 database upgraded with task/contract/checkpoint and read-grant retention and rolled back from its pre-upgrade snapshot successfully. Its existing read credential still refused checkpoint writes after upgrade.
- MV3 extension with explicit profile pairing, ordinary HTTP(S) tab inventory/focus, popup pause/connection status, browser epochs, bounded IndexedDB outbox, reconnect and command journal.
- Compiled native helper with bounded framing and exact-origin config; browser-only daemon credential; replayable browser observations and command results.
- CLI open/navigate/focus/move/close. Existing tabs require recorded Heimdall ownership, current epoch and exact URL. Setup prepares native-host registration artifacts without installing them.
- Real Chromium extension/API checks and worker-to-compiled-daemon tests. The latter substitute a test native port for OS registry discovery; normal Chrome/Edge registration and Linux desktop acceptance remain open.

- Strict YAML task/workflow parsing: unknown and duplicate keys, document versions, IDs, status lifecycles, parent/prerequisite cycles, dates, importance/estimates, typed completion checks and anchors.
- SQLite/WAL event transactions with command dedupe, conflict rejection, one OS-locked writer, atomic projection updates, and pure replay. Unknown event/database versions fail safely. Accepted commands preserve exact results across replay.
- Task create/update/import, generated IDs, workflow materialization, manual step/task completion and reopening/drop, revision checks, and stable task serialization.
- Task-file polling handles atomic editor replacement. No-op saves do not emit task changes. Conflicting edits are preserved, with a pending view and recoverable originals; publication never replaces a concurrently recreated path.
- Capture grammar with Unicode/spaces and structured parse errors, one-event multi-target membership, client-scoped origins, reassignment, candidate/unassigned/study deadlines, and expiry history.
- Non-vacuous children/subtasks completion proposals, explicit accept/reject, evidence-set dedupe, stale/superseded proposals, and cancellation after reopening anchors.
- Silence-review scheduling from user-attested step completion. No mail coverage means unknown fulfillment and a review reminder, even after its deadline.
- Authenticated loopback daemon and CLI, strict request decoding, origin/Host rejection, limited request sizes, endpoint validation, graceful shutdown, and a stable-read task watcher.

## Deliberate boundaries and deviations to the full design

| Area | Current decision |
|---|---|
| Projection layout | One versioned JSON state projection and command-dedupe table initially; event truth is unchanged. Normalize into per-entity SQL tables when query/load requirements justify it. |
| Subtask events | Step mutations are captured inside a task event with the changed `subtasks` field, task revision and per-step completion token; separate subtask event rows are not emitted yet. |
| Event migration | Event envelope remains version 1. Database markers 1–4 upgrade to 5 after a consistent snapshot. Contracts support v1/v2; grants and checkpoints support old read/CLI v1 and scoped-write/client v2. Old binaries refuse marker 5. |
| Continuity scope | Contracts and accepted decisions remain CLI-authored. Proposed/rejected decisions, Git identity and evidence/run links remain open. Explicit client writes save progress checkpoints only. `ready` is not execution authority. |
| Local IPC | Native messaging forwards to authenticated random-port loopback HTTP using a separate browser role. Private Unix IPC is deferred. No remote listener or credential is exposed to the extension. |
| Configuration | Co-located data files and compiled timer defaults. Full `config.toml`, config/state directory split, configurable TTLs/timezone/preferences, and permissions installation remain open. Core deadlines currently use UTC; DST-aware configured scheduling is not claimed. |
| Workflows | Templates materialize on add, and each task event stores its workflow. Live catalog editing/migration and template-driven status proposals are not implemented. Restart loads a valid catalog for new commands; existing tasks retain materialized workflow metadata. |
| Task edit views | History-backed no-clobber publication rather than a plain check-then-overwrite rename; hard-link support required. Unsupported filesystems leave a visible pending view. History pruning is future work. |
| Idempotency | Request IDs identify logical commands; automatically refreshed revision preconditions are excluded from the intent hash. The first execution validates its precondition; retries return the recorded result. |
| Check evaluation | Manual commands and task-level aggregate checks work. Mail/repo/agent/GitHub kinds validate but report unsupported; silence is unknown without coverage. Arbitrary automatic checks attached to subtasks are not evaluated yet. |
| Capture kind `task` | Recorded safely; next-action/subtask proposal synthesis is future work. It cannot mutate a task unattended. |
| User interfaces | `ls` currently returns tasks in stable ID order. Ranked planning, live watch/SSE, radiator, bar and launcher are not implemented. |
| Portability | Windows daemon/native helper and isolated Chromium APIs tested; Ubuntu CI passes Go tests/vet/build and extension unit tests. Linux CGO-disabled cross-build supplied. Linux desktop/native-host installation and Hyprland deployment remain unverified. |

These boundaries keep the first slice inspectable. They do not silently redefine the final specification's completion criteria.

## Next work

The seven-improvement sequence is in [IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md), with precise progress in [BACKLOG.md](BACKLOG.md). Continuity, scoped checkpoint writes and the MCP adapter now work. Next: evidence evaluation and completion revalidation. Remaining decision/Git identity details, evidence, the task GUI, verified browser actions, Braid and optional execution-host coordination remain open.

1. Finish remaining M1a configuration/catalog lifecycle and per-subtask check evaluation; specify and test workflow status proposals. Add transactional export/import with secret exclusion.
2. Complete native-registration acceptance in the user's intended Chrome/Edge installation, then Linux/Hyprland window pairing, multi-profile deployment and graceful-close acceptance. See [BROWSER-SETUP.md](BROWSER-SETUP.md).
3. Add automatic conversation source adapters, capability-based agent hooks and transcript ingestion. No content coverage until redacted real-source fixtures pass.
4. Add mailbox coverage/evidence, notifications, hierarchical planner, and Braid snapshot generations, followed by UI and deployment adapters.

The separate Braid source and databases were not changed. No hooks, extension, system service, account integration, inference provider, or background user automation was installed during implementation.
