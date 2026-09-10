# Backlog — revision 4

The authoritative design is [handoff r4](design/HANDOFF-heimdall-v1-r4.md), §12 and Appendix B.

| Slice | Scope | Planned schema |
|---|---|---|
| P0 | W07 clock repair, r4 adoption, evaluator environment, YAML check materialization, browser deltas and daily-profile pairing | 20 |
| S2a (R5/A01/A02) | Delivered: scoped computer-use intents, action grant, reports and fresh reconciliation | 21 |
| S1 | Observed surfaces, hooks, conversations, herdr sensors, focus spans; S1b adapter spike | 22 |
| S2b (R6/W08) | Startup readiness, interruption/reboot recovery, optional login restore | no reserved bump |
| S3 (C14–C16) | Braid assignment and continuity retrieval, typed checkpoint MCP records, optional intent extraction | 23 |
| S4 | Planner, notifier, configuration/preferences and TUI views | 24 |
| S5 (C21) | Mail, Codex/Desktop adapters, packaging and fresh-install replay | 25 |

Delivery order: **P0 → S2a → S1 → S2b → S3 → S4 → S5**. S-numbers are
stable labels, not numeric execution order. S2a (A01/A02) is delivered; S1 and all later slices remain unstarted. See [S2a implementation](S2A-IMPLEMENTATION.md).

## P0 acceptance

See [P0 verification](P0-VERIFICATION.md) for completed checks and machine-dependent gates.
No later slice starts until P0 is accepted.

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
[the archived backlog](design/history/BACKLOG-baseline-2f11175.md),
[the original plan](design/history/IMPLEMENTATION-PLAN.md), and
[the Sept 8 roadmap](design/history/REVISED-ROADMAP-IMPLEMENTATION-PLAN.md).
These documents are historical, not execution instructions.
