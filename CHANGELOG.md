# Development milestones

This records the implementation milestones reached by 2026-09-05. Versions describe local development builds; they do not imply published releases or completed deployment acceptance. The initial Git import captured the 0.5.0 implementation together, rather than reconstructing historical source commits.

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

C08/C09 have an initial CLI implementation; output retention, broader evidence tools and review notices remain open. Remaining C03 decision/Git identity work, task GUI, verified browser outcomes, Braid retrieval and execution-host coordination also remain open. See the [backlog](docs/BACKLOG.md) for dependencies and acceptance criteria, and [verification](docs/VERIFICATION.md) for what was actually tested.
