# Development milestones

## Unreleased — W06 reviewed application adapters (2026-09-09)

- Add immutable application recipes, CLI review/show, and recipe-aware workspace previews under schema 20.
- Launch reviewed Foot commands, rebind exact observed process/window identities, attach to surviving Herdr terminals and verify session survival after detach. Lost receipts and uncertain attempts never trigger a new launch.
- Connect paired-browser open/focus/owned-tab close to workspace operations. Extension 0.6.0 advertises recovery capability and checks browser self-restored URLs before creation/navigation.
- Open structured saved-file Neovim state; preserve potentially unsaved editors by leaving them open. Full attachment/editor-state/layout verification remains W07.
- Allow correctly redacted browser snapshots while preserving native PID validation and scoped ownership checks.

## C13 browser verification slice — 2026-09-09

- Add challenged double readback, monotonic freshness leases and independent exact browser postconditions under schema 17 and extension 0.4.0.
- Reconcile retained results and the same attempt without repeating input; bound observations, preserve epoch-loss/close uncertainty and refuse duplicate owned surfaces.
- Exercise actual Linux Chromium native-host discovery, daemon loss after a browser side effect, and before-unload closure boundaries in an isolated profile.
- Browser-to-Hyprland nonce association remains the next integration slice.


## Unreleased — C12 shared action journal (2026-09-09)

- Task-bound intent, context/manifest/authority pins, immutable attempt identity and dispatch observations under schema 16. Execution and verification stay separate; legacy browser success remains unscoped and unverified.
- Atomic intent/outbox and guarded delivery receipts, no new-ID retry of uncertain input, explicit cancellation, deadline/reconnect/restart uncertainty and retained late API reports. Unfinished actions protect referenced snapshots.
- Extension 0.3.0 carries exact action/attempt references and persists a matching request journal before browser effects. Go/browser conformance and old-ID/legacy ownership guards preserve the existing protocol.
- Scoped action/history CLI and replay/backup/schema-15 rollback tests. Independent browser postconditions and native-host registration remain C13.

## Unreleased — W04 workspace preview (2026-09-09)

- Scoped live List/Diff and explicit current-manifest/retained-snapshot previews with reattach, launch, move, leave-open, unavailable and review-required dispositions.
- Fresh owned-window and Herdr observation, explicit source/display/ownership limits, protected-point/age diagnostics and deterministic scoped comparisons.
- Private review files with a 30-second daemon-lifetime seal; validation refuses changed, tampered, expired or restarted previews and reobserves inputs without effects.
- TUI workspace comparison, saved-point/autosave details and explicit retained-request point capture. No application dispatch or database schema change.

## Unreleased — W03 durable workspace snapshots (2026-09-09)

- Immutable scoped snapshot payloads, indexed history, atomic event/receipt/head publication and complete/partial capture boundaries under schema 15.
- Manual points are pinned; explicit task/source/manifest policies enable debounced autosave with a configurable maximum dirty interval. Unchanged content adds no events; unavailable or partial observations retain the last complete head.
- Bounded payload retention, protected heads/pins, explicit pruned history, pagination, replay and backup restoration. Large payloads never enter the task projection.
- Compositor reads move outside the task writer while exact retries remain inert and concurrent revision changes are refused at publication.
- Local Btrfs process-kill, failed-write/allocation and daily-volume gates pass. Controlled VM power loss and workspace recovery actions remain later acceptance work.

## Unreleased — W02 read-only Hyprland observation (2026-09-09)

- Explicit source probe/select/stop, buffered bootstrap, periodic reconciliation, known event-gap and freshness diagnostics for local Hyprland 0.56.2.
- Immutable task/surface viewport bindings use a compositor process/socket epoch plus native stable window ID; duplicate titles, moved windows and reused addresses never infer ownership.
- Task views expose only explicitly bound windows. Session joins remain declarations with unverified pane attachment; browser profile pairing awaits C13.
- Schema 14 preserves old records, grants and exact retries with stopped schema-13 upgrade/refusal/rollback. Inventories remain bounded memory; no desktop action or durable restoration point is added.

## P04 task dependencies and scoped summaries — 2026-09-09

- Add immutable task dependency add/remove records with both endpoint revisions, exact prior heads and cycle checks across dependencies and hierarchy. Task edits/imports validate the final atomic graph; replay preserves the same boundary.
- Add explicit, bounded progress summaries ordered by saved checkpoint, due date or task ID. Scoped readers see only permitted tasks and opaque foreign-prerequisite placeholders; revocation and moved scope apply to every page.
- Display dependency status in the TUI. Completion/reopening updates the derived view without completing dependent work or dispatching agents. Schema 13 preserves earlier state, events and exact receipts.

## P03 manual preservation — 2026-09-09

- Add checkpoint-linked preservation preview, retained manual handoff requests and independently observed source/mirror/private-commit/remote facts. Failure reports remain separate from observed results; checkpoint/task state is unchanged.
- Add bounded CLI status, reconciliation receipts and portable progress export. Remote reads pin the explicit destination; no dotprivate execution or Git writes occur. Programmatic preservation remains gated by shared C12 actions and an upstream selected-files boundary.
- Schema 12 retains all previous records and receipts with stopped schema-11 migration, malformed-event, inert replay, exact retry, restart and backup restore coverage.

This records implementation milestones and current development work. Versions describe local development builds; they do not imply published releases or completed deployment acceptance. The initial Git import captured the 0.5.0 implementation together, rather than reconstructing historical source commits.

## Unreleased — Terminal interface replacement (2026-09-08)

- Replaced the browser frontend and sign-in/session routes with a Go TUI based on the supplied dashboard and dialog designs. `tui` opens it; `ui` remains an alias.
- Needs-you queue, sorted/expandable workstreams, selected context, find, compact layout, explicit completion/planning review, files and Herdr workspace/binding dialogs.
- Progress drafts persist edits and original request identity; mutations retain private exact-retry files across terminal or daemon interruption. No automatic retry or file repinning.
- Neovim review now opens an argv-only terminal tab. The TypeScript frontend and its browser tests are removed; extension test tooling is retained separately.
- Schema remains 11 and historical UI-v2 event provenance still replays. Native desktop bars, agent telemetry and application recovery remain roadmap work.

## Unreleased — Terminal/editor continuity, Herdr bindings, artifacts and progress review (2026-09-08)

- Readable `resume TARGET` with JSON available explicitly, accepted direction, checkpoint age/next action, resource drift, blockers and recorded review needs. Existing context JSON remains compatible; terminal controls are escaped.
- `checkpoint draft` and `checkpoint submit` preserve original revisions, heads and retry identity through explicit editing, conflicts and daemon restarts. Draft files are retained and never overwritten during creation.
- Portable compiled smoke paths and bounded child cleanup, a core/restart smoke, a resume workflow smoke and expanded Windows/Ubuntu CI configuration. Linux Go/vet/build and compiled core/native/continuity/MCP/evidence/GUI/browser/worker acceptance pass; WCU inspected the terminal output on Hyprland.
- A stopped schema-6 fixture from the unchanged 0.7.0 binary checks state, grants, command receipts and replay compatibility. The original fixture is retained for schema-7 migration checks. Extension version is unchanged. Workspace restoration remains future work; T02/T03 updates below add live session verification and editor commands.
- Initial W01 workspace manifests, stable task-owned surface IDs and explicit generic session bind/show/unbind commands. Immutable history, exact retries, competing-head/pane ownership checks, stale task/manifest diagnostics and pure replay; all active declarations remain unverified.
- Schema 7 with pre-upgrade snapshots, actual 0.7.0 refusal of newer state and successful rollback, unchanged scoped grant authority, request/event fixtures and a portable workspace restart/backup smoke. This W01 slice performs no desktop actions; T02 metadata integration follows below.

- Initial T02 Linux Herdr 0.8.2/protocol-20 adapter: actual pane/server/process identity, canonical cwd/Git, explicit refresh/rebind, expiring metadata and optional pane-labelled sidebar summary. Installed move/restart/expiry checks and WCU visual inspection pass. Runtime agent status never completes tasks.
- Schema 8 preserves generic v1 declarations and adds observed v2 bindings. Actual schema-6/7 fixtures, consistent pre-upgrade snapshots, pure replay and old-binary refusal/rollback are verified. Automatic refresh, remote sessions and workspace recovery remain open; T03 editor support follows below.

- Initial T03 Neovim plugin: explicit task/step selection, resume, checkpoint drafts/reopen/submission, bound artifact opening, session checks and GUI completion-review handoff. Local argv/JSON integration retains task/request identity through conflict and restart; renamed or externally changed drafts are refused. Isolated Neovim and lazy.nvim loader tests pass; WCU verified rendering. An example spec is provided without modifying global editor configuration. Schema and existing authority remain unchanged; automatic refresh and workspace recovery remain open.

- Initial P01 local Linux artifact IDs and immutable file versions, owned by explicit task/environment/host, with byte/permission identity and opt-in Git worktree/ref/index/HEAD metadata. CLI record/list/show/check distinguishes changed bytes, missing originals and declared relocation; file contents are not retained.
- Schema 9 and CLI artifact checkpoint request-v2/persisted-v3 pins. Legacy records and grant limits survive schema-6/7/8 fixture upgrades; the actual schema-8 binary passes upgrade/refusal/rollback. CLI/Neovim drafts retain exact pins; scoped context shows references and requires CLI checks without expanding filesystem/Git authority. Compiled restart/replay/backup restore, Git/confinement/race and editor tests pass. Content preservation remains open; initial P02 review follows below.

- Initial P02 CLI decision/artifact proposals and reviewed/accepted/rejected outcomes. Exact digest, contract, lineage, accepted-decision and live artifact checks refuse stale review. Artifact lifecycle stays separate from task/step completion; accepted decisions remain mandatory context and unresolved proposals appear separately in resume.
- Schema 10 preserves legacy payloads and grants, with schema-6/7/8/9 fixtures, strict event replay, exact retry receipts and compiled restart/restore/old-binary rollback checks. GUI/editor review controls and scoped agent proposal grants remain open.

- P02 GUI planning review: explicit `ui ROOT --progress-review` sessions inspect proposal/contract/artifact identity and record reviewed/accepted/rejected outcomes with notes. Original preconditions survive polling and uncertain-response retries; task completion remains separate. Ordinary sessions and MCP grants gain no new authority.
- Schema 11 adds UI review v2 with frozen non-secret session scope/lifetime and UI-attributed accepted decisions. Authority is checked before receipt lookup and before commit; replay never recreates sessions or observes files. Actual stopped schema-10 and v2 event fixtures retain the prior CLI slice.
- Neovim `HeimdallProgress` adds read-only inspection and late-response protection; `HeimdallProgressReview` provides the explicitly enabled GUI handoff. Bootstrap codes stay out of editor buffers/history. GUI proposal authoring and native editor review forms remain open.

## 0.7.0 — Local task and evidence GUI

- Daemon-embedded TypeScript interface for scoped task/step navigation, accepted direction, checkpoints, blockers, resource drift and evidence provenance.
- CLI-issued single-use sign-in codes, expiring HttpOnly sessions, frozen resource permissions, same-origin/CSRF guards and scoped polling cursors. Credentials stay out of URLs.
- Explicit completion accept/reject through the existing transaction and live evidence revalidation path, including authorization before retries and commit.
- Continuity/evidence changes refresh the feed even without a task document revision change. Bounded views disclose truncation.
- Go authorization/feed regression tests and compiled Chromium desktop/mobile, keyboard, isolation, injection, review and logout checks. TypeScript generation and GUI smoke are included in CI.
- Database schema remains 6; extension remains 0.2.0. Braid, verified browser outcomes and automatic continuation remain unimplemented. Development stops at this requested publication checkpoint.

## 0.6.0 — Completion evidence and revalidation

- CLI-configured artifact existence/digest, repository-state and test-exit evaluators tied to accepted contracts and registered resource scopes.
- Durable asynchronous attempts, independent observer provenance, bounded test output digests and unknown interrupted outcomes. Exact retries never launch another command.
- Task/step completion proposals, explicit evidence invalidation and live input/repository/executable/environment revalidation before ratification.
- Schema marker 6 with verified actual 0.5.0 upgrade and backup/restore. Existing read-grant permissions remain unchanged.
- New focused failure tests and compiled evidence smoke coverage. GUI implementation is excluded from this checkpoint.

## 0.5.0 — MCP and scoped checkpoint writes

- Official Go MCP SDK stdio adapter with task, context, history and checkpoint tools.
- Explicit checkpoint-write grants, authenticated author provenance and authority checks before retry lookup and commit. Read grants retain their original permissions.
- Shared scoped HTTP client with bounded responses and daemon endpoint rediscovery.
- Database marker 5 with pre-upgrade snapshots; actual 0.4.0 upgrade and restore checks preserve continuity and read-grant behavior.
- Official SDK and compiled stdio integration checks cover retries, conflicts, revocation, restart, replay and unchanged task completion.

## 0.4.0 — Scoped assistant reads

- Expiring, revocable exact-task/subtree credentials with explicit resource permissions.
- Scoped task/context/history APIs, bounded pagination and scope-bound cursors.
- Version-2 contracts freeze reviewed resource IDs; scope changes require reacceptance.
- Database marker 4 and upgrade/recovery checks.

## 0.3.0 — Durable continuity

- Accepted task/step contracts and decisions, supersession and canonical file/tree bindings.
- Immutable checkpoints with explicit task revision and previous-head preconditions.
- Mandatory context assembly with resource drift, ancestor changes, blockers and explicit budget errors.
- Consistent database backups and schema migration snapshots.

## 0.2.0 and earlier — Task core and browser bridge

- Strict YAML tasks/workflows, hierarchical task operations, captures and timers.
- SQLite event log, sole-writer locking, idempotent commands, replay and conflict-preserving task-file synchronization.
- Manual completion, non-vacuous aggregate proposals and explicit ratification; missing evidence coverage does not imply success.
- Authenticated loopback daemon, CLI and framed native messaging helper.
- MV3 extension with explicit pairing, metadata inventory, pause/reconnect, durable outbox and guarded owned-tab controls. The extension version remains 0.2.0.

## Next

Initial W01 workspace/session identity, T02 Herdr binding and T03 Neovim integration now build on the R0/T01 foundation. Initial P01 adds stable local Linux artifact versions and optional Git identity. Remaining review interface controls, evidence output retention, broader GUI controls, verified browser outcomes, Braid and execution-host coordination stay open. See the [backlog](docs/BACKLOG.md) for dependencies and acceptance criteria, and [verification](docs/VERIFICATION.md) for what was actually tested.
