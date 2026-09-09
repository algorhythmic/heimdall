# Workspace recovery: Hyprflow reuse and Heimdall roadmap proposal

Date: September 8, 2026.

Status: recommended development additions. This document defines proposed scope, dependencies, and acceptance criteria; it does not claim implementation or schedule installation. Existing C01–C21 identifiers and completion status remain unchanged.

Companion: [Omarchy workflow assessment](OMARCHY-WORKFLOW-ASSESSMENT.md). Governing interfaces: [Viewport and workspace manifests](design/HANDOFF-heimdall-v1.1.md), [implementation plan](design/history/IMPLEMENTATION-PLAN.md), and [backlog](BACKLOG.md).

## Recommendation and intended outcome

**Make workspace restoration after a daemon restart, compositor restart, reboot, or system crash an explicit deliverable of Heimdall's workspace milestone.** Use selected Hyprflow concepts and implementation patterns in Heimdall's Go adapters. Keep Heimdall responsible for task ownership, desired workspace state, authorization, and durable recovery. A Hyprflow runtime dependency or fork is not recommended for the initial implementation.

The outcome is a recoverable project workspace: the developer can return to the correct task, see which surfaces survived, reattach or reopen missing applications, and understand what could not be recovered. For the terminal/herdr/LazyVim workflow, restoring the right session and task context takes priority over reproducing every pixel of a tiled layout.

Three workstreams are essential:

1. Save sufficiently recent, consistent restoration snapshots before an unexpected crash.
2. Relaunch or reattach applications and map new runtime identities to saved logical surfaces.
3. Verify the resulting workspace and report missing applications, changed monitors, or unavailable sessions.

These workstreams extend the planned `Viewport.Open`, `Close`, `Focus`, `Diff`, and `List` behavior. They do not require Braid retrieval or an automatic agent execution host.

## Baseline and evidence

The comparison uses **isorensen/hyprflow 0.2.1**, commit `efc96e9aa7bf96ad00706e77511a3daf99ca4c63`, dated March 11, 2026. Links below pin that source snapshot. The similarly named Ashrynne distraction-blocking project is outside this proposal.

Heimdall 0.7.0 already persists tasks, decisions, checkpoints, evidence, browser inventory, and browser operations. It does not yet persist a general native-window layout, application launch manifest, or herdr attachment binding. Its existing [store](../internal/store/store.go) uses a single writer, SQLite WAL, and `synchronous=FULL`; add recovery records to that foundation instead of introducing a parallel session-file authority. Storage settings alone do not prove end-to-end power-loss recovery.

Hyprflow supplies session capture and restoration, but its [saved schema][h-session] lacks Heimdall task/surface identities and operation revisions. Its [CLI][h-main] has no live workspace close, focus, or diff interface. A saved-session name does not establish task membership.

## Features and reusable parts to adopt

| Feature or source | Recommended disposition | Heimdall adaptation |
|---|---|---|
| Window and monitor capture, [capture.rs][h-capture] | Adopt the data requirements and mapping approach | Collect native window identity, workspace name, monitor, geometry, floating/fullscreen state, and observation coverage through a Hyprland adapter. |
| Session serialization, [session.rs][h-session] | Adapt the model; retain Heimdall storage | Add versioned workspace snapshots and desired manifests with task/surface IDs, source boundaries, and restore policies. |
| Autosave, rotation, and age filtering, [autosave.rs][h-autosave] and [session.rs][h-session] | Adopt the user-facing capabilities | Schedule capture through the daemon, expose freshness, retain useful restore points, and protect snapshots referenced by unfinished operations. |
| Application-specific launch preparation, [capture.rs][h-capture] | Adopt typed launch recipes | Use reviewed executable/argv/cwd recipes and application adapters. Separate reopening an app from resuming agent work. |
| Terminal process inspection, [process.rs][h-process] | Use as a bounded fallback | Prefer herdr's structured identity and process information. Treat process-derived cwd and command hints as observations requiring validation. |
| Browser profile handling, [brave.rs][h-brave] | Adopt explicit profile selection | Keep tab membership and ownership in Heimdall's existing browser protocol. Profile discovery does not prove which saved task owns a window. |
| Restore preview, [main.rs][h-main] | Adopt and extend | Produce a current desired-versus-observed diff, with reattach, launch, move, leave-open, unavailable, and review-required dispositions. |
| Mockable compositor/process interfaces, [hyprctl.rs][h-hyprctl] and [process.rs][h-process] | Reuse the testing pattern | Build deterministic Go fakes for identity races, missing apps, changed displays, interruptions, and verification failures. |
| Single-workspace restore | Add as initial Heimdall scope | Restore a selected task or explicitly selected set of tasks. This is future work in Hyprflow's [roadmap][h-todo], not a shipped feature to wrap. |
| Changed-monitor handling and tiled split reconstruction | Separate the priorities | Add visible monitor fallback to basic recovery. Defer exact split-tree reconstruction; both are open items in Hyprflow's [roadmap][h-todo]. |

Reuse can mean adapting an interface, field model, algorithm, or fixture rather than importing Rust code. If copying or translating code or fixtures, record the original commit and retain the applicable [MIT notice][h-license]. This proposal copies no implementation code.

### Behavior to replace rather than carry forward

Hyprflow's restore loop counts existing windows by class/workspace and discovers newly launched windows by address difference plus class. Heimdall needs verified surface identity and explicit adoption. Its dry-run bypasses duplicate detection; Heimdall's preview must use current observations. Restore results must also be verified independently: the inspected Brave path ignores a move error before reporting success. [Restore implementation][h-restore]

Do not use whole-desktop snapshots as implicit permission to manage every captured window. Do not infer an executable from an arbitrary window class, run captured command hints automatically, or persist a browser tab ID as a cross-restart identity. Keep the saved desired workspace separate from the current live inventory.

## Workstream 1: durable and sufficiently recent snapshots

### Records to add

| Record | Minimum content |
|---|---|
| Desired workspace manifest | Task ID, manifest revision, logical surface IDs, named workspace, desired membership, restore policy, reviewed launch/attach recipes, provenance, and acceptance event |
| Observed snapshot | Snapshot ID/version, manifest revision, source event boundary, capture interval, completeness/degradation, machine/boot/compositor identities, and observed surfaces |
| Native surface observation | Logical binding when known; runtime window identity; workspace name and current ID; application class; monitor reference; geometry; floating/fullscreen state; observation source |
| Display topology | Monitor identity, logical position and size, scale, transform, and capability/version information needed to interpret coordinates |
| Session binding | Application adapter, herdr session/workspace/pane identity when supported, canonical cwd/worktree, and optional editor or browser restore reference |
| Recovery operation | Operation/request ID, pinned manifest/snapshot, per-surface action intent, authorization/policy reference, attempts, observed outcomes, and remaining uncertainty |

Exact schemas remain implementation work. Keep runtime IDs scoped to their source epoch; keep logical surface IDs stable across restarts. Display geometry must declare its coordinate convention.

### Capture and persistence

- Bootstrap with a compositor snapshot and buffered events, then reconcile periodically. Detect source restarts, gaps, and out-of-order observations. Browser, herdr, and compositor snapshots are not one atomic desktop transaction; retain each source's boundary and disclose inconsistent capture.
- Combine debounced event-driven capture with a configurable maximum dirty interval. Start validation with a proposed 30-second maximum interval while healthy; measure the actual capture-to-commit gap and report missed targets. Accepted manifest changes commit immediately. Do not rely on a shutdown hook.
- Atomically commit each accepted snapshot and its projection/head through the existing writer. A half-written snapshot must not replace a valid restore point. Add schema fixtures, migration, and backup/restore coverage.
- Preserve the last complete restore point when the compositor is unavailable, startup inventory is incomplete, or shutdown produces an empty desktop. New observations can record that state without erasing desired membership.
- Retain manual restore points and bound automatic snapshot storage. Protect snapshots used by current manifests or unfinished recovery. Define retention for workspace records explicitly; do not silently delete the existing immutable task event history.
- Show last successful snapshot time, current age, source coverage, and capture failures. A five-minute-old snapshot is usable historical evidence, not a claim to contain the last five minutes of changes.

**Acceptance:** interrupt capture or commit and restart into either the previous complete snapshot or the newly committed one. Failed observations cannot erase desired surfaces. A controlled VM power-cut test must exercise the durable storage path separately from a daemon process-kill test. Document filesystem/storage assumptions and any unrecovered interval.

## Workstream 2: relaunch, reattach, and rebind identities

### Recover according to what survived

| Restart boundary | Required behavior |
|---|---|
| Heimdall daemon only | Rediscover current surfaces, reconcile unfinished operations, and avoid relaunching applications merely because the daemon restarted. |
| Compositor/session restart | Refresh compositor identity and inspect application/session survival. Reattach surviving services where supported; classify missing state explicitly. |
| Reboot or machine crash | Recreate approved application views. Reattach only sessions independently shown to have survived, such as a supported remote session. Treat local terminated processes as unavailable. |

### Restore protocol

1. Select a pinned manifest/snapshot and current authorization or configured restore policy. `Diff` compares it with fresh inventory. An age limit is a policy input, not proof that the snapshot is correct.
2. Resolve ownership, application capabilities, dependencies, and display mapping. Reserve resident-workspace capacity transactionally. Restore only explicitly selected tasks; never silently evict another workspace to make room.
3. Persist operation intent before any launch, move, attach, or close. Serialize conflicting operations on the same surfaces/workspaces. Cancellation stops new dispatch and reconciles actions already in flight.
4. Reattach a verified surviving session or launch a missing surface using a reviewed recipe. If another operation or the user creates a similar window, require a unique verified association before moving or adopting it.
5. Record the new runtime identity against the logical surface, with its new epoch and association evidence. Browser mapping retains the existing profile/epoch and nonce-pairing requirements; titles and duplicate URLs are insufficient.
6. On lost acknowledgement, observe before retrying. If an adapter cannot determine whether launch succeeded, retain an uncertain result. A durable request ID alone cannot make an external launch exactly once.

For herdr, validate installed-version create/list/attach/detach semantics and store actual assigned IDs. For LazyVim, restore supported editor session metadata when available and disclose missing buffers/session state. A generic terminal fallback may reopen a reviewed cwd but must report that prior processes were not resumed. Browser restoration must reconcile with the browser's own session recovery before creating additional tabs.

Complete `Close` as part of this work: save verified desired membership before closure, leave unowned windows alone, detach views without destroying supported herdr sessions, and preserve partial residency when an app refuses to close. Saved task context does not authorize sending a new prompt or restarting a previous shell command.

**Acceptance:** repeated open requests and daemon restarts do not create duplicate owned surfaces; same-class terminals for different tasks remain distinct; pane movement invalidates stale bindings; browser self-restoration is reconciled; interrupted launch/close remains visible and is never repeated by event replay.

## Workstream 3: verify the reconstructed workspace and report gaps

After each action, obtain a fresh observation and compare the actual result with the requested state. An API acknowledgement is one execution fact. It does not establish correct placement, successful attachment, or recovery of application data.

Verify, where supported: the intended surface exists, the logical binding is valid, workspace membership matches, display placement is usable, requested window state holds, the correct terminal session is attached, and browser profile/tab membership is correct. Use explicit set operations or observation-driven corrections instead of blind toggles. Check focus only when the request includes it.

Publish a per-surface report containing requested action, dispatch state, verification result, observed identity, retained error/uncertainty, and next recovery action. Aggregate workspace outcomes must distinguish full recovery, partial recovery, and inability to determine state. A recovered workspace does not imply its development task is complete.

Handle the following as normal recovery results:

- Missing executable, cwd, worktree, browser profile, or application restore data.
- Expired or missing herdr/agent sessions and unavailable remote hosts.
- Browser refusal, unsaved editor state, and applications that restore their own windows.
- Unplugged/reordered monitors, changed scaling or rotation, and unsupported layout fidelity.
- Compositor disconnection, a locked/unready desktop, failed verification, or ambiguous ownership.

For changed displays, preserve logical workspace/task membership first. Offer or use an explicitly configured fallback monitor, transform/clamp geometry into usable logical coordinates, and report the layout adjustment. Without an applicable policy, return a review-required plan instead of guessing. Initial recovery can restore useful tiled membership without claiming exact split ratios.

**Acceptance:** a failed move, wrong session attachment, or unknown observation cannot be reported as fully recovered. The terminal report identifies every unrecovered required surface and its cause. Repeat observation can settle uncertainty without automatically repeating execution.

## Proposed backlog additions and dependencies

The W identifiers below belong to this proposal. They elaborate M1b workspace work and reuse the C-series action infrastructure; they do not renumber or mark existing work complete.

| ID | Deliverable | Dependencies and roadmap connection | Completion evidence |
|---|---|---|---|
| W01 | Workspace manifests, surface identity, snapshots, and operation schemas | Existing store/authz; extend C01–C03 patterns; M1b | Migration/replay fixtures; unknown versions rejected; explicit desired/observed separation |
| W02 | Hyprland observation and task/surface bindings | W01; M1b observer | Named workspaces, monitor mapping, source epochs, event-gap reconciliation, and ambiguous-binding negatives |
| W03 | Durable autosnapshot, freshness, and retention | W01–W02 | Interrupted commits, capture outages, empty startup, retention references, and measured freshness |
| W04 | Live `List`/`Diff` and terminal recovery preview | W01–W03 | Deterministic current diff, scope isolation, staleness detection, and no preview side effects |
| W05 | Journaled open/focus/close and interrupted-operation reconciliation | W04 plus C12 action/verification contract | Capacity races, graceful refusal, uncertain launch, cancellation, and inert replay |
| W06 | Application adapters: herdr/terminal, browser, then editor state | W05; browser portion uses C13 acceptance; installed-host capability spikes | Reattach surviving sessions; new-epoch binding; browser recovery dedupe; explicit generic-terminal limits |
| W07 | Postcondition verification, monitor fallback, and partial-recovery report | W05–W06 | Missing app/session/display cases; wrong-result negatives; confirmed versus requested state |
| W08 | Optional login restore, packaging, and restart acceptance | W03–W07; M1b/M4 | One coordinator, readiness gates, manual override, and daemon/compositor/reboot/VM crash demonstrations |
| W09 | Optional Hyprflow snapshot import | W01 and W04; defer until migration demand exists | Pinned schema validation; explicit task/surface adoption; untrusted recipes never auto-execute |

C14–C16 retrieval and C20 agent execution-host support are not prerequisites for manual workspace recovery. Coordinate shared operation/lease primitives with C17; do not build a second competing journal, and do not wait for the entire agent-run feature set to deliver desktop recovery. C19 can later expose the same recovery state in the GUI; a readable CLI report is required first.

Recommended release increments:

1. **Saved workspace visibility:** W01–W04; durable snapshots and useful recovery previews.
2. **Manual workspace recovery:** W05–W07; restore selected tasks with verified partial results.
3. **Startup recovery:** W08; optional automatic restore under a saved policy after desktop readiness. Restore application views within that policy; agent execution remains a separately delegated capability.

When this proposal is scheduled, add the W items to the main backlog with the dependencies above and update M1b acceptance. Actual completion requires source, test, and deployment evidence in STATUS; this document does not provide that evidence.

## Release acceptance matrix

| Scenario | Required result |
|---|---|
| Daemon crashes during snapshot publication | Previous or new complete restore point remains usable; no partial head becomes authoritative. |
| Desktop shuts down while capture is active | Desired workspace is retained; missing shutdown observations do not erase it. |
| Daemon dies after launching an app, before receiving its result | Restart observes/reconciles the existing instance or reports uncertainty; no blind relaunch. |
| Two tasks contain same-class terminals or duplicate browser URLs | Identity remains task-specific; counts/titles cannot substitute for ownership. |
| User moves a window or herdr pane during recovery | Binding is revalidated; operation pauses or reports partial recovery when its assumptions no longer hold. |
| App refuses closure or contains unsaved state | Remaining surfaces are reported; no process-kill fallback. |
| Monitor order/scale changes or a display disappears | Logical membership is preserved; fallback placement is verified and disclosed, or review is required. |
| Browser restores itself; executable/session/worktree is missing | Existing tabs are reconciled and individual gaps are reported without a false full-success result. |
| Login recovery runs twice or races a manual request | One coordinator resolves the requests; no duplicate launch or capacity overrun. |
| Database replay or old snapshot inspection | Logical state only; no external actions. |
| Controlled VM power loss and subsequent cold boot | Last durable snapshot is recovered; lost capture interval and application-state limits are documented. |

Use fakes for deterministic races and disposable applications for live acceptance. Record the actual Hyprland, terminal, herdr, browser, editor, daemon, and schema versions. A process-kill test does not stand in for a power-cut test; a relaunched terminal does not prove process/session recovery.

## Deferred scope and adoption limits

Defer exact tiled split-tree restoration, arbitrary shell hooks, a general app-plugin framework, broad all-desktop ownership, and multi-machine desktop replication. WCU may assist explicit visual verification where a structured adapter lacks coverage, but screenshot matching must not become persistent ownership evidence. Preserve independent capability reporting.

If Hyprflow remains installed for personal desktop recovery, define which restorer owns each scope. Avoid running both restore mechanisms over the same windows at login. Importing a snapshot is optional and cannot expand Heimdall's authority by itself.

The prior Hyprflow evaluation was a source review, with no live restoration trial. Rust/Cargo and Go were unavailable on PATH during that review, so upstream tests and Heimdall's Go suite were not independently rerun. This document introduces no runtime behavior and no new dependency; the tests above are proposed acceptance work.

[h-main]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/main.rs
[h-capture]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/capture.rs
[h-session]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/session.rs
[h-restore]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/restore.rs
[h-hyprctl]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/hyprctl.rs
[h-process]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/process.rs
[h-brave]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/brave.rs
[h-autosave]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/src/autosave.rs
[h-todo]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/TODO.md
[h-license]: https://github.com/isorensen/hyprflow/blob/efc96e9aa7bf96ad00706e77511a3daf99ca4c63/LICENSE
