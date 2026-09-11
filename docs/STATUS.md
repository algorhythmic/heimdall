# Implementation status

2026-09-10 — Working tree builds on HEAD `d3e4694` with schema 22 and extension
0.6.1. S2a is delivered; S1 browser surface observations and attention are implemented.
[Handoff r4](design/HANDOFF-heimdall-v1-r4.md) supersedes prior plans.
Historical baseline: `2f11175`, schema 20. P0 implementation and
verification are recorded in [P0 verification](P0-VERIFICATION.md); machine gates remain open.

## Implemented and tested

- S1 browser observed surfaces: task-independent content catalog, profile/epoch/tab occurrences and atomic observed/opened/closed/changed events. Tests cover replay, navigation, alias URLs, identity gaps, partial coverage, source epochs, task/action isolation and schema-21 backup/upgrade/refusal/rollback. Browser focus spans and `state --active` are also implemented and pass isolated Chromium worker acceptance; compositor attention, other sensors and UI remain pending. See [S1 implementation](S1-IMPLEMENTATION.md).

- Herdr agent observation: the daemon polls the live `agent.list` inventory of every active session-bound Herdr socket and records epoch-scoped `agent.observed`/`agent.detached` events keyed by host/epoch/socket/pane. The TUI header shows live working/blocked/idle counts, workstream rows show per-task agent status dots, needs-you lists blocked agents and unbound repo-cwd agents, and the selected context lists attributed agents. Task attribution derives at read time from bound panes and bound resource roots; the recorder never assigns ownership. Sequence-regression, duplicate, forged-envelope and detach-order rejection are tested.

- W07 recovery verification: CLI/TUI per-surface and aggregate reports distinguish action settlement from current existence, exact ownership, task/named-workspace membership, logical placement and supported application/window state. Explicit floating monitor fallback is reported, never dispatched. Wrong move/session, failed close, PID reuse, stale coverage and unsupported attachment/editor state cannot produce full recovery. Settled browser surfaces use bounded challenged readback with live monotonic leases. See [recovery verification](RECOVERY-VERIFICATION.md).

- W06 reviewed application recipes: digest-pinned Foot launch, exact new process/window association, Herdr direct-terminal attachment and observed session-preserving detach, paired-browser recovery with self-restored URL deduplication, and structured saved-file Neovim launch. Editor close remains unsupported to preserve unsaved buffers. W07 reports placement and capability limits; attachment rendering and editor buffer/cursor readback remain unsupported. See [application recovery](guides/APPLICATION-RECOVERY.md).

- W05 journaled workspace operations: explicit open/focus/close, status/cancel/reconcile, resident capacity and named swaps, one retained diff and pre-close snapshot, shared native attempts and bounded independent focus/closure observations. Real isolated GTK/Hyprland Lua-provider acceptance and synthetic daemon-kill-after-close acceptance pass. W06 adds application launch/detach; W07 separately reports fresh recovery postconditions. See [workspace operations](guides/WORKSPACE-OPERATIONS.md).

- C13 browser verification slice: challenged, monotonic-bounded double readback; explicit exact URL/load, focus, membership and closure postconditions; retained-result recovery without input retry; bounded reconciliation and epoch-loss uncertainty. Actual Linux Chromium native registration is tested in an isolated profile. Explicit nonce-based compositor association is delivered; see [browser pairing](guides/BROWSER-PAIRING.md). See [BROWSER-VERIFICATION.md](guides/BROWSER-VERIFICATION.md).

- C12 shared action journal with a browser consumer: task/manifest/context pins, CLI authority reference, immutable attempt identity, dispatch observation references, separate execution/verification and late-result history. Surface serialization and unfinished snapshot protection prevent new-ID retries after uncertain input. Queued/in-flight cancellation, deadline sweeps, reconnect/restart, strict wire references, legacy unverified history and schema-15 upgrade/rollback are tested. See [ACTIONS-SETUP.md](guides/ACTIONS-SETUP.md). C13 now adds independent postconditions and actual isolated Linux native-host acceptance; compositor association is delivered.

- W04 scoped live List/Diff and explicit manifest/snapshot previews: deterministic dispositions, protected-point/age diagnostics, fresh Herdr readback, exact ownership, display/epoch uncertainty and daemon-sealed stale-plan detection. Preview performs no durable mutation or application action. TUI workspace details and explicit retained-request point capture are delivered. CLI, scope, forged/expired/restarted plan, race and PTY checks pass. See [WORKSPACE-PREVIEW.md](guides/WORKSPACE-PREVIEW.md). W04 used schema 15; C12/W05–W07 subsequently delivered dispatch and scoped recovery verification.

- W03 durable workspace snapshots: immutable payloads and indexed history in the same SQLite database, atomic event/receipt/head publication, explicit per-task autosave policies, bounded current projection, manual pins, retained/pruned history and freshness diagnostics. Empty, partial and unavailable observations preserve the last complete point. Local Btrfs process-kill/publication, allocation failure, replay, backup, schema-14 upgrade/refusal/rollback and measured 24-hour capture volume pass. See [SNAPSHOT-SETUP.md](guides/SNAPSHOT-SETUP.md). Controlled VM power loss and actual compositor/reboot recovery remain later release gates.

- W02 Linux Hyprland 0.56.2 observation: explicit persistent source selection, same-user peer/process/socket epochs, buffered event subscription, double-inventory bootstrap and periodic reconciliation. Native stable window IDs prevent address reuse; unmatched windows remain unowned. Explicit viewport bindings join task/surface declarations, with separate unverified terminal/pane attachment diagnostics. Schema-14 migration/replay/retry and stopped schema-13 rollback pass. See [HYPRLAND-SETUP.md](guides/HYPRLAND-SETUP.md). W03 snapshots and W04 preview are delivered; C13 pairing and W05–W07 recovery subsequently consumed this foundation.

- P04 task dependencies: immutable add/remove chains, exact endpoint revisions, cycles checked with hierarchy and atomic task edits/imports, and derived satisfaction without task mutation or dispatch. Scoped progress summaries show latest saved checkpoints, next actions, recorded blockers, unresolved decisions and prerequisites with explicit ordering/pagination. Foreign prerequisites are opaque; scope movement/revocation and cursor isolation are tested. TUI selected context shows recorded dependency status. See [DEPENDENCY-SETUP.md](guides/DEPENDENCY-SETUP.md).

- Initial P03 manual preservation: exact checkpoint/artifact selection, read-only preview, retained handoff requests, separate source/mirror/private-commit/remote readback, reported failure outcomes, exact receipt retries, and portable progress export. Schema-12 upgrades/replay retain old authority and history. Unrelated private changes and rebase conflicts are visible; no dotprivate copy, Git write or remote push is dispatched. See [PRESERVATION-SETUP.md](guides/PRESERVATION-SETUP.md). Programmatic preservation remains gated by C12 and a narrow upstream interface.

- Initial P02: immutable decision/artifact proposals, explicit reviewed/accepted/rejected outcomes, exact digest/contract/version binding, stale input refusal and separate artifact lifecycle. Accepted decisions remain mandatory context; unresolved proposals are shown separately in CLI resume. Review never completes tasks or steps. Schema-11 upgrade/replay/retry/restore and completion evidence compatibility are tested. See [PROGRESS-SETUP.md](guides/PROGRESS-SETUP.md). Native terminal review and Neovim proposal inspection/terminal handoff are delivered. TUI proposal authoring and agent proposal grants remain open.

- Initial P01: task/environment/host-owned artifact IDs, immutable local Linux file versions, optional Git input identity and explicit checkpoint pins. Changed bytes, missing originals and declared relocation are distinct; drafts retain their pins. Schema-9 upgrade, legacy authority, pure replay, restart receipts and backup restore are tested. File contents are not retained. See [ARTIFACT-SETUP.md](guides/ARTIFACT-SETUP.md).

- Initial T03: repository-owned Neovim commands for explicit task/step selection, structured resume, checkpoint draft/edit/submit/reopen, bound artifact opening, explicit session checks and selected-target TUI handoff. Isolated Neovim acceptance covers task switches, delayed responses, conflicts, renamed/externally changed drafts, restart retries, literal filenames, resource boundaries and unavailable daemon/Herdr. The lazy.nvim example loads with installs/updates disabled; WCU inspected the resume view. See [NEOVIM-SETUP.md](guides/NEOVIM-SETUP.md). No global editor configuration was changed.

- Initial T02: actual Herdr 0.8.2 / protocol-20 Linux bindings, peer/boot/socket epochs, canonical cwd/Git identity, explicit refresh/rebind, stale/disconnected refusal and expiring display metadata. Installed tests cover movement, source restart, same-repository isolation, metadata readback/expiry and replay; WCU verified the optional pane-labelled sidebar summary. [HERDR-SETUP.md](guides/HERDR-SETUP.md) records commands and limits. No background publisher or automatic pane adoption.

- W01 foundation: versioned desired manifests, permanently task-owned logical surfaces, immutable generic session declarations and explicit accept/bind/show/unbind CLI commands. Current views report unverified bindings and changed task/manifest preconditions. Same-cwd scope, pane collisions, concurrent heads, strict replay and restart/backup checks pass. See [WORKSPACE-SETUP.md](guides/WORKSPACE-SETUP.md); W02–W07 subsequently added observations, snapshots and recovery; initial Herdr metadata was delivered separately in T02.

- Readable `resume TARGET`, with accepted direction, checkpoint age/next action, blockers, resource drift and bounded recorded review counts; explicit JSON output and unchanged existing `context` format. Reads do not mutate state. Captured terminal controls and bidi format characters are escaped.
- `checkpoint draft TARGET --output FILE` and `checkpoint submit TARGET --file FILE` preserve the original contract, revision, head and request ID through explicit editing, conflicts and restart retries. Draft creation refuses overwrite; failed or successful submission retains the file. These use existing CLI authority and schema-6 checkpoint events.
- Linux Go tests/vet/build, portable compiled core/native/continuity/MCP/evidence/resume checks, TypeScript/extension checks and Chromium GUI/browser/worker acceptance pass. WCU visually inspected the synthetic terminal workflow on Hyprland. A real 0.7.0 schema-6 fixture checks compatibility, grant limits, receipts and replay. See [VERIFICATION.md](VERIFICATION.md) for versions and limits; daily-profile native-host registration remains a deployment gate; W05–W07 implement scoped workspace recovery.

- Native Go TUI replaces the browser interface: needs-you, sorted/expandable workstreams, selected context, compact layout, retained progress drafts and explicit review/file/workspace/binding dialogs. Browser frontend assets and sign-in/session routes are removed. See [TUI-SETUP.md](guides/TUI-SETUP.md).
- Terminal review uses existing local CLI authority and backend revalidation. Simulation and compiled PTY checks cover stale decisions, uncertain exact retries, retained drafts/conflicts, keyboard navigation, resize and terminal restoration. Historical GUI-v2 records retain replay support under schema 11; no live GUI authority remains.
- CLI-configured artifact/repo/test evaluators, durable attempts, observed execution provenance and bounded output digests. Unknown interrupted requests never relaunch on retry or replay. Definitions/results are tied to contracts, reviewed resource scope, task/ancestor versions and accepted decisions. See [EVIDENCE-SETUP.md](guides/EVIDENCE-SETUP.md).
- Task/step completion proposals from observed evidence, explicit invalidation, supersession and live input/repository/executable/environment revalidation inside the writer transaction before ratification. Existing completed history is preserved; raw-output retention and richer review notices remain open.

- Official Go MCP SDK v1.7.0 stdio adapter with task/context/history/checkpoint tools. Official SDK protocol 2026-07-28 and compiled legacy 2025-11-25 checks pass. The adapter rediscovers the daemon port and never opens the database. See [MCP-SETUP.md](guides/MCP-SETUP.md).
- Explicit checkpoint-write grants, authenticated client provenance, authorization before retry lookup and commit, rollback on lost authorization, and revoked/expired/cross-grant retry denial. Progress is saved in immutable checkpoints and cannot change task completion or accepted contracts.
- Separate expiring/revocable credentials, exact-task/subtree access, explicit binding permissions for live observations, bounded history/response sizes, cursor scope checks and public endpoint discovery. Default/read credentials cannot write; scoped credentials cannot reach CLI/browser routes.
- Version-2 contracts freeze explicitly reviewed resource IDs and reject scope changes. Version-1 contracts remain replayable but require review before new checkpoints. Golden event fixtures exercise both versions plus grant issue/revoke.
- CLI-authored accepted task/step contracts and decisions, explicit supersession, canonical file/tree bindings, immutable checkpoints with atomic head preconditions, and deterministic mandatory context without retrieval. Task/ancestor changes, working-file drift, blockers and unavailable resources are visible. See [CONTINUITY-SETUP.md](guides/CONTINUITY-SETUP.md).
- Versioned continuity events and request fixtures, exact-request retry, competing-head rejection, replay/restart equality, bounded resource reads and explicit small-budget errors. Replay performs no filesystem observations.
- Database marker 21 delivers S2a native/browser intents, reports, WCU corroboration and checkpoint/completion references; schema-20 upgrade/refusal/rollback acceptance passes. The P0 baseline used marker 20, stopped schema-6 through schema-19 fixture migrations and historical actual schema-11/schema-10/0.7.0 upgrade/refusal/rollback, consistent pre-upgrade backups, exclusive live database backups, and fresh-directory recovery. Earlier compiled 0.5.0 acceptance preserved task/contract/checkpoint and read-grant state and rolled back from its pre-upgrade snapshot successfully. Its existing read credential still refused checkpoint writes after upgrade.
- MV3 extension with explicit profile pairing, ordinary HTTP(S) tab inventory/focus, popup pause/connection status, browser epochs, bounded IndexedDB outbox, reconnect and command journal.
- Compiled native helper with bounded framing and exact-origin config; browser-only daemon credential; replayable browser observations and command results.
- CLI open/navigate/focus/move/close. Existing tabs require recorded Heimdall ownership, current epoch and exact URL. Setup prepares native-host registration artifacts without installing them.
- Real Chromium extension/API checks and worker-to-compiled-daemon tests. The latter substitute a test native port for OS registry discovery; daily-profile deployment remains open; later C13/W05–W07 acceptance covers isolated native-host and Linux desktop behavior.

- Strict YAML task/workflow parsing: unknown and duplicate keys, document versions, IDs, status lifecycles, parent/prerequisite cycles, dates, importance/estimates, typed completion checks and anchors.
- SQLite/WAL event transactions with command dedupe, conflict rejection, one OS-locked writer, atomic projection updates, and pure replay. Unknown event/database versions fail safely. Accepted commands preserve exact results across replay.
- Task create/update/import, generated IDs, workflow materialization, manual step/task completion and reopening/drop, revision checks, and stable task serialization.
- Task-file polling handles atomic editor replacement. No-op saves do not emit task changes. Conflicting edits are preserved, with a pending view and recoverable originals; publication never replaces a concurrently recreated path.
- Capture grammar with Unicode/spaces and structured parse errors, one-event multi-target membership, client-scoped origins, reassignment, candidate/unassigned/study deadlines, and expiry history.
- Non-vacuous children/subtasks completion proposals, explicit accept/reject, evidence-set dedupe, stale/superseded proposals, and cancellation after reopening anchors.
- Silence-review scheduling from user-attested step completion. No mail coverage means unknown fulfillment and a review reminder, even after its deadline.
- Authenticated loopback daemon and CLI, strict request decoding, origin/Host rejection, limited request sizes, endpoint validation, graceful shutdown, and a stable-read task watcher.

## Next work

See the [Heimdall implementation roadmap](HEIMDALL-IMPLEMENTATION-ROADMAP.md) for the
P0 audit and adopted S1/S3/S5 integration design tasks. The sequence below is unchanged;
S1 includes observed-surface identity, browser events and the schema-22
catalog/container projection. Other sensors and purgeable description evidence
remain pending. See
[S1 implementation](S1-IMPLEMENTATION.md).

| Slice | Scope | Planned schema |
|---|---|---|
| P0 | W07 clock repair, r4 adoption, evaluator environment, YAML check materialization, browser deltas and daily-profile pairing | 20 |
| S2a (R5/A01/A02) | Delivered: scoped computer-use intents, action grant, reports and fresh reconciliation | 21 |
| S1 | Pinned Skald L0 capture; Heimdall hooks/sensors, surfaces, lifecycle and purgeable description evidence; S1b spike | 22 |
| S2b (R6/W08) | Startup readiness, interruption/reboot recovery, optional login restore | no reserved bump |
| S3 (C14–C16) | Braid assignment/continuity retrieval with scoped provenance, typed checkpoint MCP records, optional intent extraction | 23 |
| S4 | Planner, notifier, configuration/preferences and TUI views | 24 |
| S5 (C21) | Mail, version-probed Codex/Desktop adapters reusing pinned capture, packaging and fresh-install replay | 25 |

Delivery order: **P0 → S2a → S1 → S2b → S3 → S4 → S5**. S-numbers are
stable labels, not numeric execution order. S2a (A01/A02) is delivered; S1 now includes browser surface observations at schema 22; browser focus spans and `state --active` are implemented, while other sensors, compositor attention, lifecycle/UI and later slices remain pending. See [S2a implementation](S2A-IMPLEMENTATION.md).

**Agent execution is out of scope for Heimdall.** Agents run in Claude Code,
Codex and herdr; desktop input runs through WCU under its MCP host's approval.
Heimdall supplies accepted context, records scoped actions and reports, and
verifies outcomes from its sensors. No run state machine, dispatch outbox,
leases, fencing, resource locks, execution limits or execution-host adapter will
be added. Checkpoint handoff/resume is built; wake conditions and deduplicated
attention belong to the S4 notifier.

## Boundaries and scale triggers

S2a adds the explicit action grant; other grant kinds remain frozen. Observed-surface identity and browser focus spans are implemented in S1; compositor attention remains pending.
No sensor completes a task; evidence only proposes completion for ratification.
No raw evaluator output, prompts, transcripts or frames enter the event log.
Configuration remains compiled defaults until S4; Windows/macOS desktop adapters
return unsupported. Existing historical acceptance is not a fresh platform run.

| Item | Trigger |
|---|---|
| Normalize the projection blob into per-entity tables | blob > 1 MB, or p95 command latency > 50 ms, or `readState` > 10 ms |
| Event compaction with projection snapshots for replay | replay > 10 s, or events > 500k |
| Braid ANN index and Postgres backend | > 20k vectors, or a second device |
| Postgres for Heimdall | a second device or a second user |
| Gmail API OAuth adapter | more than one account or a Workspace that refuses app passwords |
| Fuzzy artifact lineage via Braid | exact digests miss > 20 % of observed handoffs |
| Learned fusion in Braid | ≥ 300 real labels and weighted RRF beaten on held-out data |
| WCU observer as a full compositor sensor | Heimdall's own Hyprland observer proves insufficient for verification |
| Portrait radiator page | the TUI on the landscape output proves insufficient after S4 review |
| Windows and macOS desktop backends | two or more working days a week on those machines |


## Historical identifiers

R0 technical baseline; R1/T01–T03/W01 terminal/editor continuity; R2/P01–P04
artifacts, progress, manual preservation and dependencies; R3/W02–W04 observation,
snapshots and previews; R4/C12/C13/W05–W07 actions, browser verification and
application recovery are retained identifiers for delivered work. R5/A01/A02 map
to S2a; R6/W08 maps to S2b; C14–C16 map to S3; C21 maps to S5.
C17–C20 are retired, not pending work. W09 Hyprflow import and automated P03
preservation remain deferred; multi-machine replication and raw-output retention
remain deferred. Conversation ingestion, ranked planning and mail are scheduled
at S1, S4 and S5 respectively. Proposal authoring goes to S4; proposed MCP writes
use typed checkpoint records at S3, not a proposal-write grant. Workflow timing
and daily-use acceptance belong to every slice.

The original C01–C21 definitions remain in
[the archived backlog](history/roadmaps/BACKLOG-baseline-2f11175.md),
[the original plan](history/roadmaps/IMPLEMENTATION-PLAN.md), and
[the Sept 8 roadmap](history/roadmaps/REVISED-ROADMAP-IMPLEMENTATION-PLAN.md).
These documents are historical, not execution instructions.
