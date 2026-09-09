# P03: optional planning preservation

Heimdall can preview a manual dotprivate handoff, retain its exact checkpoint and
artifact selection, and record independent observations of its result. Local
checkpoints do not depend on copying or remote publication. Start the daemon with
`./bin/heimdall start --data-dir /path/to/data`; use that same data directory below.

This is the roadmap's initial manual P03 slice. Heimdall does not call dotprivate,
copy files, stage, commit, pull, push or retry an external operation. Programmatic
preservation waits for C12 shared actions and a narrow selected-files interface.
The existing dotprivate 0.1.0 implementation at sibling revision `51d0fce` calls
`git add -A` in its private clone. Selected input arguments do not constrain that
commit; an unchanged-file ingest can also skip synchronization entirely.

## Prepare and preview

First record an artifact and save a checkpoint that pins its exact version using
[artifact setup](ARTIFACT-SETUP.md). Preservation accepts only selections already
pinned in that saved checkpoint. It never registers `.private` as a resource or
changes a contract's scope. Historical checkpoint selections remain inspectable.

Save this full version-1 envelope as `preservation.json`. Use fresh 32-character
lowercase hexadecimal IDs and the task's current revision. Sort selections by
artifact ID. Supply the private clone explicitly and confirm each mirror path
against dotprivate's dry run; Heimdall does not infer project or environment.

```json
{
  "version": 1,
  "id": "11111111111111111111111111111111",
  "target": "my-project",
  "expected_task_revision": 1,
  "plan": {
    "checkpoint_id": "22222222222222222222222222222222",
    "private_root": "/path/to/project/.private",
    "remote": "origin",
    "ref": "refs/heads/main",
    "selections": [
      {
        "artifact_id": "33333333333333333333333333333333",
        "version_id": "44444444444444444444444444444444",
        "mirror_path": "project/ingested/local/notes.md"
      }
    ]
  }
}
```

```bash
./bin/heimdall preservation preview my-project --file preservation.json --data-dir /path/to/data
```

Preview is read-only and does not contact the remote. It reports source/mirror
bytes, committed bytes, repository state, unrelated dirty/staged path counts, and
manual handoff source paths. Credential/database-looking names and symlinked files
are refused. Counts are conservative: Git stat mismatches can require inspection
even when bytes are unchanged. Ignored paths are outside the set `git add -A`
would stage; no claim is made that other files are safe to publish.

Inspect the preview. Reconcile unrelated private changes and any rebase/merge
conflict before a selected-files handoff. Add its exact `digest` to the request as
`"preview_digest": "..."`, then save the local request:

```bash
./bin/heimdall preservation request my-project --file preservation.json --data-dir /path/to/data
```

Heimdall reobserves and refuses a changed preview. The returned plan records a
manual handoff request, including blocked conditions; it does not claim execution.
Keep the request file for exact retries. Request IDs cannot be reused with changed
content. A conflict requires a new reviewed preview and a new request ID.

## Manual handoff and reconciliation

From the project directory, inspect dotprivate's own dry run for the exact selected
originals, for example `dotprivate ingest --dry-run -- notes.md`. Its mirror layout
uses the first non-template sparse project, the dotprivate environment label, and
`_external/` for originals outside the project. Check this matches your selection.
Perform preservation manually after inspecting the clone. Heimdall never launches
the command from stored task text or these records.

Save a separate observation envelope. `previous` is `none` initially, then the
last receipt ID from `preservation show`. A report is the operator's account of the
manual operation, separately labelled from filesystem/Git readback.

```json
{
  "version": 1,
  "id": "55555555555555555555555555555555",
  "target": "my-project",
  "expected_task_revision": 1,
  "observe": {
    "plan_id": "11111111111111111111111111111111",
    "previous": "none",
    "check_remote": true,
    "reported_outcome": "push_failed",
    "note": "The manual push failed; inspect the independently observed result."
  }
}
```

```bash
./bin/heimdall preservation observe my-project --file observation.json --data-dir /path/to/data
./bin/heimdall preservation show my-project --id PLAN_ID --data-dir /path/to/data
./bin/heimdall preservation list my-project --limit 25 --data-dir /path/to/data
```

`check_remote: true` explicitly permits a bounded read of the plan's pinned push
URL (fetch URL when no push URL is configured). URLs are fingerprinted locally,
never stored in events or exports. Changing that destination requires a new plan.
The query runs outside the clone with repository/global Git helpers and URL
rewrites disabled. HTTPS, SSH and absolute local remotes are supported; credentials
helpers and interactive authentication are not. SSH can use the current agent and
user SSH configuration. An unavailable authenticated remote is reported as such.
Multiple push URLs are unsupported. No Git fetch updates a local tracking ref.

The remote read uses [Git ls-remote](https://git-scm.com/docs/git-ls-remote) and
requires an exact branch-ref response. Only equality with the observed local HEAD
confirms publication. An advanced or different remote tip remains `different`;
this slice does not fetch ancestry to infer inclusion.

| Fact | Meaning |
|---|---|
| Source | Saved-version digest versus current original: matched, changed, missing, refused, unavailable, changed during observation, host or scope changed |
| Mirror | Independently read selected mirror digest and byte count |
| Committed | Raw blob bytes at the recorded private HEAD, without content filters |
| Remote | Not checked, unavailable, missing ref, matched HEAD, different tip, or changed configuration |
| Stage | `not_preserved`, `mirrored`, `committed`, `published`, or `uncertain` when repository state changed during observation |
| Report | Operator-supplied outcome and note; it never promotes an observed stage |

Reported outcomes are `not_reported`, `succeeded`, `offline`, `refused_file`,
`missing_original`, `rebase_conflict`, `source_changed_during_copy`, `push_failed`,
or `unknown`. All except `not_reported` require a note. A post-operation observer
cannot prove what happened during a manual copy; later source changes are observed
independently. Source edits or deletion do not erase an earlier confirmed copy or
commit of the checkpoint's bytes. Receipt timestamps identify the start of the bounded observation, not
permanent remote availability. Inspect the complete fact set rather than only stage.

Repeated `observe` with the same envelope returns its exact original receipt,
even after the files disappear. Use a new ID and current `previous` for fresh
reconciliation. Status, replay and retry never dispatch an operation. Requests
are bounded to 64 KiB and 16 selections, file reads to 64 MiB; local observation
has a six-second budget and remote read has a three-second sub-budget.

## Portable export and support

```bash
./bin/heimdall preservation export my-project --checkpoint CHECKPOINT_ID --data-dir /path/to/data > progress.json
```

Export explicitly includes checkpoint summary, next action, blockers, saved
accepted-decision text, artifact names, IDs and digests. It excludes operational
credentials, resource roots, private remote settings, session identities and the
live database. User-authored text is preserved verbatim: inspect that text before
sharing it. Export does not copy artifact contents or attest completion.

Observation requires Linux and a standalone private clone with an initial commit.
Detached checkout, missing files, unsupported repositories and unavailable remotes
remain visible. Windows can inspect/replay/export recorded data but cannot create
local artifact observations. Read grants, browser credentials and MCP checkpoint
grants acquire no preservation authority. Commands require the existing local CLI
credential; the schema-12 migration preserves old events and receipts.

See [request schema](../schemas/preservation-request-v1.schema.json),
[verification](VERIFICATION.md), and [roadmap dependency gates](design/history/REVISED-ROADMAP-IMPLEMENTATION-PLAN.md).
