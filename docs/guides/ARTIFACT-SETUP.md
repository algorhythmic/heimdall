# Artifact identity and checkpoint pins

Initial P01 records local Linux file identity without requiring a Git commit.
Each artifact has a stable ID owned by one explicit task or step, environment
label and observed host. Immutable versions record a location inside an active
resource binding, SHA-256 of the exact bytes, length and permissions. File bytes
are **not retained**; restoring the database cannot restore a missing file.

This is a trusted local CLI operation. Windows/macOS builds can replay records,
but fresh artifact observation currently requires Linux. Git is optional.

## Record a version

Build the current binary and start a daemon in your chosen data directory.
Prepare a task and resource using [continuity setup](CONTINUITY-SETUP.md).
Use `state TARGET` for the task revision and `resource list TARGET` for binding
IDs; inherited bindings can also be selected. No task is inferred from cwd.

Create `artifact.json` outside the observed resource tree, replacing the binding
ID below with the reviewed active binding:

```json
{
  "artifact_id": "new",
  "previous": "none",
  "name": "Planning notes",
  "environment": "local",
  "resource_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "path": "notes.md",
  "git": false
}
```

```sh
./bin/heimdall artifact record feature-work --file artifact.json \
  --expected-task-revision 1 --request-id bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  --data-dir ./demo-data
./bin/heimdall artifact list feature-work --data-dir ./demo-data
./bin/heimdall artifact show feature-work --id bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  --data-dir ./demo-data
./bin/heimdall artifact check feature-work --id bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  --data-dir ./demo-data
```

The first artifact ID and version ID both equal the request ID. A later version
uses a fresh request ID, the stable `artifact_id`, and the exact latest version
ID in `previous`. Keep the original environment and omit `name`; name and
ownership are immutable. The daemon derives the host from a SHA-256 of the
local machine ID, not from caller-supplied provenance. Environment is an explicit
label such as `local` or `staging`, not an inferred or independently verified
deployment identity. Separate tasks/environments may register the same file
under different artifact IDs.

For a tree binding, `path` selects a clean relative file path inside its bound
subdirectory. For a file binding, use `"path": "."`. Exclusions apply at every
level; `.git`, traversal, symlinks, directories and nonregular files are refused.

Moving an original makes its current check `missing`. To record a relocation,
explicitly submit a new version with the same artifact ID and a new path or
in-scope binding. This declares continuity of identity; Heimdall does not scan
for similarly named files or infer a move from equal bytes.

`show` reads recorded metadata; `check` performs a fresh observation. Both accept
`--version VERSION_ID` to select a historical record. `list` returns identities
and their heads; follow each version's `previous` to inspect history.

| Check status | Meaning |
|---|---|
| `matched` | Current head's bytes, permissions and requested Git identity match. |
| `content_changed` | File bytes or length changed. |
| `identity_changed` | Bytes match but permissions or Git metadata changed. |
| `missing` | The current declared location is absent. |
| `relocated` | An explicit newer version uses another path/binding. The selected historical location is not probed. |
| `version_changed` | A newer recorded version exists at the same selector. |
| `unbound` / `scope_changed` | Binding is inactive or no longer belongs to the target lineage. |
| `host_changed` | The database is being checked on another observed host. |
| `unavailable` | Observation is unsupported, fails, exceeds a limit or changes during the check. |
| `state_changed` | A recorded event arrived during a standalone check; check again. |

## Pin a checkpoint

Accept the complete resource scope in the current task contract before creating
a checkpoint. Add `artifacts` to the usual checkpoint input:

```json
{
  "previous": "none",
  "contract_id": "cccccccccccccccccccccccccccccccc",
  "summary": "Reviewed the planning notes",
  "next_action": "Review the remaining design decision",
  "blockers": [],
  "artifacts": [
    {
      "artifact_id": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "version_id": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    }
  ]
}
```

Replace the example IDs and use the current checkpoint ID in `previous` when
one exists. Submit with `checkpoint create TARGET --file FILE
--expected-task-revision N --data-dir DIR`. References must be unique and sorted
by artifact ID. Each must belong to the exact target, select its current version
and use an accepted resource. A fresh mismatch refuses the new checkpoint.

Artifact checkpoints use request **v2** and persisted checkpoint **v3**. Legacy
CLI v1 and scoped-client v2 checkpoint records remain readable and replayable.
The new request schema is [continuity-request-v2](../../schemas/continuity-request-v2.schema.json).

`resume` and `context` check the pinned versions and report drift. They never
replace references with newer heads. A draft copied from an artifact checkpoint
retains request v2 and its exact references. Review a new version explicitly
before submitting a new pin; a stale draft stays on disk after refusal.
[Neovim](NEOVIM-SETUP.md) displays pinned checks and protects their references as
part of draft identity. Use the CLI to prepare an explicitly revised set of pins.

Repeat the same request ID and body after an uncertain result. Committed retries
return the exact original record even after relocation, file deletion, restart
or backup restore. A changed logical request requires a new ID. Recording a
version or checkpoint does not complete a task or accept a design decision.

## Optional Git identity and limits

Set `git: true` only when repository identity is wanted. Git must be available
and the file must be inside a real repository. Metadata records canonical
worktree/common-directory paths, commit, symbolic ref, index/HEAD blob IDs and
modes, plus a digest of the selected input. Unborn branches and linked worktrees
are supported; detached HEAD has an empty symbolic ref. Unmerged input is refused.

`dirty` compares the **selected raw bytes** and executable mode against the
index and HEAD. It does not summarize other files in the repository. No Git
clean/smudge, text conversion, filters, fsmonitor or hooks are run; Git plumbing
reads metadata and blob hashes are computed directly from the observed bytes.
Repositories with content transformations may therefore report raw input as
dirty even when a normal Git status considers it clean. Global/system Git
configuration and inherited Git overrides are excluded from these probes.

Limits: 64 KiB requests/records, 64 MiB per file, 128 identities per target,
1–16 pins per checkpoint, 16 KiB output per Git command and a one-second Git
command context inside the five-second operation context. File observations run
twice and must agree. Blocking OS I/O is not a hard real-time deadline; there is
no filesystem lock or transactional filesystem snapshot. Resource observations
retain their separate [existing limits](CONTINUITY-SETUP.md#resource-and-context-limits).

All four `/artifact/record|list|show|check` routes require the local CLI bearer.
Existing scoped clients, MCP and GUI can read opaque checkpoint references under
their existing context/history permissions. They receive
`artifact_checks_require_cli` instead of expanded artifact/Git observations;
no new filesystem or Git permission is implied. Existing writer grants continue
to write their legacy checkpoint format and cannot submit artifact references.

## Migration and verification

Database markers 1–14 upgrade to **15**, after a consistent
`backups/pre-schema-15-*.db` is published under the writer lock. A failed backup
aborts migration. Older binaries refuse marker 15. Roll back by stopping the
daemon and opening the pre-upgrade snapshot with its original compatible binary
in a fresh directory with matching `types.yaml`. Post-upgrade events are absent
from that snapshot. See [backup instructions](CONTINUITY-SETUP.md#backup-upgrade-and-restore).

Request shapes and golden events are frozen in [artifact fixtures](../../testdata/artifacts/README.md).
Go tests cover Git states, confinement, grant boundaries, competing version
heads and replay. `node scripts/artifact-smoke.cjs` uses the compiled daemon on
Linux with synthetic files, including SIGKILL/restart and fresh-directory backup
restore after deleting the originals. [Verification](../VERIFICATION.md) records
the actual schema-8 binary upgrade/rollback and editor/regression results.

Content preservation, remote/multi-host observations, decision review and
workspace recovery remain separate roadmap work.

Artifact `show`/`list` now expose P02 `lifecycle` and `progress_id`. Review of an exact version uses the [progress CLI](PROGRESS-SETUP.md); a new version starts in draft and review does not complete its task. Current daemon schema is **21**; historical P01 payloads remain unchanged.
