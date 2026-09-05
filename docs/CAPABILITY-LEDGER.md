# Capability and authority ledger

2026-09-04, daemon build 0.5.0. Scoped reads and explicit checkpoint writes are exposed through the API and MCP adapter.

| Surface / identity | Implemented authority | Evidence / limit |
|---|---|---|
| Local CLI bearer | All existing CLI reads and mutations, accepted continuity records, resource registration and database backup | Authenticated loopback, exact Host, no Origin; daemon derives `actor=cli`. Caller-supplied actor is rejected. This is a trusted local credential, not cryptographic proof a human typed the command. |
| Browser-ingress bearer | Paired browser protocol ingress only | Separate credential. Cannot read task/context state, accept contracts, write checkpoints or request backups. Existing route tests plus continuity route negatives pass. |
| Browser profile | Inventory/transport identity after explicit pairing | Profile IDs are not daemon principals and cannot grant task or filesystem access. Browser content cannot authorize a command. |
| Scheduler | Existing timer/proposal transitions only | Cannot emit CLI-only continuity events. Replay never performs external effects. |
| Scoped reader | Task/subtree record and history reads; context only with all mandatory lineage/resource permissions | Separate hashed credential verifier, expiry/revocation, bounded pages, cursor-scope checks and restart rediscovery. No mutation authority. See [SCOPED-ACCESS.md](SCOPED-ACCESS.md). |
| MCP client | Task/context/history tools; checkpoint tool requires a write grant | Official SDK stdio adapter; authenticated grant provenance, authorization before dedupe/commit, structured errors and restart tests. No contract, decision, resource or completion authority. |
| GUI session | Unavailable for task UI | Extension popup remains available; no new task dashboard or browser-accessible continuity route. |
| Execution host | Unknown / unavailable | No verified launch, attach, reconcile, status, cancellation or wake adapter; no automatic run resumes. |
| Braid | Not integrated | No Heimdall index publication or retrieval path. Separate repository/toolchain convenience conveys no authority. |

Resource binding authorizes observation through the trusted CLI boundary. A scoped reader additionally needs that binding ID in its grant for live context observations. Neither permits reading file contents through the client API, executing work or locking a workspace. Checkpoint text and `ready` status never imply permission to execute or accept completion.

## Required S2 policy

The current credential maps to one immutable read grant; identity is daemon-authenticated, not supplied attribution text. Scope, binding IDs, expiry and revocation are checked on each read. Revocation is serialized with authorization and response construction. Grant v1 permits only reads. Grant v2 can add checkpoint writes; authorization is checked before dedupe and again before commit, including retries. Broader mutation policy remains deferred; replayed history is not a renewed grant. A machine client cannot acquire authority by declaring `actor=cli`, choosing a known task ID, retrieving text, or submitting a contract as accepted.

Start with bounded reads and explicit checkpoint/progress grants. Reserve accepting contracts/decisions, changing bindings, ratifying completion and granting authority for the user-controlled surface unless a later policy explicitly delegates that operation. Distinguish inherited visibility from mutation authority; applying a grant to one child must not expose siblings or let it replace ancestor contracts.

MCP and daemon tests now cover wrong-scope reads/writes, sibling/ancestor access, forged authority, expiry, revocation, credential separation, duplicate commands after revocation, pagination/cursor scope and reconnect. The daemon remains the sole writer. MCP capabilities are advertised after adapter/handshake acceptance; per-host installation remains separate. Do not hand the unrestricted CLI token to an MCP client as a shortcut.
