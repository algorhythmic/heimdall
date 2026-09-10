# Terminal interface design — September 8, 2026

Design baseline, not a current capability ledger. The table records the original
UI slice; later workspace recovery is delivered. Use [TUI setup](../guides/TUI-SETUP.md),
[workspace operations](../guides/WORKSPACE-OPERATIONS.md) and [status](../STATUS.md)
for current behavior. The original screenshot files are not retained in this checkout.

This replaces the previous card-based browser interface with the user-provided
terminal design in `screenshot-2026-09-08_22-52-14.png` and
`screenshot-2026-09-08_22-57-43.png`.

The main hierarchy is **needs you → workstreams → selected context**. Attention
items are actionable records, distinct from ordinary status. The table exposes
next action, status, resume-by, saved age, step completion and recorded sessions.
The selected context keeps accepted direction, saved work, next action, files and
bindings together. Amber marks focus/actions, green marks observed success and
active state, red marks review/failure, and muted gray marks secondary context.
Status words accompany color.

Dialogs preserve the target and original preconditions. Step inspection shows
recorded checks with uncertainty visible. Save-progress uses a durable draft and
fixed request identity. Planning review binds exact proposal/artifact digests.
Workspace preview distinguishes desired membership from available fresh session
observations. It never invents a verified recovery outcome.

| Reference element | Current implementation |
|---|---|
| Needs-you queue | Completion and planning proposals, unfiled captures, missing/stale saved progress. |
| Workstream table | Resume-by and saved ordering, explicit task/step IDs, expandable hierarchy, find and keyboard focus. |
| Selected context | Accepted direction, checkpoint/next action/blockers, resource drift, recorded session bindings and exact-owner browser observations (tier and focus history). |
| Browser attention | Latest paired-profile browser span plus an explicit `g` active read; no background poll or focus-based binding. |
| Step dialog | Criteria, check provenance, live acceptance revalidation, explicit rejection/reopen and evaluator execution. |
| Save dialog | Retained editable checkpoint draft, original pins/heads, external editor and exact retry. |
| Desktop dialog | Desired membership plus fresh checks for already-bound Herdr sessions; recovery remains unavailable. |
| Bar/popup reference | Responsive compact terminal overview; no installed native bar module. |
| Agent working/blocked/idle and jump | Not implemented by this UI slice; recorded bindings are labeled accordingly. |
| Automatic file pinning/preservation | Existing exact pins are retained; new versions and preservation require their own explicit workflows. |

Implementation uses [tcell v2](https://github.com/gdamore/tcell/tree/v2.13.10) for
portable terminal input, cell rendering, Unicode width and terminal restoration.
The application handles its own layout and state transitions. There is no HTML,
CSS, browser session or frontend runtime in the task interface.
