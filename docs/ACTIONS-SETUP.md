# Shared action journal

C12 introduced task-bound authorized intent, one immutable attempt identity and separate
execution/verification states. The first consumer is the browser bridge. Schema
16 introduced this history; the current schema 17 preserves it. Older browser operations remain unchanged; `browser status` exposes them in
`legacy_actions` as unscoped and unverified. A historical `succeeded` result maps
to `api_reported`, with verification `unsupported`. No task or surface ownership
is invented for that history.

Use extension 0.4.0 with this daemon. C13 adds [challenged browser verification](BROWSER-VERIFICATION.md) and `action reconcile`; the C12 historical contract below remains readable. The public development key and extension ID
are unchanged. Its hello advertises `action_protocol: 1`; older extensions can
continue their existing protocol but cannot receive shared task-bound actions.
Native-host installation remains the separate [browser setup](BROWSER-SETUP.md)
step. C12 tests use synthetic transport and an isolated browser API fixture;
C13 now verifies independent browser postconditions and actual isolated Linux native registration. Browser/compositor association remains an integration gate.

```bash
./bin/heimdall workspace show TASK --data-dir DATA
./bin/heimdall browser status --data-dir DATA
./bin/heimdall action context TASK --data-dir DATA
./bin/heimdall action queue TASK --file REQUEST.json --data-dir DATA
./bin/heimdall action show TASK --id ACTION_ID --data-dir DATA
./bin/heimdall action list TASK --limit 25 --data-dir DATA
./bin/heimdall action history TASK --id ACTION_ID --limit 25 --data-dir DATA
./bin/heimdall action cancel TASK --file CANCEL.json --data-dir DATA
```

An action requires the current reviewed manifest and an explicit browser surface
owned by the exact task. Context includes the task/ancestor revisions, accepted
contract heads, decisions and resource declarations. Those references grant no
filesystem or agent-execution authority. Obtain the current context and create a
new retained request, using a distinct 32-character lowercase hexadecimal ID:

```json
{
  "version": 1,
  "id": "11111111111111111111111111111111",
  "target": "alpha",
  "expected_task_revision": 1,
  "manifest_id": "33333333333333333333333333333333",
  "surface_id": "44444444444444444444444444444444",
  "context_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "browser": {
    "profile": "55555555555555555555555555555555",
    "epoch": "66666666666666666666666666666666",
    "action": "open",
    "url": "https://example.test/"
  }
}
```

Replace every identity and digest with the values from your own task, manifest
and paired profile. The daemon derives the authority reference, attempt ID,
intent digest, typed postcondition and 30-second delivery deadline. Callers cannot
submit execution/verification states or trusted observations. Local CLI authority
is required for queue/cancel; browser and scoped/MCP credentials cannot reach the
action routes. Browser result ingress can only report the exact issued profile,
epoch, action and attempt reference, after current pairing/connection checks.

The queue transaction commits intent, receipt and browser outbox together. A
poll commits `dispatching` and the inventory reference before returning input to
the extension. It requires complete inventory received within five seconds by
this daemon, current pairing/epoch and unchanged task/manifest/context/owned URL.
A reconnect or daemon restart requires a new inventory. C12's age bound is on
daemon receipt; challenge-based fresh capture, monotonic freshness and independent
postconditions belong to C13. A received snapshot cannot make the browser atomic.
The extension separately checks epoch, deadline, current tab ownership and URL
before acting, then persists its journal before any browser side effect.

| Execution | Meaning |
|---|---|
| `queued` | Intent is durable; this attempt has not been delivered. |
| `dispatching` | A delivery receipt committed; external effects may have begun. |
| `api_reported` | The adapter reported success; this does not establish the postcondition. |
| `refused` | A known pre-input refusal or pre-delivery deadline/context failure. |
| `uncertain` | Delivery/result continuity was lost, or a partial failure may have effects. |
| `cancelled` | Cancellation was recorded while the action was still queued. |

Verification has a separate vocabulary: `pending`, `matched`, `not_matched`,
`unknown`, `unsupported`. C12 keeps API success pending, and interrupted effects
unknown. It accepts no caller-authored matched result. C13 adds trusted browser
postconditions. Consequently, a task-bound open remains held after API success;
follow-up navigation/focus/move/close awaits reconciliation. The typed browser
intent already represents these operations and requires the exact task-owned
instance and URL; legacy controls cannot bypass shared ownership or reuse its ID.

New polls never redeliver a dispatched attempt. Exact retries of the original
delivery request are allowed only while it is still dispatching and all current
authority, freshness and deadline guards pass. The extension returns its saved
result for an identical request, or uncertainty for an interrupted/mismatched
journal; it never repeats that uncertain attempt. Queue retries return their
original historical receipt, even after cancellation or restart. Use `show` to
read current state. A new request ID is not a way to retry uncertain input.

Cancellation uses the observed action revision:

```json
{
  "version": 1,
  "id": "77777777777777777777777777777777",
  "target": "alpha",
  "action_id": "11111111111111111111111111111111",
  "expected_revision": 2,
  "reason": "Stop this operation and reconcile any submitted input"
}
```

Queued cancellation releases the surface. In-flight cancellation records the
request and retains uncertainty; it does not claim input stopped or close an
application. Transport replacement, a passed delivery deadline or daemon restart
also records uncertainty after possible delivery. A late API report is retained
without erasing `uncertain_since` or cancellation history. Event order supplies
causal ordering even when the wall clock moves backwards; dispatch refuses a
clock earlier than its intent or inventory.

Unresolved actions serialize their logical surface and owned browser instance.
Up to 128 unresolved actions are allowed globally. Their immutable event history
and current records are retained; the JSON projection currently keeps all action
records. High-frequency WCU capture needs its own measured storage gate. Empty
coordinator scans do not create events or receipts. History pages use indexed
action/event lookup; `--before EVENT_ID` paginates descending event order.

Optional `snapshot_id` must name an available, currently protected point belonging
to the task. An unfinished action then protects that payload even if its manual
pin is released. A known queued cancellation/refusal releases this reference;
uncertainty keeps it protected. Replay performs no external I/O or dispatch.
Database backup includes actions, receipts and referenced snapshot payloads.

Current upgrades from schema 1–16 publish `backups/pre-schema-17-*.db` before
migration. Older binaries refuse marker 17. Roll back only into a fresh directory from the
pre-upgrade backup, following [continuity setup](CONTINUITY-SETUP.md). Do not lower
the marker on an upgraded database. Browser identity, existing grants, operation
IDs and old receipt semantics remain compatible.

This is the shared journal and browser wire contract. C13 supplies verified
browser outcomes; W05 supplies multi-surface operations/resident capacity; W06/W07
add application recovery and verification. WCU and dotprivate dispatch are not
enabled by this slice. No action report completes a task or resumes an agent.
