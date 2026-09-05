# Continuity implementation backlog

Date: 2026-09-04. **Development build 0.5.0 delivers scoped checkpoint writes and the MCP stdio adapter.** The controlling design is [IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md). Dependencies describe build order, not authorization to run agents or install integrations.

## Current progress

| Items | State | Delivered / remaining |
|---|---|---|
| C01 | Initial slice complete | Request schemas/fixtures, golden events for old/new contracts, decisions, resources, CLI/client checkpoints and read/write grants; schema-5 migration, replay and actual 0.4.0 upgrade/backup/restore pass without elevating read credentials. Future record types need their own fixtures. |
| C02 | Policy documented | [Capability ledger](CAPABILITY-LEDGER.md) ties CLI/browser boundaries to tests and defines required principal/grant policy. Actual scoped credentials and grants belong to C06. |
| C03 | Partial | Accepted contracts/decisions, supersession, revision checks, canonical file/tree registration and bounded observations work. Version-2 contracts freeze explicitly supplied, reviewed resource IDs; changed scope requires reacceptance. Proposed/rejected decision workflow and Git/source identity remain open. |
| C04 | Initial slice complete | Immutable append/list/get-by-ID, explicit previous head, retry, competing-write, restart and replay checks pass. Run/evidence links will arrive with their consuming slices. |
| C05 | Initial slice complete | CLI context includes mandatory task/ancestor contracts, decisions, checkpoint and resource drift without retrieval; small budget fails explicitly. Estimate is UTF-8 bytes/4; Git identity and exact tokenizer accounting are not claimed. |
| C06 | Initial slice complete | Scoped reads plus explicit checkpoint-write grants. Authority is checked inside the writer transaction before dedupe and commit; records carry authenticated grant/author provenance. Tests cover read-only denial, cross-target/cross-grant denial, expiry, revoked retries, conflict, rollback and replay. Broader machine mutations are not delegated. |
| C07 | Initial slice complete | Official Go SDK v1.7.0 stdio adapter; task/context/history/checkpoint tools, structured errors, stable request IDs and daemon restart rediscovery. Official SDK client uses 2026-07-28; compiled stdio smoke uses 2025-11-25. User-host registration and Linux runtime acceptance remain open. |
| C08–C21 | Planned | Evidence, GUI, verified actions, retrieval and execution-host slices remain unimplemented. |

Next patch series: C08/C09 evidence and completion revalidation, with the remaining C03 decision/Git identity work. Current usage is in [MCP-SETUP.md](MCP-SETUP.md), [SCOPED-ACCESS.md](SCOPED-ACCESS.md) and [CONTINUITY-SETUP.md](CONTINUITY-SETUP.md).

## Ordered deliverables

| ID | Slice / improvement | Deliverable | Depends on | Completion evidence |
|---|---|---|---|---|
| C01 | S0 / shared | Schema and event fixtures for contract, resource, checkpoint; migration/backup design | Existing 0.2.0 | Reviewed schema, legacy marker-2 fixture, replay and upgrade checks |
| C02 | S0 / shared | Principal/grant policy and capability ledger | Existing 0.2.0 | Threat/role cases tied to current CLI/browser routes; host capabilities explicitly unknown until tested |
| C03 | S1 / #1 | Contract and accepted decision records; resource registration | C01, C02 | Revision-conflict, canonical-root, accepted/proposed and supersession cases |
| C04 | S1 / #1 | Checkpoint append/list/get with head precondition | C03 | Duplicate command, stale revision, competing update, restart and replay cases |
| C05 | S1 / #1 | Minimal context assembly and checkpoint CLI | C04 | Mandatory context present without retrieval; changed worktree; explicit small-budget error |
| C06 | S2 / #2 | Scoped daemon API and per-client credentials | C02, C05 | Cross-project read/write denial, revocation and forged-authority negatives |
| C07 | S2 / #2 | Go MCP stdio adapter: reads then checkpoint/progress writes | C06 | Real client handshake, structured errors, second-writer prevention, retry and reconnect |
| C08 | S3 / #4 | Evidence schema and artifact/repo/test evaluator registry | C03, C06 | Exact inputs/output provenance; forged result, wrong repo, partial coverage and changed-input negatives |
| C09 | S3 / #4 | Evidence invalidation and completion proposal revalidation | C08 | Criteria changes supersede pending proposal; old accepted completion history retained; no replayed test execution |
| C10 | S4 / #7 | Authenticated local UI session and scoped event feed | C06 | Single-use bootstrap, CSRF/Origin/Host, cursor-scope and session-expiry checks |
| C11 | S4 / #7 | Task/checkpoint/evidence screens and explicit review controls | C05, C09, C10 | Real-browser keyboard/layout and stale-state acceptance; no secret-bearing URLs |
| C12 | S5 / #3 | Shared action/verification schema and browser wire types | C01, C06 | Legacy success stays unverified; Go/TypeScript conformance; manifest identity preserved |
| C13 | S5 / #3 | Browser freshness, postconditions and recovery | C12 | Actual native registration plus open/navigation/focus/move/close race and crash tests |
| C14 | S6 / #6 | Resource/decision/checkpoint mapping and isolated Braid supervisor | C03, C05, C06 | Actual subprocess contract, complete generation publication, failure isolation |
| C15 | S6 / #6 | Scoped context search, revocation and mandatory/optional context budget | C14, C08 | Wrong-scope exclusion before retrieval, stale/index-pending, purge/revocation and provider-off checks |
| C16 | S6 / #6 | Held-out retrieval evaluation and channel comparisons | C15 | Consented labeled dataset, lineage split, baseline metrics and documented chosen weights |
| C17 | S7 / #5 | Run state machine, dispatch outbox, leases, fencing and resource locks | C04, C06, C09, C13 | Fake-host fault matrix; no launch during replay; uncertain dispatch cannot duplicate work |
| C18 | S7 / #5 | Manual handoff/resume, limits, wake conditions and cancellation | C17 | Resume from checkpoint with stale-state checks; revocation, retry ceiling, host-offline and cancellation races |
| C19 | S7 / #7 | Run controls, recovery view and deduplicated needs-you notices | C11, C18 | Confirmed versus requested state visible; repeated observations do not repeat notifications |
| C20 | S8 / #5 | One verified real execution-host adapter | C18 + actual host capability contract | Launch/attach-or-reconcile/status/cancel acceptance with installed host; publish supported limitations |
| C21 | S8 / all | Full interrupted-task acceptance and release package | C07, C09, C13, C15, C19, C20 | Stop after side effect/before ACK, recover without duplicate action, revalidate evidence, user-complete task, reproducible release notes |

## First patch series

1. **C01:** add schema documents and golden payloads; add a stopped/consistent backup fixture; freeze migration from schema marker 2. No model host, MCP library or browser permission changes.
2. **C02–C03:** define principals/authority provenance, accepted task contract/decision and resource identities. Extend reducers and domain commands with strict revision preconditions. Existing CLI behavior remains covered.
3. **C04:** add immutable checkpoints and atomic head advancement. Keep new state out of `tasks.yaml` so checkpoint traffic cannot race the user's task editing unnecessarily.
4. **C05:** expose CLI checkpoint create/show/list and minimal `context`. Test a real restart against synthetic tasks, resource changes and a deliberately tiny context budget.

The first series is now implemented to the extent recorded above. Remaining acceptance gaps stay open. C20 depends on a real external interface; it is not a prerequisite for delivering useful checkpoints, MCP, evidence review or the GUI.
