# Live Herdr bindings on Linux

The initial T02 adapter supports local **Herdr 0.8.2, API protocol 20, on Linux**.
It binds explicitly selected terminal panes to W01 task/surface identities,
checks live identity on request, and publishes expiring display metadata.
It does not launch agents, send terminal input, change agent runtime status or
complete tasks. Other Herdr versions and platforms fail explicitly until tested.

## Bind an actual pane

First accept a task-owned terminal surface using
[workspace setup](WORKSPACE-SETUP.md). Read the task revision, current manifest ID
and binding head with `heimdall workspace show TASK`.

Get the actual API socket from `herdr session list --json` and actual IDs from
`herdr pane list --workspace WORKSPACE_ID`. When running inside Herdr, inherited
environment variables can help select a candidate, but Heimdall never treats them
as proof of ownership or follows focused-pane defaults. Herdr changes the public
pane ID after a move into another workspace; rediscover the current ID explicitly.

```sh
heimdall session bind-herdr alpha \
  --surface aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --manifest 11111111111111111111111111111111 \
  --previous none --expected-task-revision 1 \
  --socket /actual/session/herdr.sock --pane w1:p1 \
  --request-id 22222222222222222222222222222222
```

Replace the examples with your own IDs, socket and revision. The returned binding
is version 2. Its ID is the command's request ID. A new bind needs a new request
ID; an uncertain retry uses the same ID and exact selectors/preconditions.
Successful retries return the original record without observing a replacement
server or creating a second binding.

Heimdall checks:

- A canonical, user-owned Unix socket and the same-user Linux peer. Every RPC
  checks the socket and server instance. The source epoch includes boot identity,
  peer PID/start time and socket device/inode/change timestamp; a reconnect never
  silently adopts another server. Host identity is a hash of the local machine ID. These checks
  establish a local endpoint identity, not remote authentication or binary attestation.

  The socket fingerprint now includes the full change timestamp because a
  same-process listener replacement can reuse an inode. Bindings recorded before
  this fix report `stale`/`source_changed`; explicitly rebind after checking the
  live pane. Socket metadata changes also require revalidation. Historical
  records and exact command retries retain their original observations; database
  schema remains 9.
- Exact pane, terminal, workspace and tab IDs from the installed API. A second
  observation must agree. The terminal ID prevents a moved pane from being
  adopted by a second task through its newly assigned public pane ID.
- The live shell process and its start time from `/proc`. Its canonical cwd must
  agree with Herdr's reported foreground cwd. Differing or unavailable paths are
  refused; OSC titles and inherited environment are not authority.
- Canonical Git worktree and common-directory identity, where Git applies.
  `locator.repository` is the shared Git common directory, including for linked
  worktrees; `locator.worktree` is the selected worktree root. Non-Git directories
  are supported. Malformed Git metadata, missing Git or ambiguous process cwd
  fails visibly. This records path identity, not a commit, dirty state or retained
  artifact bytes.
- If Herdr reports an agent session, its explicit ID and source/kind are retained
  and compared on refresh. A path-only session reference is not accepted as an ID.
  Herdr agent `idle`, `done` or `unknown` status does not affect task completion.

The daemon performs these bounded reads through its existing writer transaction.
The request cannot submit its own observed identity or author. Bind requests have
a five-second deadline; no filesystem or session observations occur during replay.

## Check, reconcile and unbind

```sh
heimdall session refresh alpha --surface aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
heimdall session show alpha --surface aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
```

`refresh` performs a fresh read and returns `current`, `stale` or `disconnected`
with issue codes and a check time. It never changes a binding. A saved v2 binding
is shown as `recorded` by `workspace show`; that is not a live status. Generic v1
bindings remain `unverified` and cannot publish Herdr metadata.

After a pane move, server restart, cwd/worktree change or agent-session change,
inspect the new identity and run `bind-herdr` with the same logical surface ID,
the current binding ID as `--previous`, current manifest/task preconditions and a
new request ID. Changed tasks require a newly accepted manifest first. There is
no automatic reassignment from cwd, window class, title or process hints.

Use the existing `session unbind TASK --file INPUT --expected-task-revision N`
with `previous`, `manifest_id` and `surface_id` to remove a declaration. It closes
no pane. Historic records remain available through `session show ... --id ID`.

## Publish task display metadata

```sh
heimdall session publish alpha \
  --surface aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --binding 22222222222222222222222222222222 \
  --request-id 33333333333333333333333333333333
```

Publication requires an exact current binding, rechecks its live source, and
writes only namespaced metadata. It reads the values back before reporting
`published`. The title and tokens contain task identity, next action, recorded
review counts, direction provenance and check time. Checkpoint direction is used
only while its recorded lineage and accepted decisions still match; otherwise
task direction is used with `checkpoint_needs_review`. No live resource/evidence
evaluation runs while publishing metadata. Control characters are removed and
display text is bounded.

Metadata expires after **30 seconds**. There is no background publisher installed.
`refresh` exposes stale/disconnected state in Heimdall; expiry removes old display
values when publication stops. The check time describes a bounded observation,
not a guarantee that the process remains unchanged afterward.

Herdr's default plain-shell view does not display pane metadata. For a visible
sidebar summary, explicitly add `--workspace-summary` and merge the
[example configuration](../examples/herdr-sidebar.toml) into your chosen Herdr
configuration. Heimdall does not install or edit it. The summary names the selected
pane (for example `w1:p1 | Heimdall alpha`) and its next action; it does not assign
the entire workspace or other panes to that task. When several publishers share
a workspace, the latest visible summary describes that named pane. Workspace
labels and agent runtime state remain Herdr's own values.

Pane tokens are `heimdall_task`, `heimdall_title`, `heimdall_next`,
`heimdall_review`, `heimdall_binding`, `heimdall_state`, `heimdall_direction` and
`heimdall_checked_at`. Optional workspace tokens are `heimdall_summary`,
`heimdall_next`, `heimdall_review` and `heimdall_checked_at`.

Unconfirmed writes return `unconfirmed`, without claiming readback success. A
committed command receipt prevents re-emission on retry or replay, even after
unbinding or server replacement. Inspect current state separately from an old
receipt. A crash or failed SQLite commit after publication can lead to reapplying
the same idempotent display values; sequence numbers and TTL bound this effect.
This is not the future journal for execution or recovery actions.

## Compatibility and acceptance

The CLI-only routes are `POST /workspace/herdr/bind`,
`GET /workspace/herdr/refresh?target=TASK&surface=ID[&binding=ID]`, and
`POST /workspace/herdr/publish`. Separate
[bind](../schemas/herdr-bind-request-v1.schema.json) and
[publish](../schemas/herdr-publish-request-v1.schema.json) request schemas reject
caller-supplied observations and authority fields. Browser, scoped client, MCP and
GUI credentials gain no access to these routes.

Database marker **11** retains declarations/observed bindings and [P01 artifact identity](ARTIFACT-SETUP.md), and adds [P02 progress review](PROGRESS-SETUP.md). A
consistent `backups/pre-schema-11-*.db` is required before upgrading markers 1–10.
Older binaries refuse marker 11. Roll back from the pre-upgrade backup into a fresh
directory using the procedure in [workspace setup](WORKSPACE-SETUP.md); changes
after that backup are absent. Schema-6 and actual W01 schema-7 fixtures remain in
the repository. Endpoint credentials and external files are not part of a database
backup.

`node scripts/herdr-smoke.cjs` is a separate installed-Linux gate. It creates its
own short `/tmp` runtime/config tree, synthetic tasks/repository and a dedicated
Herdr server. It tests exact IDs, same-repository task separation, movement,
source replacement, metadata readback/expiry and Heimdall crash/replay, then stops
its servers. It never selects or stops the user's existing Herdr session. Go
tests use fake sockets for malformed responses, mismatched identities, path and
Git failures, metadata uncertainty, scoped ownership and replay.

The initial T02 CLI workflow is implemented. Automatic refresh, background metadata,
process resurrection, remote Herdr sessions, broader version/platform support,
editor integration and workspace recovery remain separate work.
