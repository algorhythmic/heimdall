# Implementation backlog

Updated 2026-09-08. **The Linux technical baseline, T01 terminal continuity and initial W01/T02 identity and live Herdr adapter are implemented on the 0.7.0 foundation.** The [revised implementation plan](REVISED-ROADMAP-IMPLEMENTATION-PLAN.md) defines the execution order, retaining the contracts in [IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md). Evidence is recorded in [VERIFICATION.md](VERIFICATION.md). Generic declarations, the local Linux Herdr workflow, initial Neovim commands, P01 artifact versions and P02 CLI/TUI review with editor handoff are delivered; automatic refresh and workspace recovery remain open.

Related proposal, 2026-09-08: [Workspace recovery roadmap](WORKSPACE-RECOVERY-ROADMAP.md) recommends Hyprflow-inspired capture, durable snapshots, application reattachment/relaunch, and verified recovery after restart or crash. It defines proposed W01–W09 additions to M1b and their C-series dependencies; current completion status is unchanged.

## Current progress

| Items | State | Delivered / remaining |
|---|---|---|
| R0 | Technical baseline complete | Local Linux Go/vet/build, UI build, extension and compiled core/native/continuity/MCP/evidence/GUI checks pass; portable harness and schema-6 fixture delivered. Published CI exposed a Herdr inode-reuse defect, covered by a fix and deterministic regression; see [verification](VERIFICATION.md) and current Actions runs. Native registration and user-measured workflow timing remain open. |
| T01 | Initial slice complete | Readable/JSON resume and explicit draft/edit/submit workflow; control escaping, scope/review counts, budgets, no writes from resume, conflict/transport file retention, abrupt restart and exact retry tested. WCU inspected the Linux terminal output. |
| W01 | Identity foundation complete; broader slice partial | Versioned manifests, stable task-owned surfaces, generic bind/show/unbind, scoped CLI lookups, strict reducers, schema-7 upgrade/backup/replay and actual old-binary refusal/rollback pass. Observed snapshots/topology and recovery/action envelopes remain open. |
| T02 | Initial Linux CLI slice complete | Herdr 0.8.2/protocol-20 identity checks, canonical cwd/Git, explicit refresh/rebind and expiring metadata pass installed move/restart/readback tests. WCU verified optional pane-labelled sidebar text. Automatic refresh, background publication and other hosts/versions remain open. |
| T03 | Initial Linux editor slice complete | Explicit task/step selection, resume, retained checkpoint drafts/submission, bound artifact opening, session checks and GUI handoff. Neovim 0.12.5, isolated restart/conflict/path tests, lazy.nvim example loading and WCU rendering verified. Full user-config/LazyVim distribution acceptance and workflow timing remain open. |
| P01 | Initial Linux slice complete | Stable task/environment/host-owned artifact IDs, immutable file versions, optional Git identity, v3 checkpoint pins and v2 drafts. Scope, drift/relocation, exact retry/replay/restore and schema-8→9 upgrade/rollback pass. File retention and other platforms remain open; see [artifact setup](ARTIFACT-SETUP.md). |
| P02 | CLI/TUI review and editor handoff delivered | Immutable decision/artifact proposals, explicit review/rejection/acceptance, digest/contract/version checks and artifact lifecycle separate from task completion. Mandatory accepted direction and separate unresolved context; schema-11 compatibility/replay/restart/restore. Native terminal review, artifact inspection and Neovim inspection/terminal handoff are delivered. TUI proposal authoring and scoped proposal-write grants remain open. See [progress setup](PROGRESS-SETUP.md). |
| C01 | Initial slice complete | Request schemas/fixtures, golden events for old/new contracts, decisions, resources, CLI/client checkpoints and read/write grants; schema-6 migration, replay and actual 0.5.0 upgrade/backup/restore pass without elevating read credentials. Future record types need their own fixtures. |
| C02 | Policy documented | [Capability ledger](CAPABILITY-LEDGER.md) ties CLI/browser boundaries to tests and defines required principal/grant policy. Actual scoped credentials and grants belong to C06. |
| C03 | Partial | Accepted contracts/decisions, supersession, revision checks, canonical file/tree registration and bounded observations work. Version-2 contracts freeze explicitly supplied, reviewed resource IDs; changed scope requires reacceptance. P02 adds CLI proposed/reviewed/accepted/rejected decisions and version-bound artifact review; native terminal review is delivered; proposal authoring and other-platform artifact identity remain open. |
| C04 | Initial slice complete | Immutable append/list/get-by-ID, explicit previous head, retry, competing-write, restart and replay checks pass. Run/evidence links will arrive with their consuming slices. |
| C05 | Initial slice complete | CLI context includes mandatory task/ancestor contracts, decisions, checkpoint and resource drift without retrieval; small budget fails explicitly. Estimate is UTF-8 bytes/4. CLI artifact pins add optional selected-file Git identity; exact tokenizer accounting is not claimed. |
| C06 | Initial slice complete | Scoped reads plus explicit checkpoint-write grants. Authority is checked inside the writer transaction before dedupe and commit; records carry authenticated grant/author provenance. Tests cover read-only denial, cross-target/cross-grant denial, expiry, revoked retries, conflict, rollback and replay. Broader machine mutations are not delegated. |
| C07 | Initial slice complete | Official Go SDK v1.7.0 stdio adapter; task/context/history/checkpoint tools, structured errors, stable request IDs and daemon restart rediscovery. Official SDK client uses 2026-07-28; compiled stdio smoke uses 2025-11-25. User-host registration and Linux desktop deployment remain open. |
| C08 | Initial CLI slice complete | Artifact existence/digest, exact-root repository predicates and configured test execution; accepted definitions, durable attempts, complete declared resource observations, lineage/decision binding, bounded output/executable/environment digests, retry/restart/replay and malformed/forged/partial/stale negatives. Raw-output retention, broader evidence tools and stronger external-input/process-tree coverage remain open. |
| C09 | Initial CLI slice complete | Explicit invalidation, task/step proposals and live revalidation before ratification. Contract/resource/repository changes block stale completion; accepted task history remains. Continuous invalidation and dedicated post-completion review notices remain open. |
| C10 | Replaced by local TUI | The native terminal client reuses CLI authority and backend review APIs. Browser frontend/session routes are retired; historical review provenance remains replayable. |
| C11 | Terminal replacement delivered | Needs-you, expandable workstreams, selected context, review/save/file/workspace/binding dialogs, find, compact layout and retained retries. Real PTY and simulation checks pass. Native desktop bars and full workspace recovery remain open. |
| C12–C21 | Planned | Verified actions, retrieval, execution-host slices and broader GUI run controls remain unimplemented. |

P02 CLI/TUI progress review and Neovim inspection/handoff are implemented on the terminal/editor and P01 artifact foundation. Next R2 work is P03 optional preservation, alongside user workflow baseline measurements and later review interface expansion. C03/evidence gaps are completed where the new slices consume them; C12 becomes shared infrastructure for verified recovery and WCU. P01 checkpoint pins include optional local Linux artifact/Git identity; file-content preservation and remote/platform expansion remain open. Editor usage is in [NEOVIM-SETUP.md](NEOVIM-SETUP.md); other interfaces are documented in [GUI-SETUP.md](GUI-SETUP.md), [EVIDENCE-SETUP.md](EVIDENCE-SETUP.md), [MCP-SETUP.md](MCP-SETUP.md), [SCOPED-ACCESS.md](SCOPED-ACCESS.md) and [CONTINUITY-SETUP.md](CONTINUITY-SETUP.md).

## Existing C-series deliverables and dependencies

These identifiers and completion requirements are retained. Their numeric order is not the revised execution order; use the R increments below. C17 reuses shared action/operation primitives from W05, while its agent-run coordination remains separate.

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
| C10 | Replaced by local TUI | The native terminal client reuses CLI authority and backend review APIs. Browser frontend/session routes are retired; historical review provenance remains replayable. |
| C11 | Terminal replacement delivered | Needs-you, expandable workstreams, selected context, review/save/file/workspace/binding dialogs, find, compact layout and retained retries. Real PTY and simulation checks pass. Native desktop bars and full workspace recovery remain open. |
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

## Revised delivery order

R0's technical work, T01 and the initial W01 identity foundation are delivered as recorded above; W01's remaining observed/recovery schemas and other new items remain open. Detailed code boundaries, acceptance scenarios, capability gates and the patch sequence are in [REVISED-ROADMAP-IMPLEMENTATION-PLAN.md](REVISED-ROADMAP-IMPLEMENTATION-PLAN.md).

| Increment | IDs / deliverable | Dependencies and release gate |
|---|---|---|
| R0 | Linux build/compiled smoke baseline, schema-6 fixture, installed capability record | Current build; portable compiled checks and documented platform gaps |
| R1 | T01 readable resume/checkpoint helper; W01 workspace/session schema; T02 explicit herdr bindings/metadata; T03 thin LazyVim integration | R0; T02 uses W01, T03 uses T01/T02; correct task context after same-repo session switching |
| R2 | P01 artifact/Git references; P02 progress/decision review; P03 optional dotprivate preservation; P04 scoped cross-project dependencies/view | R1; P02/P03 use P01, P04 uses P02; P03 dispatch also requires C12; planning without code commits and version-bound review |
| R3 | W02 Hyprland observation; W03 durable autosnapshots; W04 live List/Diff/preview | W01/T02, then W02 → W03 → W04; retained complete restore point and current scoped preview |
| R4 | C12 shared actions; C13 browser verification; W05 journaled operations; W06 application adapters; W07 recovery verification | W04+C12 for W05; browser W06 also needs C13; manual recovery without blind duplicate dispatch |
| R5 | A01 durable manually initiated WCU intent/results; A02 reconciliation and evidence links | C12/W05 shared primitives, T02, C08/C09 and P01 artifact links; interrupted input remains uncertain until fresh verification |
| R6 | W08 optional login restore, packaging and restart/crash acceptance | W03–W07; one coordinator, readiness/policy gates, actual restart and controlled VM tests |
| Later | W09 optional Hyprflow import; C14–C16 retrieval; C17–C21 continuation and broader integrated release | Original acceptance gates retained; no retrieval/agent-host prerequisite for manual workspace recovery |

C12 can begin after R0 alongside the terminal/planning/capture work. P03 uses its shared journal for programmatic preservation; preview/manual handoff can ship earlier. R5 can start once its shared action dependencies exist without waiting for every application adapter. R6 does not require WCU. The preferred order validates manual use before startup automation. Remaining C03/C08/C09 and broader M-series gaps stay open unless a slice supplies their completion evidence.

## Historical initial patch series (C01–C05)

1. **C01:** add schema documents and golden payloads; add a stopped/consistent backup fixture; freeze migration from schema marker 2. No model host, MCP library or browser permission changes.
2. **C02–C03:** define principals/authority provenance, accepted task contract/decision and resource identities. Extend reducers and domain commands with strict revision preconditions. Existing CLI behavior remains covered.
3. **C04:** add immutable checkpoints and atomic head advancement. Keep new state out of `tasks.yaml` so checkpoint traffic cannot race the user's task editing unnecessarily.
4. **C05:** expose CLI checkpoint create/show/list and minimal `context`. Test a real restart against synthetic tasks, resource changes and a deliberately tiny context budget.

The first series is now implemented to the extent recorded above. Remaining acceptance gaps stay open. C20 depends on a real external interface; it is not a prerequisite for delivering useful checkpoints, MCP, evidence review or the GUI.
