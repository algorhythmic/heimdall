# P04: task dependencies and scoped progress summaries

Task dependencies record an explicit planning prerequisite between two tasks,
including tasks in different projects. They are separate from accepted completion
checks. Adding, removing or satisfying a relation never completes work, changes a
checkpoint, runs an adapter or starts an agent. Existing step `after` prerequisites
keep their completion semantics.

Use the same `--data-dir` as the running daemon. The local development executable
is `./bin/heimdall`.

## Record and inspect a dependency

Read both task revisions with `state TASK`. Save a version-1 request envelope:

```json
{
  "version": 1,
  "id": "11111111111111111111111111111111",
  "target": "implementation",
  "depends_on": "design-review",
  "expected_task_revision": 3,
  "expected_prerequisite_revision": 2,
  "previous": "none",
  "op": "add",
  "reason": "Implementation needs the reviewed design."
}
```

```bash
./bin/heimdall dependency add implementation --file dependency.json --data-dir /path/to/data
./bin/heimdall dependency list implementation --data-dir /path/to/data
./bin/heimdall dependency show implementation --id RECORD_ID --data-dir /path/to/data
```

IDs are 32 lowercase hexadecimal characters. Each mutation requires the observed
revision of both endpoints and the exact previous relation head (`none` initially).
A fresh mutation uses a fresh request ID. An exact retry returns the original
receipt, including after restart, removal or later task changes. Changed content
under the same ID is a conflict.

To remove a relation, save a new envelope with `op: "remove"`, the latest relation
ID as `previous`, current task revisions and an explicit reason. Submit with
`dependency remove TARGET --file FILE`. `list` shows active relations; `show`
inspects an exact historical record. Re-adding a removed relation links to its
removal record. At most 128 active prerequisites are permitted per task.

Relations target tasks, not `task#step`. Use the existing task `after` fields for
step prerequisites. The graph includes parent-to-child hierarchy edges so a child
cannot require its own ancestor. Transitive cycles across projects are refused.
Task edits/imports are checked after their complete atomic command: reparenting
cannot introduce a cycle, while a valid multi-task move may pass through a
transient intermediate graph before commit. Replay checks the same command
boundaries. Failed graph changes leave tasks and relation history untouched.

A prerequisite is `satisfied` when its task has a workflow success status,
`pending` otherwise, or `dropped` when explicitly dropped. Dropping a prerequisite
does not satisfy it. Completing or reopening that task updates the derived view;
it does not change the dependent task's status. These planning relations do not
silently add a new evaluator to an already accepted completion contract.

## Progress summary

```bash
./bin/heimdall progress summary implementation --data-dir /path/to/data
./bin/heimdall progress summary project-root --subtree --sort checkpoint --data-dir /path/to/data
./bin/heimdall progress summary --all --sort due --limit 25 --data-dir /path/to/data
```

`--all` is an explicit CLI-only scope; a task actually named `all` remains an
ordinary task. Default scope is the named task. `--subtree` adds descendants.
The response describes recorded state only; it performs no filesystem or remote
observations. It includes:

- Latest saved checkpoint ID, timestamp and summary.
- Recorded next action and its direction status, plus recorded checkpoint blockers.
- Task prerequisites and current derived status.
- Existing step `after` relationships and their statuses.
- Unresolved decision count and the first eight decision proposals, oldest first.

The ordering is explicit: `checkpoint` (default) sorts latest saved checkpoint
first, `due` sorts earliest `resume_by` first, and `id` sorts task ID. Missing
checkpoints/dates sort last; task ID breaks ties. This is not ranked planning.
The latest saved checkpoint is used without a heuristic score of its contents.

Page sizes are 1–50, with a 512 KiB response bound. Pass the returned `next_cursor`
as `--cursor CURSOR` with the same target, subtree and sort. Any intervening event
invalidates the cursor; restart pagination. The cursor is a position, not authority.
A single oversized row is refused rather than silently dropping mandatory fields.

The TUI selected-context pane shows active task dependencies and their recorded
status; the normal refresh updates that display. Dependency authoring currently
uses the CLI. Existing task/step completion review remains unchanged.

## Scoped clients

```bash
./bin/heimdall client summary project-root --subtree --sort id --credential reader.credential.json
./bin/heimdall client dependencies implementation --credential reader.credential.json
```

These read-only commands use the existing task/subtree read grant. A foreign
prerequisite outside the current grant becomes one `restricted` placeholder:
no foreign ID, title, path, reason, history, status or count is included. When both
endpoints are visible, the relation and its current status can be inspected.
If mandatory ancestor direction is outside scope, the summary labels direction
`scope_limited` and uses the task's own next-action field. User-authored text on
visible records remains visible, as with existing checkpoint reads.

Scope and expiry are checked under the same store inspection used to build the
response. Moving a prerequisite out of a granted subtree removes its details;
revocation denies the entire response. Cursors bind the grant identity and scope.
Clients cannot request all-project scope or reach CLI dependency mutation/history
routes. Browser credentials gain no dependency or summary access. No resource
observation or execution permission is added to a read grant.

Schema 13 preserves previous event families and exact receipts; startup publishes
`backups/pre-schema-13-*.db` before upgrading older data. Rollback uses the stopped
pre-upgrade snapshot and its matching old binary. See [request schema](../schemas/dependency-request-v1.schema.json),
[verification](VERIFICATION.md) and [roadmap](REVISED-ROADMAP-IMPLEMENTATION-PLAN.md).
