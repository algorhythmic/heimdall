# Heimdall in an Omarchy development workflow

Assessment date: September 8, 2026.

Scope: Heimdall's fit for a developer using terminal, herdr, and LazyVim as the primary interfaces to agents and their outputs; integration with Wayland Computer Use (WCU); and planning or other non-implementation progress across projects using dotprivate.

Status: source assessment and recommendations. This document does not change the implementation backlog or record acceptance of a new development scope.

## Assessment

**Heimdall has a credible foundation for preserving work across projects and agent sessions. Its biggest opportunity is making that foundation accessible through terminal, herdr, and LazyVim.** The current implementation delivers more of the underlying state management than the everyday integration.

The answers to all three questions are yes, with a distinction between implemented capabilities and proposed integrations. Productivity improvements remain hypotheses to validate in the actual workflow.

| Question | Assessment |
|---|---|
| Can Heimdall improve productivity in this Omarchy environment? | Strong architectural fit; incomplete operational fit. Prioritize task binding, readable resume context, and progress capture through the existing developer surfaces. |
| Can Heimdall use and enhance computer use? | Yes. Connect WCU's immediate observations and execution outcomes to Heimdall's durable task state, evidence, and recovery history. |
| Can Heimdall track planning and non-implementation progress with dotprivate? | Yes. Heimdall can record the meaning of progress while dotprivate preserves selected supporting artifacts. |

## 1. Productivity through terminal, herdr, and LazyVim

Heimdall is especially relevant when switching projects, replacing an agent session, or returning to unfinished work.

### What already exists

| Implemented capability | Benefit in this workflow |
|---|---|
| Accepted objectives, constraints, and decisions | A replacement agent can recover the agreed direction. |
| Immutable checkpoints with next actions and blockers | Work can resume without reconstructing progress from terminal scrollback. |
| Resource snapshots and drift detection | Heimdall can flag changes since a checkpoint. |
| Scoped MCP reads and checkpoint writes | Agents can retrieve context and save progress through their existing host. |
| Revisioned, editable `tasks.yaml` | Task editing already fits a LazyVim workflow. |
| Artifact, repository, and test evidence | Completion review can use observed results. |

These capabilities are implemented in the [continuity model](../internal/model/continuity.go), [MCP adapter](../internal/mcpbridge/server.go), and [evidence subsystem](EVIDENCE-SETUP.md). Resource observations retain digests and metadata; they do not preserve or return the underlying file contents.

### The operational gap

Heimdall currently lacks herdr integration, automatic agent-session capture, terminal workspace restoration, ranked planning, and an execution-host adapter. Linux/Hyprland deployment also remains unverified in its recorded acceptance status. The [design anticipates these integrations](design/HANDOFF-heimdall-v1.1.md), but the [implementation status](STATUS.md) defines current readiness.

The highest-value additions for this workflow are:

1. **Explicit task-to-session binding.** Connect a task to its repository/worktree, herdr session/workspace/pane, and agent session. Reconcile pane movements and restarts. Directory names or inherited environment variables alone are insufficient, especially when multiple tasks share a repository.
2. **A short terminal resume view.** Show accepted direction, the latest checkpoint, changed resources, blockers, and the next action. JSON is useful for adapters; the developer needs a readable view.
3. **A thin LazyVim integration.** Provide task selection, checkpoint capture, artifact opening, and completion review through familiar editor commands. Neovim exposes a structured API suitable for this integration. [Neovim API documentation](https://neovim.io/doc/user/api/)
4. **Progress displayed inside herdr.** The installed CLI exposes workspace/pane metadata reporting, agent inspection, output reads, and waits. Heimdall could publish task name, next action, and review-needed state while herdr continues reporting agent runtime state. These commands were inspected through help output; a live integration was not tested.

An agent can be idle while its task still needs review, and a task can be blocked while an agent works on an alternative. Preserve those distinct meanings in the interface.

**Recommendation:** prioritize a terminal integration and checkpoint workflow ahead of further GUI expansion for this developer environment. Measure time to resume a project, repeated context supplied to agents, and the manual effort required to keep progress current.

## 2. Integration with WCU and computer use generally

### Responsibilities

| Component | Recommended responsibility |
|---|---|
| Heimdall | Objectives, accepted decisions, durable progress, evidence, and recovery history |
| herdr | Terminal sessions, panes, agent lifecycle, and terminal output |
| WCU | Fresh desktop observations, guarded input, and immediate outcome checks |
| Braid | Optional retrieval of relevant reference material and validated procedures |
| dotprivate | Preservation and synchronization of selected supporting artifacts |

WCU already offers observations, guarded actions, bounded sequences, and outcome waits. At the reviewed snapshot, its uncommitted working tree also contained a bounded context adapter, an optional Braid adapter, and a request-local execution ledger. The source was ahead of its implementation handoff document. See the [WCU README](../../wayland-computer-use/README.md), [context adapter](../../wayland-computer-use/scripts/cu/context_retrieval.py), and [execution ledger](../../wayland-computer-use/scripts/cu/execution.py).

The ledger distinguishes submitted input, partial or uncertain execution, verification, and unattempted steps. It is request-local state; it does not itself provide durable cross-session recovery.

### Integration opportunities

**Persist action outcomes across sessions.** Heimdall could retain WCU results under the relevant task and checkpoint. Its current checkpoint schema has no structured action/run links. Records should identify the task, action, adapter/runtime, target, observations, execution outcome, and verification outcome.

**Recover interrupted workflows.** Record an action's intent before dispatch and its result afterward. If the host disappears between them, preserve an uncertain outcome and reconcile it through fresh observation. Replaying Heimdall's event log must never repeat desktop input. Durable intent recording does not establish exactly-once execution.

**Connect interface outcomes to task evidence.** A WCU observation might establish that a dialog closed; a filesystem evaluator might establish that the expected export exists. Heimdall can connect those facts to the task's acceptance criteria. Neither a successful tool response nor an arbitrary repaint establishes task completion.

**Provide task context to computer use.** Heimdall supplies the objective, constraints, accepted decisions, and next subtask. WCU supplies current window and action context. A resumed session needs fresh observations: saved frame IDs and desktop identities cannot serve as continuing input authority. WCU's standalone observer revisions are also distinct from its input frame IDs.

**Coordinate shared-desktop access.** If several tasks eventually dispatch computer use, Heimdall will need a desktop input lease and cancellation/reconciliation handling. That coordinates cooperating agents; WCU's immediate input guards remain necessary, and the user can still change the shared desktop.

For terminal and editor operations, use structured interfaces wherever possible: herdr's API for panes and agents, Neovim's API for editor state, and Hyprland IPC for window/workspace events. WCU covers visual interactions and applications that lack a suitable interface. [Hyprland IPC documentation](https://wiki.hypr.land/ipc/)

### Authority and implementation boundary

Heimdall's existing MCP clients can save checkpoints under an explicit grant, but cannot submit independently trusted execution evidence. Integration needs a narrowly scoped adapter identity and action-result contract. A checkpoint saying that a click succeeded remains an agent report.

This fits the existing [capability model](CAPABILITY-LEDGER.md) and planned shared action/verification work, C12 onward, in the [backlog](BACKLOG.md). A task's saved context does not grant execution authority, and checkpoint access must not become a shortcut to unrestricted CLI credentials.

The first useful integration can capture and review WCU outcomes during manually initiated work. Automatic dispatch and continuation can follow after interruption recovery is reliable.

## 3. Planning and non-implementation progress with dotprivate

This is a natural application of Heimdall's existing model. Checkpoints, decisions, blockers, and acceptance criteria apply to research, design, evaluation, and implementation. Heimdall already includes `research` and `study` workflows, plus an application workflow with research and drafting steps. [Default workflows](../internal/core/defaults/types.yaml)

Dotprivate supplies an artifact layer: specifications, experiment reports, debugging notes, and other files can remain outside the project's committed history while being preserved in a private repository. Its current implementation records file mirroring and synchronization; it does not record the meaning of progress. [dotprivate README](../../dotprivate/README.md)

For example, Heimdall could record:

> WCU integration: research complete; design awaiting review. The comparison report is saved. The unresolved decision is which observations qualify as completion evidence. Next action: review the proposed evidence contract.

The report lives in a project or dotprivate location. Heimdall holds the task state, decision history, checkpoint, and a versioned reference to the report.

### Recommended additions

| Addition | Purpose |
|---|---|
| Artifact references | Preserve stable project/artifact identity, relative path, environment, digest, and Git revision when available. |
| Explicit progress semantics | Distinguish draft, reviewed, accepted, superseded, blocked, and completed. File existence alone does not establish substantive review. |
| Cross-project views | Show latest meaningful progress, unresolved decisions, next actions, and explicit dependencies such as “Heimdall integration awaits a WCU capability.” |
| Checkpoint-triggered artifact preservation | Give planning sessions a save/ingest opportunity even when no code commit occurs. |

Accepted decisions and checkpoints already provide part of this foundation. Proposed/rejected decision review, structured artifact references, and cross-project dependency handling need further implementation. Keep these recommendations distinct from current support.

### Storage and synchronization details

- Dotprivate's post-commit hook runs `sync`, which synchronizes the `.private` clone. Refreshing mirrored originals requires `ingest`. Planning sessions therefore need an explicit preservation trigger independent of code commits.
- Dotprivate retains mirrors of missing originals. An importer needs explicit stale/deleted handling instead of treating every retained file as current. See the [ingest and sync implementation](../../dotprivate/dotprivate).
- Keep Heimdall's live SQLite database local and authoritative. Dotprivate can hold supporting artifacts and portable progress exports. Synchronizing the live database requires a separate replication design.
- Bind selected private artifacts explicitly. Heimdall automatically excludes `.git` from tree observations, but does not automatically exclude `.private`. Broad bindings can introduce unnecessary observation work and unrelated drift. [Resource handling](../internal/continuity/resources.go)
- Dotprivate's private remote and sparse checkout do not supply encryption or per-project authorization. Heimdall's scoped access decisions must remain independent of what a clone can materialize.

## Recommended delivery order

| Priority | Deliverable | Practical validation |
|---|---|---|
| 1 | Terminal/herdr/LazyVim task binding and resume views | Switch away from a project, return in a new agent session, and recover direction and next action with less manual context reconstruction. |
| 2 | Planning checkpoints and artifact references | Save a research or design session without a code commit; recover the current artifact, unresolved decisions, and next action across projects. |
| 3 | Durable WCU action evidence and interruption recovery | Interrupt work after input but before acknowledgement; retain uncertainty, observe current state, and avoid duplicate input. |
| Later | Automatic continuation | Validate host capabilities, authority, cancellation, leases, and recovery before dispatching work automatically. |

Use the manual workflow to establish value first. Track resume time, repeated instructions, lost or stale decisions, progress-maintenance effort, and recovery correctness. WCU task evaluations can additionally measure tool calls, latency, and outcome verification. No general productivity or computer-use speedup was established by this assessment.

## Evidence and verification limits

- Heimdall was reviewed at commit `60175d1`, the documented 0.7.0 development checkpoint. Source, tests, design, capability boundaries, and backlog were inspected.
- WCU was reviewed from its local working tree, including uncommitted context and execution changes. Those findings describe the inspected source, not a published or connected plugin release.
- Eight WCU context-contract tests and four execution-contract tests passed during the assessment:

  ```sh
  PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -p 'test_context_contract.py' -v
  PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -p 'test_execution_contract.py' -v
  ```

  These focused tests do not establish live desktop acceptance, a working Braid deployment, or end-to-end Heimdall integration.
- Dotprivate's README and implementation were inspected. Its synchronization flow was not executed.
- The installed herdr CLI's help and subcommand help were inspected. No live panes, agents, or workspaces were changed.
- Go was unavailable on PATH, so Heimdall's Go suite was not independently rerun. Existing CI and platform results are repository-reported evidence.
- The assessment made no code or configuration changes. This report is the resulting documentation artifact.

Links to WCU and dotprivate source assume sibling checkouts under the same parent directory as Heimdall.
