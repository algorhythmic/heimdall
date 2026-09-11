# S1 implementation

Started 2026-09-10 against `d3e4694`, following
[handoff r4](design/HANDOFF-heimdall-v1-r4.md) and the
[implementation roadmap](HEIMDALL-IMPLEMENTATION-ROADMAP.md).
The working tree uses schema 22 and extension 0.6.1. Browser surface observations,
sampled attention spans and browser `state --active` are implemented;
this is partial S1 delivery, not full S1 acceptance or deployment.

## Implemented

`internal/surface.Identify` provides the task-independent observed-content
identity from r4 §5.3: first 12 hex characters of SHA-256 over
`kind|normalized pointer`. It retains the raw pointer separately, derives kinds
for chat, tab, terminal, repository, artifact and mail pointers, and performs
no filesystem or network observations. Existing desired-surface identities and
ownership rules are unchanged.

URL normalization removes fragments, specified tracking parameters and literal
trailing path slashes while preserving meaningful query order, duplicate values
and escaping. Scheme and host are lowercase. Opaque native identifiers retain
punctuation. Malformed pointers and URLs containing credentials are rejected
without echoing the pointer in errors. Repository recognition requires a lexical
`.git` path component; it does not guess repository identity from cwd.
The r4 implementation clarifications record the exact normalization policy.

Paired browser inventories now append version-1 `surface.observed`,
`surface.opened`, `surface.closed` and `surface.changed` records in the same
transaction as their inventory snapshot/delta and command receipt. Schema 22
adds the content catalog (`observed_surfaces`) and epoch-scoped last container
occurrences (`surface_containers`) to the existing CLI `state` output.

- The first inventory proves existence; new tabs after a complete baseline can
  be recorded as opened. Two tabs with equivalent normalized URLs share content
  identity while retaining separate raw locators, titles and container records.
- Navigation closes the old content and opens the new content within the same
  tab and transaction. Tracking-only URL, title and window-placement changes
  retain content identity. Focus/load-only updates do not emit content changes.
- Positive observations use only supplied tabs. Closure uses a complete full
  snapshot, an explicit delta removal or observed navigation. Partial snapshots
  do not refresh inherited tabs or imply closure.
- An inventory URL that cannot be normalized remains accepted by the existing
  browser path, with an explicit `invalid_pointer` gap and no fabricated content
  identity. A subsequent resolvable observation clears that container's gap.
- Reconnect, unpair and source epoch loss do not assert closure. Container
  `present` describes the last observation; consumers must also check pairing,
  current epoch and fresh sensor coverage before claiming current presence.
- The reducer validates authority, source epoch/sequence/times, tab metadata,
  identity and transitions. Short-hash collisions fail rather than merging
  different content. Invalid event batches roll back the inventory, surface
  records, projection and receipt together.

These projections never establish task bindings, desired-surface ownership or
independent action verification. The existing browser action and recovery
consumers continue using their existing inventories and challenged readbacks.
Old inventories replay without retroactively generating surface observations.

## Browser attention and active task reads

The extension now emits sampled `surface.focused` intervals on blur, tab/window
switch or raw-URL navigation, with a two-second minimum. Source timestamps and
`duration_s` describe inventory-resolution attention, not exact input timing.
Reconnect, pause, worker restart, partial/failed collection, unsupported focused
content and challenged readback reset the in-memory tracker. Offline collection
never carries spans. The store refuses malformed durations, duplicate/overlapping
spans and mismatched source/content; its bounded projection keeps the latest span
per profile and the event log retains history. Schema remains 22.

`heimdall state --active --json` requests fresh challenged readback, waits at most
four seconds and selects only exact existing tab ownership under a current task
revision and reviewed manifest. It reports active, unbound, ambiguous, unavailable
(`unknown`) or no browser focus (`none`) with coverage gaps. It adds no action,
changes no task binding and accepts no persisted freshness after runtime restart.
The CLI-authenticated route is `/state?active=1`; Hyprland fallback remains pending.
The extension package list now includes `focus.js`.

## Capture dependency gate

Read-only inspection of the adjacent Skald checkout on 2026-09-10 found:

- `go.mod` declares `module skald`.
- `docs/CONTRACT-V1.md` explicitly describes the public capture packages as
  pre-release, with publishable module path and provider compatibility freeze
  still outstanding.
- The available directory has no discoverable Git repository metadata from
  which to establish an immutable release pin.

Resolved 2026-09-10: `github.com/algorhythmic/skald` is published and pinned at
the immutable tag `v0.1.1` (heimdall `go.mod`). The earlier `v0.1.0` was cut
before `aiTitle` title normalization landed upstream; `v0.1.1` carries it along
with the Devin source and session-page fixes. Only `sessionrecord` and
`sessioncapture` are imported — no Skald daemon, archive DDL or sibling
`replace`. Provider compatibility freeze remains tracked upstream in Skald's
STATUS.md.

## S1.2 direct consumer (2026-09-10)

`internal/session` is the consumer service. `heimdall source add --provider
PROVIDER --root PATH` journals `source.configured` (CLI authority only);
canonical root and the contracted `sessionrecord.Namespace` are derived at
registration and never recomputed later. The daemon poller (5 s) runs
`sessioncapture.Inventory` per active root, `Identify` per candidate, and
journals `source.registered` with a persisted logical stream token before first
capture — relocation reuses identity and updates the locator only.

Each stream reads from its committed checkpoint via `sessioncapture.Read`.
Normalized `title`/`native_recap` records become `description_observed`
(native provenance, native_range coverage); the first record of a new stream
yields `conversation.started` with a cwd→resource-root task binding.
`source.gap` journals coverage gaps, `source.checkpointed` advances only after
its records are journaled, and `source.lost` marks disappearance — never
deletion. Per-event command IDs are content-derived, so interrupted batches
replay into dedupe.

`heimdall init --hooks` merges `heimdall hook` into `~/.claude/settings.json`
SessionStart/SessionEnd preserving existing hook chains (verified against the
installed herdr chain) and configures the Claude projects root. `heimdall hook`
reads a provider payload on stdin; `HandleHook` resolves or registers the
stream and derives source time from the payload timestamp or the transcript's
own records — never an ingestion-time substitute. End requires a known
conversation and counts turns from transcript records.

Verified live: 12 Claude streams registered across five project roots, real
native recaps surfaced through the evidence cache, cwd binding linked sessions
to `heimdall`, `skald` and `wayland-computer-use` tasks, and a SessionStart
hook resumed a live conversation.

## Verification

Used the existing `.tools/go` toolchain and repository module/build caches:

- `go test ./internal/browser ./internal/store ./internal/model -count=1` passes
  lifecycle, normalized identity sharing, atomic navigation, duplicate delivery,
  partial inventory, gaps, epoch loss, rejection/rollback and migration tests.
- The browser lifecycle test starts with the synthetic schema-20 task/workspace/
  action fixture, verifies non-sensor state remains unchanged, and compares
  authoritative state bytes after reopening and replaying without a browser.
- The existing 100-tab/300-change volume regression passes at 668,862 serialized
  event bytes including receipts. This is synthetic evidence, not the P0
  ordinary-day acceptance gate.
- `go test ./... -count=1` passes with fresh execution of all packages. The
  sandbox initially prevented local HTTP/Unix-socket fixture listeners; the
  authorized rerun outside the sandbox passed.
- `go vet ./...` and
  `go build -o /tmp/heimdall-s1-focus ./cmd/heimdall` pass.
- `node --test extension/test/*.test.js` passes, including minimum duration,
  navigation, blur, gap/reset, backwards-clock and failed-focus-read coverage.
- Go focus tests cover atomic span persistence/retry/replay/refusal and a live
  in-process poll/challenge/readback round trip for `state --active`. Selection
  tests cover foreign tabs, stale manifests, competing focus claims and absent
  runtime leases; the CLI test verifies the authenticated active query.
- Compiled `.tools/heimdall-s2a` → new binary migration passes using the synthetic
  external-actions fixture: schema 21 → 22 preserves all 99 events; the old
  binary refuses schema 22; restoring the pre-schema-22 backup lets the old
  binary open schema 21 with the same events. Temporary evidence is in
  `/tmp/heimdall-s1-migration-fqg31v2k/result.json` (not a permanent fixture).
- The earlier identity increment passed fixed independent hash vectors and
  approximately 506,000 fuzz executions. Its hostless-root regression input is
  retained under `internal/surface/testdata/fuzz`.

The isolated Chromium worker acceptance passes with the compiled focus binary:
real tab APIs emitted a focus span; `state --active` returned the challenged
unbound tab; pause/resume, offline buffering and daemon restart/reconnect passed.
Run with `PLAYWRIGHT_BROWSERS_PATH="$PWD/.tools/playwright"` and
`HEIMDALL_BIN=/tmp/heimdall-s1-focus node scripts/worker-smoke.cjs`. The native-host
discovery step is a test shim, not OS registry discovery. The fixture uses a
same-window tab switch because headless window activation did not change Chrome
focus. No daily-profile acceptance or deployment was performed. The P0 daily-profile
pairing and ordinary-day volume gates remain open. Remaining S1 work includes
capture integration, conversation/agent projections, purgeable description
bytes, compositor attention, Herdr observations, generic sensor health events, artifact
occurrences, capture popup, TUI panels and full S1.5 acceptance. Schema 22 is the
S1 migration marker; its presence does not mean these remaining features exist.

## S1 browser review clarifications (2026-09-10)

URL identity intentionally normalizes a bare trailing `?` (an empty query) away,
so it is not a distinct content identity from the same URL without a query.

The observed-surface reducer currently validates a positive observation against
the merged browser projection, not the exact tab IDs supplied by a partial
inventory. The producer emits positive observations only for supplied tabs. A
future increment may retain supplied tab IDs by sequence to enforce that boundary
inside replay.

The store now refuses a focus span if its container was last observed under a
different browser connection, and refuses chronological overlap with the previous
retained span for the profile regardless of epoch or connection. This replaces
the earlier narrower statement that overlap refusal was connection-scoped.

## TUI observed surfaces and attention (2026-09-10)

The selected-target context now renders browser observations only through exact
recorded open-tab ownership under the current reviewed task and manifest. It
shows bounded escaped pointers, pointer gaps, tier 0/1, last focus and current
versus historical profile/epoch state. A global attention line shows the latest
span per paired browser profile. `g` performs the existing authenticated
`/state?active=1` read on demand and renders its verdict, locator, target, gaps
and age; it does not poll or create a binding. Compositor spans remain a later
projection input for this panel.

## Hyprland compositor attention and active fallback (2026-09-10)

The existing Hyprland observer now tracks buffered socket2 focus transitions and
persists version-1 compositor surface/workspace attention spans after a two
second debounce. Source epoch loss, socket EOF, failed capture and reconnect
discard open spans. The bounded projection retains the latest span per source epoch
and per native window within the current epoch (at most 256); replay verifies
source-head/epoch provenance, native identity epoch, monotonic sequence and no
overlap from the span payload alone. No compositor window inventory is appended
to the event log. Paired connected browser windows
are excluded to keep tab-level browser attention authoritative. `state --active`
keeps its four-second total budget, prefers fresh tab focus, and falls back to a
fresh compositor read only for exact current viewport ownership under a reviewed
manifest. The fallback reports sensor coverage gaps and never binds a task.

## Conversation lifecycle and purgeable descriptions (2026-09-10)

S1.3 now delivers pure conversation types, source identity validation, schema-22
`conversation.started`, `conversation.ended` and `conversation.description_observed`
v1 reducers, store ingestion APIs and CLI-authenticated `conversations [TARGET]
[--json]`. Matching source/native IDs resume the same 32-hex Heimdall ID. Explicit
bindings require an existing current task revision. Inactivity is display-only.

Description events/receipts/state retain metadata and both digests, never prose.
The purgeable SQL cache stores at most 4,096 normalized UTF-8 bytes per digest,
with scope-checked associations, permanent withdrawal metadata and a 32-reference
projection history window. Missing cache tables are recreated at open without a
schema bump. Replay never requires text; explicit bounded hydration verifies the
source revision, both digests and retention metadata without changing events/state.
Withdrawal races and shared digests recheck current association availability.

Synthetic event/state and identity fixtures cover deterministic replay, hydration,
reopen, withdrawal, gaps, scope isolation, truncation, strict provenance, retries,
resume, task validation, transaction rollback, CLI auth and terminal escaping.
The amendment authority regression compares all non-conversation replay state
including populated contracts, decisions, checkpoint heads and verification records,
then exercises another authorized checkpoint. See `NOTES-desc.md` for observed
verification results and [the guide](guides/CONVERSATIONS.md) for retention limits.

S1.1 remains blocked on a released Skald pin. This implementation depends only on
Heimdall and the standard library; v1 identity encoding is implemented from the
published specification. Hooks, transcript capture/parsing, configured resolvers,
agent events and binding-order inference remain future work. No extension edits.
