# Workspace and session declarations

W01's first implementation records desired workspace membership and explicit
terminal session bindings. These are CLI-authored declarations. The generic
adapter does not inspect processes, canonicalize paths, check session existence,
publish herdr metadata, launch programs or operate the desktop. Generic bindings are reported as `unverified` in `workspace show`. Live Linux Herdr bindings are now available through the separate [T02 commands](HERDR-SETUP.md); saved v2 bindings are `recorded` until freshly checked.

## Create a desired workspace

Use a running daemon and an existing task, for example `alpha`. Inspect its
revision with `heimdall state alpha`. Save this input as `workspace.json`:

```json
{
  "previous": "none",
  "name": "Planning alpha",
  "surfaces": [
    {
      "id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "kind": "terminal",
      "label": "Planning shell",
      "required": true,
      "restore_policy": "manual"
    }
  ]
}
```

```sh
heimdall workspace accept alpha --expected-task-revision 1 \
  --request-id 11111111111111111111111111111111 --file workspace.json
heimdall workspace show alpha
```

Replace the example IDs with fresh 32-character lowercase hexadecimal IDs for
your own records. One way to generate an ID is
`python3 -c 'import secrets; print(secrets.token_hex(16))'`. Keep each logical
surface ID across manifest revisions and session replacements. Each new command
needs its own request ID; reuse that request ID and exact input when retrying an
uncertain command. Flags can include `--data-dir PATH` as usual.

There is one current manifest per task. A revision replaces the complete desired
surface list, using the existing manifest ID as `previous`. Up to 32 surfaces are
allowed. Supported kinds are `terminal`, `editor`, `browser` and `native`; only
terminal surfaces can have session bindings in this version. All restore policies
are `manual`. An empty list is an explicit empty workspace.

A surface ID permanently belongs to its task and kind, including after removal.
Unbind an active session before removing its surface. Accepting a changed task
requires the current task revision. Workspace changes do not change task status,
accepted contracts, checkpoint heads or `tasks.yaml`.

## Bind, inspect and replace a terminal session

Save `session.json`, substituting the manifest and surface IDs returned above:

```json
{
  "previous": "none",
  "manifest_id": "11111111111111111111111111111111",
  "surface_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "locator": {
    "adapter": "generic",
    "environment": "local",
    "host": "synthetic-host",
    "source_epoch": "server-instance-1",
    "session_id": "planning",
    "workspace_id": "workspace-1",
    "pane_id": "pane-1",
    "platform": "linux",
    "cwd": "/synthetic/shared-worktree"
  }
}
```

```sh
heimdall session bind alpha --expected-task-revision 1 \
  --request-id 22222222222222222222222222222222 --file session.json
heimdall workspace show alpha
heimdall session show alpha --surface aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
```

The generic identity contract requires pane IDs to be unique within the declared
session and source epoch. Use a distinct epoch for each source/server instance;
reusing runtime IDs from a restarted server does not establish continuity. Host
and environment labels are caller declarations, not authenticated machine IDs.
Changing a declared workspace or cwd cannot claim an already-bound pane for a
second surface. Two tasks may declare the same cwd while owning different panes.
An adapter will need to verify actual assigned identities before using a binding
to select a live session. Do not label inferred herdr IDs as verified.

`platform` accepts `linux`, `darwin` or `windows`. `cwd` is an absolute declared
path in that platform's syntax. Optional `repository` and `worktree` paths must
be supplied together; optional `agent_session_id` is also a declaration. Paths
are retained as provided, without filesystem or Git verification. These fields
do not create resource bindings or grant file access.

To replace a session, submit `session bind` with a **new request ID**, the current
binding ID as `previous`, the current manifest ID and the replacement locator.
The logical surface ID stays the same. To remove the declaration, save an input
with `previous`, `manifest_id` and `surface_id`, omit `locator`, then run:

```sh
heimdall session unbind alpha --expected-task-revision 1 \
  --request-id 33333333333333333333333333333333 --file unbind.json
```

Unbind closes no application. It remains possible after the task changes, using
the current task revision and current heads. A fresh bind requires a manifest
reviewed at the current task revision. Changes to a task or manifest appear as
explicit issues in `workspace show`; reads never silently refresh bindings.

Current and historical records are distinct. Use `workspace show TASK --id ID`
for an immutable manifest, or `session show TASK --surface SURFACE --id ID` for a
binding record. An old exact retry returns its original receipt even after
replacement or unbinding; inspect current state separately. Record lookups check
the exact task and surface; repository paths never select the task. Missing or
stale task/head preconditions are rejected, and input files are retained.

## Storage, transport and remaining work

- CLI routes: `GET /workspace/state?target=TASK`,
  `GET /workspace/manifest?target=TASK&id=ID`,
  `GET /workspace/session?target=TASK&surface=ID[&id=ID]`, and
  `POST /workspace/command`. All use the existing local CLI bearer and Host/Origin
  guards. Browser, scoped client, MCP and GUI authority is unchanged.
- [Request schema](../schemas/workspace-request-v1.schema.json) and
  [request/event fixtures](../testdata/workspace/README.md) freeze version 1.
  Requests and records are limited to 64 KiB; scoped current/individual reads to
  512 KiB. There is no unbounded history endpoint. Unknown fields, versions,
  control characters and launch policies are rejected.
- Database marker 15 retains workspace/session, observed Herdr records and [P01 artifacts](ARTIFACT-SETUP.md), and adds [P02 progress review](PROGRESS-SETUP.md). Before upgrading an
  existing marker 1–8 database, Heimdall creates a consistent
  `backups/pre-schema-15-*.db`. Older binaries refuse marker 15. To roll back, stop
  Heimdall, preserve the current data directory, and copy the pre-upgrade database
  as `heimdall.db` into a **fresh** data directory with the compatible `types.yaml`.
  Start the old binary against that directory. Post-backup changes are absent;
  the database backup does not include endpoint credentials or external files.
- Replay only reduces recorded events. It never inspects paths, attaches to
  sessions, publishes metadata or executes restoration. Historical task,
  contract, checkpoint, grant and command receipts retain their authority.

W01 remains partial: observed snapshots/topology and recovery/action envelopes
will be versioned with their consumers. The initial T02 CLI adapter now supplies canonical repository/worktree checks, installed Herdr identity verification, explicit reconciliation and expiring metadata; see [Herdr setup](HERDR-SETUP.md). Automatic refresh and recovery remain open. Initial [T03 editor integration](NEOVIM-SETUP.md) is delivered; W02 desktop observation, W03 autosave and later recovery operations remain open. No live desktop acceptance is claimed for
these declarations.
