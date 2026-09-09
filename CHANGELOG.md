# Development milestones

This records implementation milestones and current development work. Versions describe local development builds; they do not imply published releases or completed deployment acceptance. The initial Git import captured the 0.5.0 implementation together, rather than reconstructing historical source commits.

## Unreleased — Terminal/editor continuity, Herdr bindings and Linux artifact versions (2026-09-08)

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
- Schema 9 and CLI artifact checkpoint request-v2/persisted-v3 pins. Legacy records and grant limits survive schema-6/7/8 fixture upgrades; the actual schema-8 binary passes upgrade/refusal/rollback. CLI/Neovim drafts retain exact pins; scoped context shows references and requires CLI checks without expanding filesystem/Git authority. Compiled restart/replay/backup restore, Git/confinement/race and editor tests pass. Progress/decision review and content preservation remain open.

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

Initial W01 workspace/session identity, T02 Herdr binding and T03 Neovim integration now build on the R0/T01 foundation. Initial P01 adds stable local Linux artifact versions and optional Git identity. Remaining C03 decision review, evidence output retention, broader GUI controls, verified browser outcomes, Braid and execution-host coordination stay open. See the [backlog](docs/BACKLOG.md) for dependencies and acceptance criteria, and [verification](docs/VERIFICATION.md) for what was actually tested.
