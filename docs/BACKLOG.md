# Backlog — revision 4

The authoritative design is [handoff r4](design/HANDOFF-heimdall-v1-r4.md), §12 and Appendix B.

The [Heimdall implementation roadmap](HEIMDALL-IMPLEMENTATION-ROADMAP.md) maps the
adopted integration design tasks and acceptance gates into this order. Its S1 design
reconciliation is adopted; surface identity and browser observation events are
implemented at schema 22, while the remaining S1 sensors and UI are pending. Skald is being implemented
in a separate project/session.

| Slice | Scope | Planned schema |
|---|---|---|
| P0 | W07 clock repair, r4 adoption, evaluator environment, YAML check materialization, browser deltas and daily-profile pairing | 20 |
| S2a (R5/A01/A02) | Delivered: scoped computer-use intents, action grant, reports and fresh reconciliation | 21 |
| S1 | Pinned Skald L0 capture; Heimdall hooks/sensors, surfaces, lifecycle and purgeable description evidence; S1b spike | 22 |
| S2b (R6/W08) | Startup readiness, interruption/reboot recovery, optional login restore | no reserved bump |
| S3 (C14–C16) | Braid assignment/continuity retrieval with scoped provenance, typed checkpoint MCP records, optional intent extraction | 23 |
| S4 | Planner, notifier, configuration/preferences and TUI views | 24 |
| S5 (C21) | Mail, version-probed Codex/Desktop adapters reusing pinned capture, packaging and fresh-install replay | 25 |

Delivery order: **P0 → S2a → S1 → S2b → S3 → S4 → S5**. S-numbers are
stable labels, not numeric execution order. S2a (A01/A02) is delivered; S1 now includes browser surface observations at schema 22; browser focus spans and `state --active` are implemented, while other sensors, compositor attention, lifecycle/UI and later slices remain pending. See [S2a implementation](S2A-IMPLEMENTATION.md).

## P0 acceptance

See [P0 verification](P0-VERIFICATION.md) for completed checks and machine-dependent gates.
P0 code is delivered in `5e0c790`; daily-profile pairing and ordinary-day volume
acceptance remain open. The operator explicitly authorized S2a to proceed with
those gates pending; S2a is delivered in `d3e4694`. Keep the deployment gates
visible rather than treating that exception as completed P0 acceptance.

**Agent execution is out of scope for Heimdall.** Agents run in Claude Code,
Codex and herdr; desktop input runs through WCU under its MCP host's approval.
Heimdall supplies accepted context, records scoped actions and reports, and
verifies outcomes from its sensors. No run state machine, dispatch outbox,
leases, fencing, resource locks, execution limits or execution-host adapter will
be added. Checkpoint handoff/resume is built; wake conditions and deduplicated
attention belong to the S4 notifier.

## Historical identifiers

R0 technical baseline; R1/T01–T03/W01 terminal/editor continuity; R2/P01–P04
artifacts, progress, manual preservation and dependencies; R3/W02–W04 observation,
snapshots and previews; R4/C12/C13/W05–W07 actions, browser verification and
application recovery are retained identifiers for delivered work. R5/A01/A02 map
to S2a; R6/W08 maps to S2b; C14–C16 map to S3; C21 maps to S5.
C17–C20 are retired, not pending work. W09 Hyprflow import and automated P03
preservation remain deferred; multi-machine replication and raw-output retention
remain deferred. Conversation ingestion, ranked planning and mail are scheduled
at S1, S4 and S5 respectively. Proposal authoring goes to S4; proposed MCP writes
use typed checkpoint records at S3, not a proposal-write grant. Workflow timing
and daily-use acceptance belong to every slice.

The original C01–C21 definitions remain in
[the archived backlog](history/roadmaps/BACKLOG-baseline-2f11175.md),
[the original plan](history/roadmaps/IMPLEMENTATION-PLAN.md), and
[the Sept 8 roadmap](history/roadmaps/REVISED-ROADMAP-IMPLEMENTATION-PLAN.md).
These documents are historical, not execution instructions.
