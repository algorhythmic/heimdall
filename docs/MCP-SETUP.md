# MCP and scoped progress writes — 0.6.0

The daemon now supports explicitly granted checkpoint writes, and the Go MCP adapter exposes task, context, history and checkpoint tools over stdio. The daemon stays running as the sole database writer. Each MCP adapter is a separate lightweight process using a scoped credential and the daemon's loopback API; it does not start a daemon, open the database or move runtime state into the extension.

## Prepare access

Start the daemon using the README instructions. Create the task, bind its resources and accept its reviewed contract using [Continuity setup](CONTINUITY-SETUP.md). Then issue a credential. In PowerShell 7, from the Heimdall project:

```powershell
$expires = [DateTime]::UtcNow.AddHours(8).ToString('o')
.\bin\heimdall.exe grant issue feature-work --name 'Feature worker' --expires $expires --checkpoint-write --output .\feature-worker.credential.json --data-dir .\demo-data
```

Omit `--checkpoint-write` for a reader. Add `--subtree` when the intended scope includes descendant tasks. If the task has resource bindings, also supply `--resources ID1,ID2` with their reviewed IDs. Complete mandatory ancestor visibility and permission for every observed binding are required for context and checkpoint writes. A child-only grant cannot read or checkpoint across an ungranted parent scope.

Existing read credentials stay read-only after upgrade. Write authority requires a newly issued grant with the explicit write option. Only the trusted CLI can issue/revoke grants, accept contracts or decisions, bind resources or ratify completion.

## Launch from an MCP host

Configure the host to launch the executable with `mcp --credential ABSOLUTE_FILE_PATH`, using stdio. A host that accepts the common `mcpServers` JSON format can use this template with its actual paths:

```json
{
  "mcpServers": {
    "heimdall": {
      "command": "C:\\path\\to\\Heimdall\\bin\\heimdall.exe",
      "args": ["mcp", "--credential", "C:\\private\\feature-worker.credential.json"]
    }
  }
}
```

The token stays in the private file; it is not a command-line argument, stdout diagnostic or event payload. Stdout is reserved for MCP. Errors go to stderr. The adapter discovers the current port through `client-endpoint.json` before each tool request, so an already-running adapter can reconnect after a daemon restart. It never falls back to the unrestricted CLI credential or starts an inference provider.

This patch supplies the executable and tested interface. It does not register it in a user's host settings, install a plugin or create a background service. Host-specific UI/registration acceptance remains separate.

## Tools

| Tool | Inputs and result |
|---|---|
| `heimdall_task` | `target`; returns one authorized task record, including its revision |
| `heimdall_context` | `target`, optional `budget` (default 16000); returns mandatory accepted context, checkpoint and authorized live observations |
| `heimdall_history` | `target`, `kind`, optional `limit` and `cursor`; bounded exact-target history page |
| `heimdall_checkpoint` | Explicit request ID, task revision, contract/head preconditions and progress fields; returns an immutable checkpoint with authenticated author/grant provenance |

Example checkpoint arguments, substituting the actual IDs/revision from task/context:

```json
{
  "target": "feature-work",
  "request_id": "0123456789abcdef0123456789abcdef",
  "expected_task_revision": 1,
  "previous": "none",
  "contract_id": "fedcba9876543210fedcba9876543210",
  "summary": "Implementation saved; focused checks still pending",
  "next_action": "Run the relevant checks and inspect their results",
  "blockers": []
}
```

Use a fresh random 32-character lowercase hexadecimal `request_id` for each logical write. `previous` is the current checkpoint ID, or the explicit sentinel `none` only when none exists. `current_step` is an optional local step ID. The accepted contract must still match the task revision and reviewed resource scope.

After an uncertain response, repeat **exactly the same arguments and request ID**. The adapter does not retry writes automatically or generate a new ID. An authorized retry returns the original receipt without observing resources again. Changed arguments with the same ID conflict. Expired, revoked, wrong-scope or different-grant retries cannot retrieve that cached result. After rotating to a new credential, inspect current context/history before starting new work rather than resubmitting a write from the old grant.

The checkpoint's `actor` is `client:<grant-id>`, and its version-2 record includes `grant_id`. The daemon derives both; tool arguments cannot forge them. A summary is a progress claim. Recording it cannot complete a task, change its contract or turn claimed test results into verified evidence.

## Ordinary CLI access

The same granted operation is available without MCP:

```powershell
.\bin\heimdall.exe client checkpoint feature-work --credential .\feature-worker.credential.json --expected-task-revision 1 --request-id 0123456789abcdef0123456789abcdef --file checkpoint.json
```

The file contains the usual checkpoint input from Continuity setup (`previous`, `contract_id`, `summary`, `next_action`, `blockers`, optional `current_step`). The client command requires an explicit request ID. It uses `POST /client/checkpoint`; no other client mutation route is enabled.

## Errors, limits and persistence

Tool failures return `isError=true` with structured `code`, `message`, and retry guidance. Codes include `access_denied`, `conflict`, `budget_too_small`, `daemon_unavailable` and `invalid_request`. Budget errors retain the required estimate. Conflicts require inspecting current state; authorization errors require reviewing the grant. A disconnected daemon is visible and does not invent saved progress.

Tool arguments and daemon checkpoint requests are limited to 64 KiB; stdio input lines are limited to 128 KiB. Daemon responses are limited to 512 KiB. History allows 1–50 records per page and reauthorizes each cursor. Context keeps the existing byte-based token estimate and mandatory-context refusal behavior. Filesystem observations remain bounded, without a filesystem lock or Git commit/ref capture.

Authorization runs under the writer transaction before cached-result lookup and again before committing new events. Revocation is serialized with those checks; expiry is evaluated against the daemon clock. A transaction that loses authorization before commit rolls back its events and receipt. Already-released response bytes cannot be recalled.

Database schema 5 introduced version-2 write grants and client checkpoints; the current schema 6 adds evaluator/evidence records without broadening MCP authority. Old read grants, CLI checkpoints and contract versions remain replayable. Startup creates a consistent `pre-schema-6-*.db` backup before upgrading markers 1–5. As with earlier releases, restoring an old snapshot restores its old revocation state: review/revoke restored grants before reconnecting clients. No raw client tokens are stored in the database.

## Compatibility and verification

The adapter pins the [official Go MCP SDK v1.7.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0), which supports the current revision and legacy negotiation. Its dependency requires Go 1.25; this build was developed with Go 1.27.1. [SDK module declaration](https://github.com/modelcontextprotocol/go-sdk/blob/v1.7.0/go.mod)

Tests exercise an official SDK client through the scoped HTTP daemon using protocol `2026-07-28`, plus the compiled stdio adapter using legacy `2025-11-25` initialization. The compiled check covers discovery, reads/writes, exact retries, stale heads, wrong-project denial, daemon restart, replay, revoked retries and the sole-writer lock. Reproduce with `node scripts/mcp-smoke.cjs` and `go test ./...`. CI includes the Windows smoke check; remote CI and user-host registration have not been run here. Linux remains a cross-build until tested on Linux.

Dependency root notices are retained under [licenses](licenses/INDEX.txt), including the SDK's full current license/transition notice. Include these notices with redistributed artifacts.

The next implementation slice is evidence capture/evaluation and completion revalidation. Proposed/rejected decisions, Git/source identities, Braid integration, the task GUI, verified browser actions and execution-host coordination remain tracked in the backlog.
