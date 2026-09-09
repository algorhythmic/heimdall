# Revised implementation plan: everyday continuity and workspace recovery

Prepared September 8, 2026, from Heimdall commit `60175d1` (daemon 0.7.0, extension 0.2.0, database schema 6) and the current working tree.

Status: implementation started September 8. R0's Linux technical baseline, T01 readable resume/checkpoint drafts, initial W01 workspace/session identity, T02 Linux Herdr CLI integration and initial T03 Neovim commands are delivered in the working tree. User workflow timing measurements, broader editor-configuration acceptance and W01's observed/recovery envelopes remain open. See [verification evidence](VERIFICATION.md) and [current backlog](BACKLOG.md). The sequence below replaces the delivery ordering in the earlier [continuity plan](IMPLEMENTATION-PLAN.md) while retaining domain contracts and outstanding acceptance requirements. C01–C21 keep their identities and recorded completion status. W01–W09 retain the scope in the [workspace recovery proposal](WORKSPACE-RECOVERY-ROADMAP.md). T, P, and A identifiers add terminal, planning, and WCU work.

## 1. Direction and first useful outcome

Make Heimdall useful when returning to development, research, and planning in terminal, herdr, and LazyVim. Then make the task's application workspace recoverable after interruptions. Use the existing continuity and evidence backend throughout.

The first acceptance scenario is deliberately small: bind two tasks that share a repository to different terminal sessions, save a checkpoint, change projects, and return through a fresh session. Show the correct accepted direction, changed resources, blockers, and next action; flag stale bindings. A later increment restores the selected task's missing application views and reports exactly what survived, what was recreated, and what remains uncertain.

Delivery priorities:

1. Verify the existing build on the intended Linux environment; deliver readable resume/checkpoint workflows and explicit session binding.
2. Preserve planning progress, reviewed artifacts, and cross-project dependencies without requiring a code commit.
3. Capture durable workspace restore points and expose a current recovery preview.
4. Deliver manual, journaled application recovery and durable WCU outcomes using a shared action contract.
5. Add optional login recovery after manual recovery passes interruption tests. Evaluate retrieval and automatic agent continuation as subsequent capabilities.

This follows the [workflow assessment](OMARCHY-WORKFLOW-ASSESSMENT.md) and makes the recovery proposal concrete. Further GUI expansion, ranked planning, conversation ingestion, mail, and Braid do not block these increments. The September 8 user-directed TUI replacement supersedes the browser frontend; retain the underlying completion review and revalidation contracts.

## 2. Starting point and evidence limits

| Area | Current source and recorded capability | Work to carry forward |
|---|---|---|
| Storage | Single writer, WAL/FULL, schema 13, immutable events, command receipts, one JSON state projection; consistent database backup | Extend the existing writer and migrations; measure snapshot volume before high-frequency capture. See [store](../internal/store/store.go). |
| Continuity | Accepted contracts/decisions, resource digests, immutable checkpoints, deterministic context and drift checks | Human-readable CLI, general Git/worktree identity, structured artifact/action references, proposed/rejected decision review. See [model](../internal/model/continuity.go) and [context](../internal/continuity/context.go). |
| Authority | Separate CLI/browser credentials; scoped reads and explicitly granted checkpoint writes; local TUI completion review | New adapter/result and workspace capabilities need explicit policy. Existing grants must not acquire them during migration. See [ledger](CAPABILITY-LEDGER.md). |
| Evidence and GUI | Initial C08–C11 implementations are delivered | Preserve stale-evidence and completion revalidation. Raw-output retention, broader process/input coverage and post-completion notices remain open. |
| Browser | Paired profiles, epoch-bound ownership, inventory and command journal | C12/C13 verification and actual native-host registration; browser-to-compositor association is still an integration gate. |
| Desktop and sessions | Initial versioned manifests and generic session declarations; verified Linux Herdr binding/refresh/metadata; no observer or restore coordinator | W01–W08 plus thin terminal/editor integrations; generic terminal fallback is required. |
| Planning and external systems | Task/step hierarchy, local context, artifact review, manual preservation observations and scoped task dependencies; no programmatic preservation, WCU adapter, Braid integration, or execution host | Add these incrementally; repository presence or tool help is not integration acceptance. |

At planning time the repository was at the September 5 implementation checkpoint. The September 8 assessment and recovery proposal were already present as untracked documents, and BACKLOG already linked the proposal. Those are inputs, not newly implemented capabilities. The observations below describe that planning baseline; subsequent R0/T01 and initial W01/T02 implementation results are recorded in VERIFICATION.

Read-only checks made for this plan:

- Inspected the model, store/replay/auth boundaries, context CLI, browser records, smoke scripts, CI configuration, and current status/backlog.
- Go was not on PATH and no project build artifact was present. Go tests, builds, and compiled acceptance were not rerun. Prior passing results are the repository's recorded evidence, not a new CI verification.
- [CI](../.github/workflows/test.yml) runs Go/build/extension checks on Windows and Ubuntu, but compiled continuity/MCP/evidence/GUI checks only on Windows. The smoke scripts select `bin/heimdall.exe` directly. Portability of those checks belongs in R0.
- Installed herdr reports 0.8.2. At planning time, help exposed session listing/attachment, workspace and pane APIs, process information, agent inspection, and display-only metadata; only read-only inspection had been performed. T02 subsequently verified protocol-20 identity, movement, source restart and metadata in isolated sessions; see [Herdr setup](HERDR-SETUP.md). Process restoration/reattachment acceptance remains a W06 gate.
- Neovim reports 0.12.5 and Node reports 26.8.1; the repository CI selects Node 24. Installed versions must be recorded separately from tested compatibility. `hyprctl version` failed with a socket timeout setup error in this command environment; no live compositor acceptance was established.
- WCU's sibling checkout is at `10d4021` with uncommitted changes, including its [request-local ledger](../../wayland-computer-use/scripts/cu/execution.py) and [execution contract](../../wayland-computer-use/skills/wayland-computer-use/references/execution-and-context.md). Pin the actual adapter source and installed plugin separately at implementation time.
- dotprivate's sibling checkout is at `51d0fce`. Its [implementation](../../dotprivate/dotprivate) copies selected originals on ingest, then can commit, pull/rebase, and push. Synchronization stages the entire private clone. No synchronization was executed.

## 3. Release sequence and dependency gates

R labels denote reviewable increments, not promised release versions or dates. Risk describes integration uncertainty, not elapsed-time estimates.

| Increment | Deliverable / IDs | Entry dependencies | Exit gate | Risk |
|---|---|---|---|---|
| R0: Linux baseline | Portable compiled checks, clean schema-6 fixture, installed-capability record, isolated manual workflow baseline | Current 0.7.0 source | Reproducible Linux build and core/continuity/MCP/evidence/GUI checks; platform gaps recorded separately | Medium |
| R1: Return to a task | T01 readable resume/checkpoint flow; W01 identity/schema foundation; T02 session bindings/herdr metadata; T03 thin LazyVim commands | R0; T02 uses W01; T03 uses T01/T02 | Two tasks sharing a repo survive session replacement without wrong-task context or stale-pane writes | Medium |
| R2: Preserve planning work | P01 artifact references/Git identity; P02 progress and decision review; P03 optional dotprivate preservation; P04 cross-project progress/dependencies | R1; P02/P03 use P01; P04 uses P02; P03 programmatic dispatch also needs C12 | Research/design checkpoint without a code commit; exact artifact/version and unresolved dependency recoverable; offline preservation failures visible | Medium |
| R3: See saved workspaces | W02 observation/bindings; W03 autosnapshots; W04 List/Diff/preview | W01/T02; W03 follows W02; W04 follows W03 | Restart preserves a valid restore point; current scoped preview explains freshness and gaps without executing actions | High |
| R4: Recover manually | C12 shared actions; C13 browser verification; W05 operation coordination; W06 application adapters; W07 verified recovery report | W04 + C12 for W05; W06 browser portion also needs C13 | Selected task restores usable terminal/browser/editor views; crash after side effect never causes blind duplicate dispatch | High |
| R5: Preserve computer-use outcomes | A01 durable manually initiated WCU action capture; A02 reconciliation and task-evidence links | C12/shared W05 primitives, T02, existing C08/C09; P01 for retained artifact links | Interrupted WCU input remains uncertain until fresh observation; ledger and task acceptance remain distinct | High |
| R6: Recover at login | W08 readiness, optional restore policy, packaging and restart acceptance | W03–W07 and the manual recovery gate | One coordinator; daemon/compositor/reboot/controlled-VM crash cases pass on the documented target | High |
| Subsequent | C14–C16 retrieval; C17–C20 continuation/run controls; C21 broader integrated release | Existing C dependencies plus shared action/recovery primitives | Their original evaluation/host gates remain applicable | High |

Dependency details:

- T01 can ship before the W01 migration; it formats existing context and submits existing checkpoint commands. Do not delay the first usable CLI for every adapter.
- W01 defines session/surface identities once. T02 consumes them; W02 adds compositor observations to them. A session binding is useful before any native window is managed.
- C12 contract/fixture work can begin after R0 and be exercised against the existing browser. C13 and W02–W04 are independent until browser recovery in W06.
- R2 is the preferred usability priority; its dotprivate integration is optional. A missing private remote does not block local planning records or R3/R4.
- P03 programmatic preservation consumes C12's durable action/receipt infrastructure. Bring that infrastructure forward if implementing dispatch in R2; otherwise deliver preview/manual handoff and observed preservation receipts first. Do not introduce a separate dispatch journal just for dotprivate.
- A01/A02 can proceed once shared actions and operation serialization exist; they do not need every W06 application adapter or startup recovery. The preferred order establishes manual recovery first.
- C17 must reuse the action journal and operation-lock primitives introduced for W05. Its agent-run outbox, worktree ownership, limits, and external-writer fencing remain additional work. Workspace restoration is not gated on all of C17.
- C21 still names the earlier broad release and retains C15/C20 requirements. R4 and R6 have their own workspace gates; neither claims C21 completion.

## 4. Terminal and planning implementation slices

T01 commands are now implemented as documented in [CONTINUITY-SETUP.md](CONTINUITY-SETUP.md); initial T03 editor commands are in [NEOVIM-SETUP.md](NEOVIM-SETUP.md). Initial W01 manifest and generic binding commands are documented in [WORKSPACE-SETUP.md](WORKSPACE-SETUP.md). Other commands in this section are proposed interfaces unless explicitly described as existing. Freeze spellings and request schemas in their first patch; preserve existing JSON defaults for scripts.

| ID | Scope and implementation | Acceptance |
|---|---|---|
| T01 | Add `heimdall resume TARGET` as a readable view over the existing context service, with JSON available explicitly. Add a checkpoint draft/edit/submit helper using existing revision, contract, previous-head and request-ID preconditions. Display accepted direction, checkpoint age, resource drift, blockers, next action, and evidence review needs. | No mutation from resume; missing/stale context remains visible; terminal control sequences in captured text are escaped; competing checkpoint writes preserve the draft and return a conflict. Existing `context` JSON remains compatible. |
| T02 | Bind task/project, canonical repository/worktree, environment/host, herdr session/workspace/pane, and optional agent session to W01 logical identities. Add explicit bind/show/unbind and refresh operations. Resolve actual assigned IDs through the installed herdr API; publish namespaced task/next-action/review metadata with a stale/disconnected indication. | Same-repo tasks and same-class terminals remain distinct; inherited environment and cwd are hints only; pane move/replacement and server restart invalidate or revalidate bindings. Agent runtime status cannot mark a task complete. |
| T03 | Add a small repo-owned Neovim/Lua integration for task selection, resume view, checkpoint draft submission, artifact opening and launching existing completion review. Use structured CLI/API responses and Neovim APIs. Package an example setup rather than changing global editor configuration during development. | Isolated Neovim config covers task switching, stale checkpoint conflict, unavailable daemon/herdr, and artifact opening. No implicit completion ratification, shell-command execution from task text, or credential exposure in buffers. |
| P01 | Add stable artifact IDs and immutable artifact versions tied to explicit resource bindings: project/task, environment, relative locator, observed digest, optional repository/worktree/commit/ref and dirty-input identity. Extend checkpoint references through a new validated payload version. | Same paths in different projects/environments do not collide; moved/missing originals and changed content are distinct states; non-Git planning works; old checkpoints remain readable and replayable. |
| P02 | Add proposal/review events for decisions and artifact progress. Track artifact lifecycle (draft/reviewed/accepted/superseded) separately from task/step blocked/completed state. Acceptance binds the reviewed digest and applicable contract; material changes require review again. Keep accepted decisions in mandatory context and show unresolved proposals separately. | A file's existence, an agent report, or “reviewed” state alone cannot complete work. Stale decision/artifact acceptance is refused. Existing completion proposal/revalidation remains authoritative; authorized manual attestation still works. |
| P03 | Add an optional, explicit checkpoint-linked preservation operation for selected dotprivate artifacts, with preview, result/status inspection and safe reconciliation. Keep local checkpoint persistence independent of remote success. Export portable progress metadata without live credentials or the database. | No code commit required. Offline, refused file, missing original, rebase conflict, changed source during copying, and failed push retain distinct outcomes. Exact local/mirrored digests and confirmed repository revisions are recorded. |
| P04 | Add an explicit task dependency relation for cross-project work and a scoped summary showing latest meaningful checkpoint, decision awaiting review, next action and blockers. Reuse task hierarchy and step `after` semantics where they already apply. | Cycle and stale-revision checks; revoked scope cannot expose another project's title/path or history; satisfied dependencies update the view but never dispatch agents. Sort by explicit fields and label it as such; no ranked-planner claim. |

P03 has an important upstream boundary: dotprivate ingest may call a synchronization routine that stages **all** changes in the private clone. Passing selected input paths does not constrain that later commit. The initial adapter must inspect and serialize access to the private checkout, refuse unrelated dirty/staged changes under a selected-files policy, and verify the preservation result. If the installed version cannot establish that boundary, provide a preview/manual handoff and record the result; a narrow upstream interface is then a separate prerequisite for unattended preservation. A remote failure cannot be turned into success by an unchanged-file ingest retry that never retries the push.

Local checkpoint save, artifact observation, mirror copy, private commit, and remote publication are separate facts; there is no distributed transaction. Store the checkpoint and a requested preservation receipt locally, run an explicitly authorized preservation operation, then append its result. Reconcile partial results before retrying. Never rewrite the immutable checkpoint to pretend later publication happened at checkpoint time.

Artifact bindings remain explicit. General tree observation currently excludes `.git`, not `.private`; the adapter must not silently register the whole sidecar or broaden an accepted contract's resource scope. File digests are references, not retained file contents. Artifact acceptance is bound to bytes; private storage location alone does not provide Heimdall authorization or encryption.

## 5. Workspace implementation slices

Implement the existing [Viewport contract](design/HANDOFF-heimdall-v1.1.md#8-browser-and-viewport-operations) in Go. Adopt Hyprflow's capture, recipe, preview and testing concepts from the pinned comparison in the recovery proposal. No Hyprflow runtime dependency or fork is required. Any copied/translated source or fixtures must carry their pinned origin and MIT notice.

For the revised M1b milestone, acceptance includes W01–W07 and the daemon/compositor/reboot/crash demonstrations in W08. Optional automatic login restore and installation packaging overlap M4; installing a login hook is not required to demonstrate manual recovery after reboot. Early R3/R4 releases must state their tested interruption boundaries rather than claiming the complete M1b gate.

| ID | Concrete work | Depends on | Required evidence |
|---|---|---|---|
| W01 | Versioned desired manifest, logical surface/session binding, observed snapshot/topology, recovery request and result schemas. Add event reducers, scoped lookups, migration and backup fixtures. | Existing store/auth; C01–C03 patterns | Desired/observed separation; old marker-6 upgrade, unknown-version refusal, inert replay, binding scope negatives. |
| W02 | Read-only Hyprland adapter: bootstrap snapshot with buffered events, named workspace and monitor mapping, source epochs, event-gap detection, periodic reconciliation. Join to explicit T02 bindings; unknown matches remain unowned. | W01, T02 | Duplicate class/title, moved windows/panes, reused addresses, event gaps, source restart and ambiguous pairing. |
| W03 | Debounced autosnapshot with configurable maximum dirty interval, atomic head publication, manual points, freshness/coverage diagnostics and bounded payload retention. | W01–W02 | Interrupted commit yields old or new complete point; outage/startup/shutdown cannot erase desired membership; pinned snapshots survive retention. |
| W04 | Scoped live List/Diff and `workspace preview` against a pinned manifest/snapshot plus fresh observations. Return reattach/launch/move/leave-open/unavailable/review-required dispositions. | W01–W03 | Deterministic diff, stale-plan detection, no external action from preview; source inconsistency and changed displays are visible. |
| W05 | Journaled open/focus/close, operation status/cancel/reconcile, transactional resident-capacity reservations and conflicting-surface serialization. Save verified desired membership before graceful close. | W04, C12 | Capacity/swap races, app refusal, cancellation in flight, lost launch acknowledgement and no dispatch during replay. |
| W06 | Application adapters in order: generic terminal and herdr; paired browser; supported editor session state. Reviewed executable/argv/cwd or typed attach recipes; actual new runtime identity is rebound after observation. | W05; browser also C13; installed capability fixtures | Surviving session reattachment, expired session limits, browser self-restore dedupe, missing executables/worktrees/profiles, unsaved editor state. |
| W07 | Verify existence, ownership, task/workspace membership, usable display placement and supported attachment/window state. Add per-surface and aggregate recovery reports with explicit uncertainty. | W05–W06 | Wrong move/session, unknown coverage and failed close cannot produce full recovery. Monitor fallback is applied only under the selected policy and reported. |
| W08 | Optional user-session startup coordinator with desktop/browser/herdr readiness, bounded waiting, manual override, policy revision binding, packaging and upgrade/uninstall instructions. | W03–W07 | Duplicate startup suppressed; locked/unready desktop waits visibly; daemon/compositor/reboot/controlled-VM interruption demonstrations. |
| W09 | Optional import of the pinned Hyprflow snapshot schema into a draft manifest. Defer until an actual saved-session migration need exists. | W01, W04 | Explicit task/surface adoption; schema validation; imported command hints never execute automatically. |

Snapshot policy starts with the proposal's **30-second maximum dirty interval while healthy**, as a validation target rather than an established guarantee. Manifest acceptance commits immediately. Record source capture boundaries, capture-to-commit latency, age and missed targets. Sources are not one atomic desktop transaction; keep the last complete restore point when current coverage is degraded.

WIP capacity is reserved before launch, including unresolved operations that may have created surfaces. Closing or cancelling does not release capacity until reconciliation justifies it. An explicit swap names the resident to close and can fail partially. Leave unowned windows alone; an unsaved prompt or refused close leaves partial residency. Never fall back to killing processes.

R4 restores selected tasks. A daemon restart should rediscover existing surfaces without relaunching them. A compositor restart needs new identity/coverage checks. Reboot cannot recover terminated local processes: reopening a terminal at a reviewed cwd is a degraded result, while a remote session can be reattached only after independently proving it survived. Editor buffers/undo/unsaved text are not recovered merely because an editor window reopens.

Monitor recovery prioritizes named workspace/task membership and usable logical coordinates. Record scale/transform and clamp geometry under an explicit fallback policy; otherwise report review required. Exact tiled split ratios and pixel-identical layouts are deferred. Browser native-to-compositor mapping still requires fresh nonce pairing; duplicate URLs, counts and ordinary titles cannot establish ownership.

## 6. Shared actions and WCU integration

C12 is a shared substrate, not a second browser stack. Add a task-bound action record with request ID, logical target, manifest/contract/resource revisions as applicable, adapter/runtime epoch, authority reference, intended postcondition, attempt identity and observation references. Preserve separate execution and verification states from the earlier plan. Existing browser success remains API-reported and unverified when migrated; keep browser IDs, extension identity, pairing rules and retry receipts compatible.

Use one daemon-owned operation journal and shared serialization primitives for browser, workspace and WCU actions. Persist authorized intent before dispatch. Revalidate authority and relevant identity immediately before dispatch; reconcile results afterward. A crash between dispatch and receipt leaves an uncertain operation. A new request ID or expired coordinator lease is not proof that repeating an external side effect is safe. Observation can settle a prior operation without creating a new execution attempt.

| ID | Implementation | Acceptance |
|---|---|---|
| A01 | Pin one WCU adapter contract; add authenticated, task-scoped begin/result operations around manually initiated WCU work. Persist the intent before the wrapper submits input and then retain per-step injection/verification/uncertainty plus bounded observation/artifact references. Link actions to checkpoints through versioned references. | Host termination before dispatch and after input/before result produces distinguishable recorded states. Duplicate/late/forged/wrong-task results cannot overwrite another attempt or manufacture trusted evidence. A retrospectively imported log remains an attributed report. |
| A02 | Add explicit observation-based reconciliation and an evidence adapter for specific accepted postconditions. Supply accepted task context to the WCU wrapper and require fresh observations on resumption. Use shared desktop-input serialization for cooperating dispatchers and stop new input on cancellation/revocation. | Partially submitted input is not retried blindly. Repaint/stability/tool success does not establish task completion. Fresh exact readback or independent artifact evaluator can verify the relevant criterion; stale frame/accessibility identities confer no input authority. |

Do not give an existing MCP checkpoint client the unrestricted CLI credential. Introduce a narrow principal/grant capability for reporting results on an authorized action and constrain it to that action/attempt, task and adapter. CLI/explicit restore policy remains the initial workspace command authority. Read permission, task context, `ready`, herdr agent state and saved recipes do not grant execution authority. Reuse authorization already recorded for the permitted operation; an expanded scope needs its own authority.

WCU retains immediate input guards; Heimdall retains durable intent and recovery history. The current WCU ledger is request-local, so merely saving successful responses cannot cover a host that disappears before returning them. A01's pre-dispatch wrapper is part of the interruption contract. Its ledger distinguishes backend input submission from application acceptance; preserve that distinction in Heimdall. Desktop input locks coordinate participating clients and cannot prevent the user or an unrelated client changing the desktop.

Retrieval remains optional in both systems. WCU's optional Braid context adapter does not implement Heimdall's C14–C16 workstream-scoped memory. Restoring application views or attaching a terminal must never implicitly send a prompt, rerun a captured shell command, or continue an agent run.

## 7. Code boundaries, migration and operational data

Extend current packages where their responsibilities already fit. Proposed new paths below are implementation destinations, not existing files.

| Path | Change |
|---|---|
| `cmd/heimdall/`, `internal/client/`, `internal/daemon/` | Resume/checkpoint helpers, binding/artifact/workspace/action endpoints and bounded status reads. Preserve daemon-only database access. |
| `internal/model/`, `schemas/`, `testdata/` | New workspace/session/artifact/action payloads, explicit lifecycle/version validation, requests and Go/TypeScript conformance fixtures. Reuse the current schema location. |
| `internal/store/`, `internal/authz/` | Reducers, atomic heads/receipts/reservations, migrations, scoped queries, new capabilities and revocation checks. |
| `internal/continuity/`, `internal/checks/` | Artifact/Git references, decision review, context presentation data, action-to-evidence mapping and existing completion revalidation. |
| `internal/workspace/`, `internal/adapters/hyprland/`, `internal/adapters/herdr/` | Desired/observed reconciliation, capture scheduling, Viewport service and bounded external interfaces. Use platform build constraints or explicit unsupported adapters so Windows core builds continue. |
| `internal/actions/`, `internal/adapters/wcu/`, `internal/adapters/dotprivate/` | Shared operation lifecycle and authenticated outcomes; thin WCU and preservation adapters. Dotprivate's Git operations also need durable receipt/reconciliation. |
| `integrations/nvim/`, `scripts/`, `.github/workflows/` | Thin Lua commands, portable smoke harness and fake/installed-adapter acceptance. No broad editor distribution or shell framework. |
| `internal/tui/` | Maintain terminal views; expose recovery/progress details from the same domain state. Run controls stay with C19. |

Before each event-family change, add schema-6 upgrade fixtures and retain previous event versions. New fields cannot be slipped into strict old payloads: legacy checkpoints use versions 1/2 with different provenance rules; P01 adds CLI version 3 through request version 2. Freeze a new version and compatibility behavior for artifact/action links, preserve old records, and test old/read credentials retain exactly their former authority. Advance the database marker when old binaries would otherwise misread new state; test old-binary refusal and stopped-backup rollback.

Snapshot frequency creates a specific storage risk: every command currently reads and rewrites the complete JSON projection, and old events are retained. W03 must measure representative daily capture volume, event/projection growth, writer latency and restore-point lookup before enabling autosave. Store large immutable snapshot payloads in a dedicated table in the **same SQLite database**, with event metadata/digests and bounded current heads; publish payload, event and head through the same writer transaction. Do not copy full desktop payloads into every projection revision.

Define workspace payload retention separately from task event retention. Protect accepted/current manifest references, manual pins and unfinished-operation snapshots. Pruning emits an explicit retention record; a historical snapshot whose payload was pruned remains identifiable but unavailable for restoration. Replay rebuilds metadata/heads and preserves that unavailability without recapturing the desktop. Backup/restore must include retained payloads. Choose further per-entity normalization from measurements rather than starting with a repository-wide storage rewrite.

Keep external I/O outside long-held writer transactions: observe with source/revision boundaries, then validate and commit; commit dispatch intent, then call the adapter. Use bounded queues, calls and retry budgets, an injected clock and process/boot epochs. Do not hold the task writer while waiting for a compositor, subprocess, private remote or user dialog.

Configuration work is limited to registered adapters, capture/retention policy, selected launch/restore recipes and authority needed by these slices, preserving `--data-dir`. Full XDG/config/catalog redesign is not a prerequisite. Store credentials outside artifacts, event payloads and exported progress. Keep the live database local; multi-machine state replication remains separate from private artifact synchronization.

## 8. First patch series

Start with these small patches; each has its own evidence and can be reviewed without implementing the full roadmap.

1. **R0 baseline and portable harness.** Parameterize executable selection and temporary-data paths in compiled smoke scripts; keep Windows support and run the relevant compiled checks on Ubuntu. Record exact toolchain versions and a schema-6 upgrade fixture. Establish the manual task-switch/planning baseline. Native desktop tests remain a separate installed-target gate.
2. **T01 resume view.** Format existing context through a new readable command. Cover missing contract/checkpoint, stale resource, explicit task selection, bounded output and terminal-text escaping. Preserve current `context` JSON.
3. **T01 checkpoint helper.** Create a draft from the current task/contract/head, let the user edit it, then submit with the original preconditions and stable request ID. Save conflicting drafts for reconciliation. This does not require a new event schema.
4. **W01 identity foundation.** Freeze logical surface/session IDs and desired/observed/workspace envelopes. Add schema migration/replay/scope fixtures and binding commands; no launch or desktop mutation yet. Reserve extensibility for action references without implementing an agent run state machine.
5. **T02 herdr binding and metadata.** Prove installed API identity semantics in an isolated session. Add read/reconcile and display-only metadata integration; validate same-repo tasks, pane movement and stale bindings. A failed capability spike narrows support rather than enabling command-text heuristics.
6. **T03 editor wrapper.** Deliver a small optional LazyVim setup against those stable commands, then demonstrate the R1 task-switch scenario. Continue into P01/P02; take C12 contract work alongside R2/R3 when useful.

Implementation update: the technical work in patch 1 and both T01 patches (2–3) are delivered and tested on Linux. Go/Node tools were installed under ignored project-local paths. A synthetic terminal was opened for WCU verification; no user database was migrated. Workflow timing measurements remain open. Patches 4–6 are next.

## 9. Validation and release evidence

Run meaningful targeted tests for each changed boundary, then the established regression checks affected by the slice. Documentation-only planning does not establish those implementation results.

| Boundary | Mandatory scenarios |
|---|---|
| R0/R1 continuity | Linux compiled daemon/client restart; MCP grant revocation; same-repo tasks; changed worktree; moved/replaced pane; missing herdr; stale checkpoint conflict; unchanged JSON consumers. |
| R2 planning | No-code-commit research session; digest changes after review; missing/stale mirror; source changes during preservation; offline push/rebase conflict; unrelated private changes; dependency cycle; cross-scope read denial. |
| W03 durability | Kill during capture and publication; disk/commit failure; empty startup and shutdown observations; expired source coverage; retention of active references; backup restores payloads and heads. |
| C12/C13 and R4 | Old browser success stays unverified; real native registration; duplicate URL/title/class; reused epoch/address; browser self-restore; concurrent open/capacity race; user movement; app refusal; launch succeeds before daemon loses ACK. |
| R4/R5 uncertainty | Result delay/loss, cancellation/revocation in flight, unknown old execution, failed postcondition and unavailable readback. Re-observation can settle status; event replay and status queries never dispatch side effects. |
| W07/W08 platform | Missing executable/cwd/profile/session; wrong terminal attachment; monitor reorder/scale/unplug; locked desktop; partial close; duplicate login trigger; daemon/compositor/reboot and controlled VM power interruption. |
| Existing completion | Wrong-resource/stale/partial/forged action evidence does not ratify completion; user acceptance remains explicit; historical accepted completion is preserved. |

Use fakes for deterministic faults and scope races. Use dedicated native-browser profiles and isolated desktop/session fixtures for actual adapter checks. Reboot and power interruption belong in a disposable VM or designated test environment; process-kill tests alone do not prove crash durability. Record filesystem/storage assumptions, snapshot age at failure, unrecovered interval and observed outcome per scenario.

Measure the workflow using the same consented task-switch and planning cases before and after R1/R2: time to identify the correct task and next action, repeated context supplied, missing/stale decisions, and checkpoint/preservation effort. For recovery, record required surfaces restored, unresolved/incorrect bindings, duplicate dispatch, snapshot freshness, capture failures and time to a usable workspace. Set numerical performance targets after the baseline; incorrect ownership, silent uncertainty and duplicate uncertain input are correctness failures, not speed tradeoffs.

Each increment ships with source/tests, migration and rollback evidence, supported-version/platform matrix, installed acceptance results where applicable, known gaps, setup instructions, and updated STATUS/BACKLOG. Keep schema, adapter/API success, verified workspace recovery, and completed development task as separate claims.

## 10. Deferred work and decisions that can block a slice

| Decision or capability | Working default | Blocking point / fallback |
|---|---|---|
| herdr session/agent identity and detach behavior | Validate installed 0.8.2 semantics; use actual IDs | Blocks the corresponding T02/W06 capability only. Generic terminal reopening reports lost process continuity. |
| Editor session fidelity | Use a configured, validated session mechanism | Blocks claims of buffer/session recovery; reopening reviewed files remains a limited capability. |
| Snapshot cadence/retention | Start validation at 30 seconds while healthy; bound retained payloads with explicit pins | Measurement and crash tests block W03 autosave acceptance, not T01. |
| Monitor fallback and login behavior | Manual selected-task restore first; saved policy controls fallback/startup | No applicable policy yields review-required; W08 is optional. |
| dotprivate selected-files boundary | Inspect current clone and installed ingest/sync behavior | Unrelated changes or insufficient API guarantees block automatic preservation; local checkpoints and manual preservation remain useful. |
| Trusted WCU reporting | Pinned wrapper with durable begin/result and narrow grant | Blocks A01/A02 trusted evidence; imported text remains an agent/adapter report. |
| Hyprflow import | Defer W09 | Only implement for an actual migration need; core restore has no Hyprflow dependency. |
| Braid and real execution host | Keep disabled until separately configured and validated | C14–C16 and C20 gates apply. Context, planning and manual workspace recovery work independently. |

Remaining C03/C08/C09 gaps should be closed where these slices consume them: general Git/artifact identity and decision review in P01/P02; specific WCU evidence coverage in A02. Raw-output retention, detached-process coverage and post-completion notices remain explicit backlog gaps unless separately delivered. GUI task editing/run controls, automatic source ingestion, mail, ranked planning, exact tiled layouts, Windows/macOS viewport parity and live-database replication keep their existing separate scope.

**P02 CLI/TUI progress review and Neovim inspection/handoff** are implemented; see [progress setup](PROGRESS-SETUP.md). It adds immutable proposals, explicit review/rejection/acceptance, digest/contract/artifact revalidation, mandatory accepted decisions and separate unresolved context, with schema-11 compatibility gates and local CLI authority in the TUI. TUI proposal authoring and agent proposal grants remain open. **P03 manual preview/handoff and observed preservation receipts** are delivered under schema 12; see [preservation setup](PRESERVATION-SETUP.md). Programmatic dispatch remains gated by C12 and a narrow upstream interface. **P04 dependencies and scoped summaries** are delivered under schema 13; see [dependency setup](DEPENDENCY-SETUP.md). Next is **R3/W02 observation and bindings**, followed by W03 autosnapshots and W04 preview, after this foundation and the initial local Linux [P01 artifact/version slice](ARTIFACT-SETUP.md), following the delivered R0 technical baseline, T01–T03 and initial W01 identity foundation. W01 manifests, stable task-owned surfaces, generic session declarations, scoped CLI lookups and schema-9 migration/replay are implemented. Initial T02 now verifies local Herdr 0.8.2 identities, supports explicit live checks/rebinding and publishes expiring metadata with optional pane-labelled sidebar summaries. Initial T03 adds Neovim task selection, resume, checkpoint drafts, bound artifact opening, session checks and TUI handoff; see [editor setup](NEOVIM-SETUP.md). P01 adds immutable task/environment/host-owned file identity, optional selected-input Git metadata and explicit checkpoint pins; its schema-8→9 migration, replay, restart and backup restore gates pass. File contents are not retained. Automatic refresh, remote/version expansion and restoration are not delivered. Observed snapshot/topology and recovery/action envelopes remain open and will be frozen with their consumers; the full W01 and R1 gates are not complete. Collect user workflow timing measurements alongside this work before making productivity claims.
