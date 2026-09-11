# Heimdall implementation roadmap

Reviewed 2026-09-10 against Heimdall `d3e4694`, schema 21, and the separate
Skald checkout's implementation plan, status and v1 contract. This is a planning
consolidation with the reviewed amendment adopted into
[handoff r4](design/HANDOFF-heimdall-v1-r4.md) on 2026-09-10. Design adoption
does not implement S1. Skald implementation continues in its separate session;
shared planning requirements are synchronized without changing its code.

## Current state and P0 verdict

**P0's code is implemented; its full daily-use acceptance is incomplete.**
Commit `5e0c790` contains the P0 implementation. S2a subsequently landed in
`d3e4694`; the current schema is 21. The recorded operator exception allowed S2a
to proceed with P0's two machine-dependent gates still open. It does not establish
that those gates passed or grant a new deployment authorization in this review.

| P0 requirement | Evidence and status |
| --- | --- |
| W07 observation clock repair | Implemented in `internal/workspace/recovery.go` and `internal/browser/workspace_readback.go`; delayed-time and lease tests pass. |
| r4 adoption and roadmap reconciliation | Implemented in the handoff, README, STATUS and BACKLOG; historical plans retained. The stale blanket P0 gate in BACKLOG is corrected alongside this review. |
| Evaluator environment allowlist and named environment revalidation | Implemented in `internal/checks/evaluate.go`; exclusion and named-value invalidation tests pass. |
| Atomic YAML check materialization | Implemented in `internal/core/materialize.go`; replay, override, ancestor/step scope and rollback tests pass. Saving does not execute checks. |
| Browser deltas and replayable snapshots | Implemented in `internal/browser/service.go` and `extension/worker.js`; replay/boundary/volume and extension delta tests pass. Surface identities and focus spans remain S1. |
| Real YAML-only `test.exit` evidence | Recorded as passing in [P0 verification](P0-VERIFICATION.md), with retained fixture evidence and restart/replay results. Not rerun in this document review. |
| Daily-profile native-host registration and pairing | Still pending in the implementation evidence; isolated Chromium acceptance does not satisfy this gate. |
| Ordinary browsing day below 1 MB | Still pending. The recorded approximately 452 KB synthetic workload is not a measured ordinary day. |

The §13 `r4-p0` tag is also absent from the local tag list. Treat it as an open
release bookkeeping item, not evidence that the code is absent; check release
policy and acceptance before creating it. This review did not query remote tags.

Fresh verification for this review: selected P0 regression tests in
`internal/core`, `internal/checks`, `internal/browser` and `internal/workspace`
passed with `-count=1`; `node --test extension/test/inventory-delta.test.js`
passed. This was not a fresh full-suite, live-browser or machine deployment run.

## Ordered Heimdall work

Keep **P0 → S2a (A01/A02) → S1 (sensors) → S2b (W08) → S3 → S4 → S5**.
Slice numbers are stable labels. S2a uses existing sensors; W08 follows the new
sensors so startup hardening does not delay hook ingestion. S3 consumes S1's
observations and descriptions; S4 consumes that context for plans and attention.
Skald's earlier Codex coverage does not advance Heimdall's S5 adapters.

| Order | State / schema | Consolidated scope and exit evidence |
| --- | --- | --- |
| P0 | Code delivered / 20 | Close daily-browser pairing and ordinary-day volume gates using the procedure in P0 verification. Record actual profile, interval, bytes including receipts, tab/change counts, reconnects and coverage gaps. Retain these as visible deployment work. |
| S2a — A01/A02 | Delivered / 21 | Preserve action grants, durable intent/report/cancel, WCU corroboration, owned-window context, fresh sensor reconciliation and action references. Acceptance is in [S2a implementation](S2A-IMPLEMENTATION.md); no Skald dependency or redesign. |
| S1 — sensors | In progress: browser surfaces/focus / 22 | Pin the released Skald L0 capture dependency (started 2026-09-10); implement direct Claude hooks/transcript ingestion, observed surfaces, Herdr observations, browser/Hyprland focus spans, capture popup, sensor coverage, artifact occurrences and conversation/surface TUI panels. S1b remains a capability spike followed by one adapter only if it passes. Details below. |
| S2b — W08 | Unstarted / no reserved bump | Desktop/browser/Herdr startup readiness, bounded waits, manual override, duplicate-start suppression, optional policy-bound login restore, startup packaging/uninstall and `doctor --startup`. Demonstrate daemon interruption, compositor restart, reboot and controlled VM power loss with consistent recovery reports and uncertain unsettled actions. A process kill does not satisfy the VM gate. |
| S3 — C14–C16 | Unstarted / planned 23 | Braid assignment and continuity retrieval, label export/classification, scoped typed checkpoint MCP capture/observe and optional configured intent extraction. Add native-description nodes with distinct provenance, revision-safe retrieval and independent operation. Record the real-label F3 outcome; claims remain separate from evidence. |
| S4 | Unstarted / planned 24 | Configuration/preferences, planner and proposal authoring/ratification, notifier delivery/deduplication, needs-you/plan/drift TUI and optional contrib views. Deterministic plan golden under `--now`, one drift notification/sound per stale task/day and TUI update within 1 s. Skald groups/recaps never change task state or authorize execution. |
| S5 — C21 | Unstarted / planned 25 | Maildir then IMAP with coverage/checks, version-probed Codex/Desktop adapters using shared parsing, preserved notify/hook chains, systemd packaging and fresh-install export/replay. Verify each advertised source capability, real mail check within 60 s and end-to-end proposal/ratification fixture. |

S2b owns startup/recovery packaging; S5 owns complete installation and provider
coverage. Preserve that distinction rather than counting the same acceptance
twice. No additional schema number is reserved for Skald; add the actual migration
row with its owning implementation.

## S1 task breakdown and prerequisites

- [x] **S1.0 — Reconcile the design before coding its conversation pipeline.**
  Adopted the [Skald amendment](design/SKALD-HEIMDALL-AMENDMENT.md) into r4 on
  2026-09-10, with digest-only description events, purgeable evidence bytes and
  separate replay/hydration. Native lifecycle, inactivity, native descriptions
  and S3 optional extraction are distinct. Design adoption is complete; the surface identity and browser observation increments are now implemented (see S1.4).
- [x] **S1.1 — Freeze and pin the reusable capture boundary.** Coordinate with
  Skald L0 on v1 encoding, namespace/root aliases, persistent logical stream
  tokens, identity vectors, provider compatibility and sanitized fork/alias
  fixtures. Record the dependency version and usable module path; do not ship
  against a mutable sibling checkout. Consume `sessionrecord`/`sessioncapture`
  without Skald's daemon or archive DDL. L0 started in Skald on
  2026-09-10; publish from that module and pin in S1. The first-repository fallback
  remains available if ownership must move; S1 does not wait for Skald L1–L6.
  Local dependency inspection on 2026-09-10 confirms `module skald`; its v1
  contract still calls the capture foundation pre-release with module publication
  and provider compatibility freeze outstanding. No immutable dependency can yet
  be pinned from that checkout; no sibling `replace` was added.
- [x] **S1.2 — Implement Heimdall's direct consumer.** Configure sources and
  `/hook`/`init --hooks`; preserve provider/Herdr hook chains. Normalize hooks and
  transcript records with the same parser. Apply existing binding rules after
  parsing and map external keys to existing Heimdall IDs. Persist selected
  records, gaps and source checkpoints atomically through event transactions.
  Do not invent conversations for project notes with no proven conversation.
- [ ] **S1.3 — Add digest-only description events and purgeable evidence.**
  Register versioned `conversation.description_observed` and replay/migration
  handling. Events retain source/record/revision references, description digest,
  kind/provenance, versions, times, coverage and availability, with no text.
  Keep allowlisted normalized text in a projection-side evidence table keyed by
  digest and authorized through source/scope associations. Cap it at 4,096 UTF-8
  bytes; distinguish original and retained-byte digests and mark truncation.
  Populate at ingest; hydrate separately after replay only from permitted sources
  or an optional feed with verified digests. Absent bytes are diagnostic coverage
  gaps. Withdrawal purges text/retrieval copies and blocks rehydration of withdrawn
  associations. No text in events, receipts or serialized authoritative state.
- [x] **S1.4 — Implement lifecycle and the remaining sensor surface.** Native
  end differs from idle/inactive; source disappearance is a coverage gap and a
  resumed native ID stays the same conversation. Add surfaces, Herdr observations,
  tab/window focus, attention, capture UI and exact-digest artifact occurrence
  relationships under r4's existing binding and observation rules. Heimdall’s panel
  shows bound conversations, lifecycle and current description or gap; Skald’s
  window shows the archive.
  Started 2026-09-10: `internal/surface.Identify` implements the independent
  r4 §5.3 identity/normalization boundary with fixed hash vectors and fuzz
  coverage. The next increment adds atomic browser surface events and the
  schema-22 catalog/container projection, including replay, rollback, gaps and
  navigation. Extension 0.6.1 adds sampled focus spans; `state --active` now
  selects tab-level focus through bounded fresh readback and existing ownership.
  Completed 2026-09-10: compositor attention, `sensor.degraded`/`recovered`
  health transitions, `artifact.origin_observed` exact-digest transfers plus
  derived `same_content` peers, the extension capture popup (`capture` bridge
  message), `source activate|deactivate` lifecycle, and conversation/sensor
  lines in the TUI context pane. See [S1 implementation](S1-IMPLEMENTATION.md).
- [x] **S1.5 — Demonstrate acceptance.** With Skald absent, consume sanitized
  lifecycle/recap fixtures and a verified local Claude transcript. Real
  `agent.blocked` appears within 2 s and clears on next tool use; explicit task
  binding and end source references survive replay; `state --active` identifies
  tab-level focus. Replay remains deterministic. Exercise duplicate delivery,
  partial writes, rotation/rewrites, repeated text, missing IDs, aliases/forks,
  unknown versions, withdrawn/oversized descriptions and same-ID resume.
  Verified 2026-09-10: replay determinism, recap isolation, partial-write
  checkpoint hold, hook resume/end/unknown-refusal
  (`internal/session/service_test.go`), blocked-observe/clear and
  sensor health (`internal/workspace/herdr_test.go`), origin rejection
  matrix (`internal/store/origin_test.go`). Provider edge cases
  (aliases/forks/missing IDs/unknown versions) remain Skald's
  sessioncapture conformance scope upstream. Description digest mismatch,
  purge, withdrawal/replay and cross-scope isolation were covered by the
  S1.3 conversation store tests.

## S3 and S5 integration additions

- [ ] **S3.1:** Use an independently configured Heimdall Braid dataset and pin
  its actual current contract. Publish only permitted retained records and
  accepted relationships; distinguish native claims from decisions/checkpoints.
  Every MCP response requires producer/authority/scope/revision provenance, with
  explicit unknowns and per-item provenance for mixed results; test its omission.
- [ ] **S3.2:** Enforce caller scope before traversal and expansion; retain
  source and index revisions/digests; test exact/lexical reads, revision conflicts,
  withdrawals and scope narrowing. Braid failure must preserve mandatory context,
  task commands, checkpoints and direct collection. Optional intent extraction
  requires an explicitly configured provider and does not accept a decision.
- [ ] **S5.1:** Reuse available Codex/Desktop parsers while independently probing
  installed versions and accessible sources. Unsupported/opaque content and
  ephemeral recaps remain explicit capability gaps. Preserve hook/notify chains
  and verify clean-install/restart behavior; Skald's capability claims are not
  Heimdall acceptance evidence.

## Separate Skald work and optional follow-ons

L0 started 2026-09-10. L1–L2 are the standalone product; L3 adds valuable retrieval.
L4–L6 retain their existing optional integration/capability gates. Skald starts
Braid at lexical weight 1 and other retrievers at zero; fused retrieval needs
measured real-label evidence before any quality claim.

The sibling [Skald status](../../skald/STATUS.md) now reports the **first L1
archive/daemon slice implemented**, including explicit sources, persisted root
aliases/stream tokens, bounded reads and synthetic backup/fresh restore without
native files. It does not claim full release acceptance. Provider fork/alias
fixtures, installed-provider verification, a public module release, large-file
performance/storage and extended recovery gates remain open. The stronger L1
independent-destination and backup-failure requirements added here still need
recorded acceptance evidence. These implementation/test claims belong to the
separate session and were not rerun in this design review.

| Skald milestone / owner | Work and relationship to Heimdall |
| --- | --- |
| L0 — Skald, shared contract coordination | Finish the freeze/release gates above; this is S1's narrow code-reuse dependency. |
| L1 — Skald | Sole-writer archive daemon, registered sources, backfill/tailing, revisions/gaps/checkpoints, capacity, CLI reads and SQLite consistent backups to a configured, recorded non-Git destination outside dotprivate. L1 exit requires fresh-directory restore from that destination with native files absent, digest/foreign-key/checkpoint checks and interruption/failure evidence. Independent of Heimdall. |
| L2 — Skald | Session TUI, description selection/history, project/groups, health and optional Herdr activation; verify resizing, Unicode and stale locators. Independent of Heimdall S1. |
| L3 — Skald | Its own Braid publication, exact/lexical retrieval, bounded expansion/export and read-only MCP, with scope/revision/deletion/failure tests. Does not implement Heimdall S3. |
| L4 — Both consumers, separate implementations | Prove released capture reuse in Heimdall S1 and both independent-service scenarios. Optional authenticated Heimdall read enrichment retains instance/target/revision/fetch time. No task-status synchronization. L0/L1 packages may be reused before L4 completes. |
| L5 — Skald | Additional desktop/web capability spikes, imports, installation/uninstall and recovery. Does not move Heimdall's S5 coverage earlier. |
| L6 — Skald; dotprivate owns its interface | Portable selected-session/artifact exports/imports, scoped project-document reads and receipts. Can follow L1; Braid publication uses L3. Full archive backup stays L1. Git automation waits for a generic selected-files interface. |

Optional work has no reserved Heimdall slice or schema and must not become a
prerequisite for the main order:

- [ ] **Normalized feed, after direct capture works:** scoped snapshot plus tail
  at one boundary, original record identities, revisions/withdrawals/gaps and
  raw-body export disabled for Heimdall. Test disconnect, expired cursor,
  snapshot/tail races and transport switching without duplicates or silent gaps.
  Native checkpoints and archive cursors are distinct. Automatic failover is
  optional; an inaccessible native path is not a functioning fallback.
- [ ] **Selected external retrieval:** explicitly configured Skald or dotprivate
  sources, file/version citations for notes, original identities for exports,
  caller scope and native/export deduplication. Do not mirror a whole archive or
  treat a Git commit, recap or group as accepted evidence or a task assignment.
- [ ] **Automated preservation:** pin the single
  [dotprivate-owned selected-files contract](../../dotprivate/docs/SELECTED-FILES-CONTRACT.md)
  after implementation and release, then verify each consumer’s authorized
  request/receipt integration. The contract owns checkout serialization, selection
  isolation and uncertain-push reconciliation. Keep Heimdall’s manual workflow
  until this gate passes; dotprivate is not the full-archive backup destination.

Neither application opens or migrates the other's database. Neither Skald,
Braid nor dotprivate availability gates Heimdall's core local operations.
Skald's own grouping/capture/retrieval must also work without Heimdall.

## Source reconciliation notes

The Skald consumer amendment is adopted into r4; its implementation remains
planned. Skald owns its live plan, status and v1 contract in the adjacent checkout.
The [reviewed plan snapshot](history/references/SKALD-IMPLEMENTATION-PLAN-2026-09-10.md)
is archived for provenance, with an [owner pointer](design/SKALD-IMPLEMENTATION-PLAN.md)
at its former path. Sibling links require adjacent checkouts. Implementation and
test claims above are dated snapshots from the separate session, not a second
live status page for Skald.
