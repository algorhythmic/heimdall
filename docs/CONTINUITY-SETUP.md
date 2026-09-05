# Contracts, checkpoints and resume context — 0.6.0

The first continuity slice is usable through the CLI. It stores an accepted objective, constraints, decisions, resource bindings and immutable checkpoints separately from `tasks.yaml`. Checkpoint traffic does not change the task document revision. The MCP adapter and explicitly granted client checkpoints are described in MCP-SETUP.md. Automatic execution, evidence evaluators, Braid and the task GUI remain open.

## Try it

Start the daemon in a separate test data directory using the README instructions. In another PowerShell 7 terminal, create a task and accept its contract:

```powershell
$data = '.\demo-data'
.\bin\heimdall.exe add 'Implement a feature' --id feature-work --type project --status active --data-dir $data
$task = .\bin\heimdall.exe state feature-work --data-dir $data | ConvertFrom-Json
$revision = $task.revision
@{previous='none'; objective='Implement and verify the feature'; constraints=@('Preserve existing user edits')} |
    ConvertTo-Json | Set-Content -Encoding utf8 contract.json
$contract = .\bin\heimdall.exe contract accept feature-work --expected-task-revision $revision --file contract.json --data-dir $data | ConvertFrom-Json
```

`none` explicitly means there is no previous contract. To revise a contract, put the current contract ID in `previous`. The server copies the task or step's current `done` criteria into the accepted record; the caller cannot replace them through this command. Task changes make the accepted contract stale until explicitly reaccepted. New version-2 contracts also freeze the complete sorted resource_ids list from the task lineage. Omitting it accepts only an empty scope. New or removed bindings require explicit scope reacceptance; for an existing task, include all reviewed inherited and direct binding IDs.

Optionally bind a small working directory. Use the actual path to your files; a tree containing dependencies or build artifacts may exceed the limits.

```powershell
@{kind='tree'; root='C:\path\to\working-files'; path='.'; exclude=@('node_modules','.tools','bin')} |
    ConvertTo-Json | Set-Content -Encoding utf8 resource.json
$binding = .\bin\heimdall.exe resource bind feature-work --expected-task-revision $revision --file resource.json --data-dir $data | ConvertFrom-Json
# Explicitly accept the new binding scope before recording a checkpoint.
@{previous=$contract.id; objective=$contract.objective; constraints=$contract.constraints; resource_ids=@($binding.id)} |
    ConvertTo-Json | Set-Content -Encoding utf8 contract.json
$contract = .\bin\heimdall.exe contract accept feature-work --expected-task-revision $revision --file contract.json --data-dir $data | ConvertFrom-Json
```

Record a checkpoint and assemble context:

```powershell
@{previous='none'; contract_id=$contract.id; summary='Initial implementation saved'; next_action='Run focused checks'; blockers=@()} |
    ConvertTo-Json | Set-Content -Encoding utf8 checkpoint.json
.\bin\heimdall.exe checkpoint create feature-work --expected-task-revision $revision --file checkpoint.json --data-dir $data
.\bin\heimdall.exe checkpoint show feature-work --data-dir $data
.\bin\heimdall.exe checkpoint list feature-work --data-dir $data
.\bin\heimdall.exe context feature-work --budget 16000 --data-dir $data
```

The next checkpoint requires the latest checkpoint ID in `previous` and the current accepted contract ID. `context` includes the task, ancestors, accepted contracts and decisions, latest checkpoint, and live resource observations. Changed files, changed task/ancestor contracts, added or removed bindings, changed decisions, blockers and unavailable resources produce `needs_review`. Review those changes and write a new checkpoint to acknowledge the new working state. `ready` means the saved context matches the observations made by this command; it is not permission to execute or proof that work is complete.

## Other operations

| Command | Input / result |
|---|---|
| `contract show TARGET` / `contract list TARGET` | Current contract / all accepted revisions |
| `decision accept TARGET --file FILE --expected-task-revision N` | `{"text":"Accepted decision","supersedes":"optional prior ID"}`; omit `supersedes` for a new decision |
| `decision list TARGET` | Accepted history, including superseded records; `context` includes only current decisions |
| `resource list TARGET` | Binding history and active flags |
| `resource unbind TARGET --id ID --expected-task-revision N` | Deactivate a binding; existing checkpoints keep the old reference |
| `backup --output FILE` | New consistent database snapshot; refuses an existing destination |

Every command also accepts `--data-dir PATH`. Targets may be `task-id#step-id`; quote these in the shell. Bindings and decisions on ancestors are inherited when assembling context. `current_step` on checkpoint input is an optional local step ID. Checkpoint `show` returns the current head; `checkpoint show TARGET --id ID` retrieves a specific historical record belonging to that target. History is available through `list`, ordered by source event. These unrestricted CLI history responses remain unpaginated. Machine clients use the bounded, paginated history route in SCOPED-ACCESS.md.

Mutation requests require the observed task revision, even for unbinding. A stale revision or competing contract/checkpoint head returns conflict. Use a fixed global `--request-id` containing 32 lowercase hexadecimal characters for a logical retry. Repeat the exact request, including revision and head preconditions: the saved result is returned without repeating filesystem observations. A changed request needs a new ID. Continuity timestamps come from the daemon clock; CLI `--now` alone does not change them.

## Resource and context limits

- File/tree roots must resolve to absolute canonical paths. Relative CLI roots resolve from the CLI working directory. Resource paths must stay within that root. Reads use Go `os.Root`; symlink resources and nonregular files are rejected. Canonicalization needs permission to resolve parent directories.
- A task lineage may observe at most 16 active resources. Each resource is limited to 4,096 files, 8,192 visited entries and 64 MiB per pass. Trees automatically exclude `.git`; additional exclusions are exact basenames at any depth. Two passes must produce the same digest. Filesystem observations share a five-second operation context; blocking OS I/O is not a hard real-time deadline.
- Digests cover file contents, relative names, file permissions and directory names. File contents are not returned or stored. This is a bounded observation, not a filesystem lock or transactional filesystem snapshot. Git commit/ref identity, browser outcomes and completion evidence are not captured. These coverage gaps appear in every context response.
- Request and persisted record limits are 64 KiB. Mandatory context is never truncated to make room for retrieval. Its budget estimate is `ceil(serialized UTF-8 bytes / 4)`, not an exact model token count. A small budget returns HTTP 422 `budget_too_small` with the required estimate. Retrieval is not involved.

## Backup, upgrade and restore

Database markers 1–5 upgrade to 6. While holding the sole-writer lock, startup first publishes `backups/pre-schema-6-<id>.db`; a failed snapshot aborts the upgrade. Event envelopes remain version 1; new contracts are version 2, and CLI checkpoints remain v1; client checkpoints and write grants are v2 with authenticated grant provenance. Marker 6 adds evaluator/evidence records. Old contracts replay unchanged but need explicit scope review before new checkpoints. Replay never reads resources or executes actions. Executables from 0.5.0 and earlier refuse a marker-6 database.

`backup --output FILE` uses SQLite `VACUUM INTO` for a consistent live database snapshot, with exclusive hard-link publication to avoid overwriting a concurrent destination. Its parent directory must exist and support hard links. This uses SQLite's documented snapshot behavior. [SQLite VACUUM INTO](https://www.sqlite.org/lang_vacuum.html#vacuum_with_an_into_clause)

This is a **database-only backup**. Preserve the matching `types.yaml` separately, and preserve any unaccepted task-file edits before recovery. The database includes user-entered content and absolute resource paths; it is not redacted. Endpoint credentials, browser-host configuration, external working files and browser storage are not included.

To recover, stop the relevant daemon and create a fresh recovery directory. Copy the snapshot there as `heimdall.db` and copy its matching `types.yaml`. Do not copy old WAL/SHM files, endpoints or a newer `tasks.yaml`. Start the compatible binary against that directory: it regenerates the task edit view and endpoint credentials. Verify `state`, `replay`, `doctor` and `context` before adopting it. Retain the original directory for inspection. For rollback, use the pre-schema-6 snapshot with the binary compatible with its original marker; post-upgrade events cannot be retained by that old binary. Snapshots retain grant verifier/revocation state; review and revoke restored grants before reconnecting clients, since an older snapshot can precede a later revocation.

## Interfaces and remaining work

The wire shape is in [continuity-request-v1.schema.json](../schemas/continuity-request-v1.schema.json), with [request fixtures](../testdata/continuity/requests.json). Runtime decoding additionally rejects duplicate/unknown keys, oversized UTF-8 payloads and invalid state-dependent operations. Accepted records are defined in `internal/model/continuity.go`; their event names are `contract.accepted`, `decision.accepted`, `resource.bound`, `resource.unbound` and `checkpoint.recorded`. These records and their historical heads survive restart and replay.

Authenticated CLI routes are `GET /continuity/state`, `GET /continuity/context`, `POST /continuity/command`, and `POST /continuity/backup`. Browser and client credentials cannot access them. The new [scoped read API](SCOPED-ACCESS.md) provides separate task/subtree credentials and bounded history. Explicit checkpoint-write grants and MCP are now available; see MCP-SETUP.md.

Reproduce the end-to-end check with `node scripts/continuity-smoke.cjs`. Supplying an archived 0.2.0 executable as its first argument additionally exercises a real legacy upgrade and rollback. Supply an archived 0.3.0 binary and `--legacy-continuity` to also preserve and upgrade real old contracts/checkpoints. The script uses synthetic data, closes its daemon processes, and retains its scratch directory for inspection.
