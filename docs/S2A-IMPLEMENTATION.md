# S2a — scoped computer-use records (A01/A02)

Delivered at schema 21 after P0 commit `5e0c790`. Scope is r4 §8.2,
§9.5 and §12. The operator authorized this slice while P0's daily-profile and
ordinary-day measurement gates remain pending. S1 (schema 22), S2b/W08 and later
slices remain unstarted; retired C17–C20 execution orchestration remains retired.

## Register, act, report, observe

Issue an expiring credential using `grant issue --action TARGET` (the spelling
`grant issue TARGET --action` also works). As with read grants, explicitly grant
mandatory ancestor and resource visibility needed for context. Action-write and
checkpoint-write cannot be combined. Only local CLI authority issues/revokes
grants; an action credential cannot change bindings, accept contracts, write
checkpoints, ratify completion or invoke an input dispatcher.

Use `heimdall_context` to inspect declared owned windows. Mere focus never creates
ownership: the task must have a current accepted workspace manifest and explicit
viewport binding to an exact window in the selected desktop source/epoch.
Context/resume can return fresh address/title/workspace metadata when available.

The two new MCP tools use the scoped loopback routes `POST /client/intent` and
`POST /client/report`. Each logical request has `version: 1`, its own 32-hex
`id`, and the exact `target`; retain identical arguments and ID for retries.

Example intent body (replace the example ID and target):

```json
{
  "version": 1,
  "id": "0123456789abcdef0123456789abcdef",
  "target": "example-task",
  "purpose": "Move focus through two controls in the owned fixture",
  "steps": 2,
  "expected": {"kind": "window_focused"}
}
```

An optional `surface_id` selects a declared owned window; otherwise the daemon
requires the focused window to be owned. Registration performs fresh readback
and returns the immutable intent plus exact runtime window identity/title and
workspace for the external harness. Intent v6 pins task revision, manifest,
context, source, binding and window. The lifetime is at most 120 seconds and
cannot outlive the grant. There are 1–64 steps, defaulting to one. Aliased
concurrent window intents are refused.

The agent passes the returned target to WCU and follows WCU's own approval,
freshness and input guards. Heimdall initiates no input. Report each step with
`intent_id`, sequential `step` and `outcome` (`succeeded`, `failed` or
`uncertain`). Optional `wcu_request_id` and 64-hex `metrics_digest` retain
correlation. Reports are bounded claims, never verification. An external intent
has execution `external`; the last report settles it to `api_reported` (or
`uncertain` when so reported).

Independent readback produces the verification. Native predicates are
`window_exists`, `window_absent`, `window_focused` and `window_workspace`
(with `workspace`). Browser predicates additionally include `owned_tab_url`
(with `url`, `redirect_policy: exact`, and optional `load_condition:
committed|complete`), `owned_tab_absent` and `owned_tab_focused`.

Browser registration requires the existing proved native-container association,
paired profile/epoch and an active tab owned by the same task/surface. Foreign
tabs in that container, or filtered tabs whose container cannot be established,
refuse admission. Registration and reconciliation use challenged extension
readback with its live monotonic freshness lease, alongside fresh Hyprland
observation. URL, load, absence and focus are checked against the pinned tab;
outer-window focus alone cannot verify tab focus. Native browser titles/classes
are removed from journaled observations. Context metadata requires the current
owned active tab/title to agree. No new pairing mechanism or browser input
dispatcher is introduced.

## Cancellation and evidence

`heimdall action cancel INTENT_ID` records cancellation using local CLI
authority. Supply the global `--request-id` to retain exact retry identity.
The existing `action cancel TASK --file REQUEST.json` revision-pinned form
remains available. Revocation and cancellation refuse later reports, including
cached successful report retries after revocation. A refused known-intent report
has a durable refusal receipt returned with HTTP 403. Unknown credentials and
foreign intent IDs receive an opaque denial.

Missing final reports expire or recover from restart as `uncertain`. Finalized
execution reports survive ordinary grant expiry. Cancellation/revocation still
permit independent reconciliation. The daemon retries unresolved readback at
most eight times; CLI `action reconcile` can request another observation.
These operations never stop, dispatch or repeat external input.

Checkpoint v4 adds optional `actions` (request v3); each reference must identify
an independently matched, current action on the exact target and revision.
A separate checkpoint-write credential is required. Existing artifact/resource
authority is unchanged.

A trusted completion definition can cite a chosen intent ID:

```yaml
checks:
  - id: focused
    kind: action.verified
    action_id: 0123456789abcdef0123456789abcdef
```

Choose the ID before registration so the task definition remains current.
The check evaluates the registered intent's postcondition and cites
`action:INTENT_ID:OBSERVATION_ID`, not a report. Existing completion proposals
and human ratification remain the completion path; review the intent's expected
predicate as part of that decision. Acceptance independently reobserves native
state under the writer and rechecks fresh challenged browser state prepared
before entering it. Focus/URL drift refuses completion. Checkpoint citations
alone do not complete a task.

## Optional WCU corroboration

Before daemon start, an operator may create `DATA_DIR/wcu-observer.json`:

```json
{
  "argv": ["/usr/bin/python3", "/absolute/pinned-bundle/wayland-desktop-observer/scripts/observer_server.py"],
  "revision": "4c709b707e0dc26bffbbf915d79461969f03b44fb6180549fc26d8806c7d6bb5",
  "reports_dir": "/absolute/private/request-exports",
  "trace_dir": "/absolute/private/wcu-traces"
}
```

The revision is an operator-supplied provenance pin, not automatic executable
hash verification. Omit optional directories when unavailable. If `trace_dir`
is omitted, the adapter uses `WAYLAND_CU_TRACE_DIR` when set. Missing observer
configuration disables corroboration; invalid configuration refuses startup.
No machine configuration is installed automatically.

Each reconciliation starts a short-lived read-only MCP Desktop Observer and
calls only `observe` with the exact address, `images: false`, metadata and
accessibility channels, and `max_age_ms: 0`. The process has a 1.5-second
deadline and a 2-MiB response limit; Linux process groups are killed/reaped.
Only source, pinned/runtime revision, freshness, partial status and digest are
retained. Unavailable or incomplete accessibility is explicit, not treated as
complete evidence. Frames and raw accessibility text are discarded.

Request-local results can be explicitly exported to
`reports_dir/WCU_REQUEST_ID.json`, with matching `request_id`,
`target_window.address` (and PID when available), and
`sequence.steps_total/steps_completed`. Exports are bounded to 256 KiB,
regular files beneath the selected directory; escaping symlinks are refused.
These caller-supplied exports are corroboration, not authenticated provider
claims. A target mismatch sets reconcile reason `target_mismatch` but never
changes the binding or observation-derived verification.

The pinned WCU schema-1 JSONL traces correlate by exact `trace_id` matching
the report's `wcu_request_id`. Bounded scanning retains only counts, total
duration, submitted count and a digest. “Submitted” is not application
acceptance. Traces contain no usable binding identity; only the request-local
export can corroborate that identity. No raw trace payload is journaled.
Unavailable, uncorrelated, truncated or out-of-budget traces add no authority.

## Acceptance evidence

- Full Go suite and targeted race checks cover ordered/exact-retry reports,
  wrong-task and unowned-window refusals, revoked authority inside the writer,
  durable refusal receipts, expiry/restart without repeated input, cancellation
  by intent ID, current checkpoint references and deterministic replay.
- Browser tests exercise the real challenge/readback protocol with synthetic
  transports: foreign-tab admission refusal, exact owned URL verification,
  lost live lease, URL drift, sanitized native metadata and replay.
- MCP acceptance runs the official SDK through the stdio bridge's in-memory
  transport and a real scoped HTTP server: registration/retry, sequential
  reports, independent reconciliation and revocation refusal.
- Completion acceptance proves reports alone remain unknown, proposals cite the
  independent observation, drift refuses ratification and fresh matching
  readback permits it. Observer subprocess tests cover deadlines, partial
  coverage, raw-content exclusion, bounded exports and trace correlation.
- Real native action `9ef1993380da914b37b7de802606e39f` used two guarded WCU
  Tab inputs and independently verified focus as matched. This earlier run used
  published bundle `fb4ac4da6ab03192d99ca4b8e26963a3eb60e9f8e5748350adbc0badad1957ab`.
- The September 10 run, intent `8b90a7df37b391a019159dfb1aaea728`, reported
  two successful guarded WCU inputs but independently verified `not_matched`
  after focus moved elsewhere. Fresh/partial WCU corroboration and two report
  correlations were retained, and replay matched. The export ID
  `s2a-live-20260910` was caller-chosen, not a provider trace ID. No input was
  repeated. Local evidence is ignored under `.tools/s2a-native-final/`.
- Schema-20 fixture acceptance preserves all 99 historical events, receipts and
  replay state with a consistent pre-schema-21 backup. An actual P0 binary
  refuses the upgraded database; restoring that backup into a fresh directory
  works with the P0 binary at marker 20. No marker was lowered for rollback.

The September 10 runtime was WCU contract `wcu-tools-4`, context schema 1
(`wcu-context-1`), published bundle
`4c709b707e0dc26bffbbf915d79461969f03b44fb6180549fc26d8806c7d6bb5`.
Tool-contract SHA-256:
`5485fba8a8d7df27ff260677907852a158b6e51add8a2cb8e4d6bfa106afddd6`.
Loaded skill SHA-256:
`8062f66aacb1524b49229bc360f3851e27ca459b0a961d901a9a2472bfb6b61b`.
Observer version:
`2b5401dff8eb14d11e49e7c2d214d9e8c0dd90c0c099e3171b58b80fd25d448a`.

P0's actual daily-browser registration/pairing and ordinary-day inventory-volume
measurement remain deployment gates. No user browser/profile or machine setting
was changed during S2a. Browser S2a acceptance uses isolated synthetic protocol
fixtures, not an assertion that the daily profile is paired.
