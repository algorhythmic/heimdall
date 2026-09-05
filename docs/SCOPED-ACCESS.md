# Scoped client access — 0.5.0

Heimdall now supports separate, expiring read credentials for a task or task subtree. Clients can read permitted task records, bounded continuity history and mandatory context. The daemon remains the only writer. The MCP adapter and explicit checkpoint-write option are now available; see MCP-SETUP.md. The instructions below describe default read access.

## Create a reader

With the daemon running and an existing task, use PowerShell 7:

```powershell
$data = '.\demo-data'
$expires = [DateTime]::UtcNow.AddHours(8).ToString('o')
.\bin\heimdall.exe grant issue feature-work --name 'Feature reader' --expires $expires --output .\feature-reader.credential.json --data-dir $data
.\bin\heimdall.exe client task feature-work --credential .\feature-reader.credential.json
.\bin\heimdall.exe client history feature-work --kind checkpoint --limit 10 --credential .\feature-reader.credential.json
.\bin\heimdall.exe grant list --data-dir $data
```

The default grant covers the named task, including its steps. Add `--subtree` to include descendant tasks. It does not grant access to parents or siblings. Task contents can contain references to other tasks; those references do not authorize fetching the referenced records.

Live resource observation needs explicit binding IDs: add `--resources ID1,ID2` at issuance. Those IDs must be active bindings within the granted task scope. Clients may read binding metadata in their permitted task's history; the additional resource permission authorizes the file/tree digest observations used by `client context`. It does not provide file contents or arbitrary filesystem reads.

`client context TARGET --credential FILE --budget 16000` requires permission for **all** mandatory ancestor context and active inherited resource bindings. A grant limited to a child cannot retrieve its parent's context. The command refuses that request rather than silently omitting required context. Grant the appropriate project subtree when project-level context is intended. A newly added binding is not automatically authorized by an existing read grant.

## Credential lifecycle

Issuance writes a new private credential file before making the request. The file contains a random 256-bit token, daemon data-directory location, and the exact issuance request. Existing files are never overwritten. The token is not printed. The daemon stores only its SHA-256 verifier, and ordinary grant listings omit even that verifier. Windows file protection follows the selected directory's ACL; Unix mode 0600 alone does not establish a Windows process isolation boundary.

If issuance has an uncertain result, retry the saved request:

```powershell
.\bin\heimdall.exe grant activate --credential .\feature-reader.credential.json
```

This returns the original issuance receipt. It does not extend expiry or reactivate a revoked grant. Inspect `grant list` for current state. Expiry is required, must be in the future, and is limited to 30 days. Client timestamps do not control authorization.

Use the returned grant ID to revoke it:

```powershell
.\bin\heimdall.exe grant revoke GRANT_ID --data-dir $data
```

Revocation and expiry are checked on every read, including later history pages. Revocation persists across restart and replay. A read already authorized and constructed may still finish transmission; revocation cannot recall bytes already released. Authorization and resource observation run under the same store lock as revocation and other writes, and expiry is rechecked after constructing the response.

To rotate a credential, revoke its old grant and issue a new grant into a new file. Credentials survive ordinary daemon restarts. Clients rediscover the random loopback port from `client-endpoint.json`, which contains no credential, rather than reading the CLI or browser credential files. A process that can independently read the unrestricted CLI token is outside the isolation provided by this scoped API; do not give that token or the private data directory to a client as a shortcut.

## Read API and pagination

| Method / route | Behavior |
|---|---|
| `GET /client/task?target=ID` | One authorized task record; task-wide scope also permits its steps |
| `GET /client/history?target=ID&kind=checkpoint&limit=20` | Exact-target history; kind is contract, decision, resource or checkpoint |
| `GET /client/context?target=ID&budget=16000` | Complete authorized mandatory context and live observations |

Only the scoped credential is accepted on these routes. The CLI and browser credentials do not act as client credentials. A client credential is rejected from existing CLI/browser routes, grant administration and backups. Default read grants also reject checkpoints; only an explicit checkpoint-write grant can use POST /client/checkpoint. No request field can supply authority. Origin-bearing requests and wrong Host values are rejected.

Responses are capped at 512 KiB. History pages contain 1–50 records, subject to that byte limit, ordered by immutable ID. `next_cursor` is passed back as `--cursor CURSOR` or the HTTP `cursor` parameter. Cursors are untrusted positions tied to grant, target, kind and event boundary; each page is independently authorized. Any intervening event returns conflict and requires starting a fresh page sequence. Cursors do not grant access. Context budgets remain byte-based estimates and fail explicitly when required context cannot fit.

The unrestricted local CLI's existing `checkpoint list` and other history views remain unchanged. Machine clients use the bounded `/client/history` route.

## Persistence and recovery

Database schema 5 retains read grants and reviewed contracts while adding version-2 checkpoint-write grants and client checkpoints. Markers 1–4 upgrade only after a consistent `pre-schema-5-*.db` backup. Version-1 contracts and checkpoints remain replayable; an old contract has unreviewed resource scope and must be explicitly reaccepted before recording a new checkpoint. Supply the sorted, complete `resource_ids` list reviewed for the task lineage; a mismatch rejects acceptance. Adding/removing bindings makes the corresponding accepted contract stale even before a checkpoint exists.

Database backups contain grant verifiers and revocation state as of the snapshot, but no raw client tokens. Restoring an older snapshot also restores its old grant state. Review and revoke restored grants before reconnecting clients; otherwise a grant revoked after the snapshot could become valid again. Copy the matching catalog and restore into a fresh directory as described in [Continuity setup](CONTINUITY-SETUP.md).

Wire administration shape: [grant-request-v1.schema.json](../schemas/grant-request-v1.schema.json). Fixed persisted examples cover old/new contracts, checkpoints, resources, decisions and grant issue/revoke in [events-v1.json](../testdata/continuity/events-v1.json). Golden replay never opens the synthetic resource path.

## Remaining work

C06 now includes explicit checkpoint-write grants, authenticated provenance and transactional authorization before dedupe/commit. C07 supplies tested MCP stdio transport. Host-specific registration and Linux runtime acceptance remain open. Proposed/rejected decision workflow, Git/source identity, evidence evaluators, Braid integration, task GUI and execution-host coordination remain later work. No client, extension or system service was installed by this patch.
