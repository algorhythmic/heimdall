# Workspace List, Diff and preview

W04 is a read-only comparison of current desired surfaces, a selected retained
workspace point, and fresh observations. It works on the W02 Hyprland 0.56.2
source and W03 snapshots. The preview slice landed at schema 15; current schema is 21. No preview emits events,
captures a point, changes a binding, reports Herdr metadata or dispatches an
application action. Existing scoped/MCP/browser credentials have no access.

```bash
./bin/heimdall workspace list TASK --data-dir DATA
./bin/heimdall workspace diff TASK --data-dir DATA
./bin/heimdall workspace preview TASK --manifest MANIFEST_ID --snapshot SNAPSHOT_ID \
  --output /tmp/workspace-preview.json --data-dir DATA
./bin/heimdall workspace validate TASK --file /tmp/workspace-preview.json --data-dir DATA
```

`list` returns only the exact task's desired surfaces and explicitly owned live
windows. `diff` selects the current manifest and snapshot head once and reports
their IDs. Missing inputs remain visible. `preview` requires explicit immutable
IDs; the manifest must still be current and belong to the task. A historical
snapshot can precede that manifest, making added/removed surfaces visible.
Use `--json` for complete machine-readable results. `--output` creates a new
private file and refuses overwrite. Preview files contain scoped layout and
identity metadata; they contain no CLI credential.

The selected point must remain the current head or explicitly pinned to avoid
`snapshot_not_pinned_or_current`. A retained but unprotected point can still be
inspected, but its rows require review. A pruned point supplies metadata only.
No read implicitly pins it. Use the explicit [snapshot commands](SNAPSHOT-SETUP.md)
to capture or protect a point. Age is always shown. The optional
`--max-snapshot-age-seconds N` adds an explicit age policy; zero imposes no age
limit because an unchanged workspace may have an old valid point.

| Disposition | Meaning and limits |
|---|---|
| `leave-open` | An owned window matches saved placement, or a surface was removed from desired membership. Removed and unowned windows are left alone. This does not verify application content or pane attachment. |
| `move` | A currently owned window differs in supported workspace/display or floating/window state. A future operation must recheck ownership and verify placement. |
| `reattach` | The owned window is missing and a current Herdr binding was independently re-observed. A reviewed attach recipe, unique new window association and attachment verification are still required. |
| `launch` | The explicitly owned window is absent in the same source epoch. A reviewed executable/argv/cwd recipe, new-instance association and application verification are required. No executable is inferred from a title, PID or class. |
| `unavailable` | Fresh consistent source coverage is unavailable. The last good inventory is not presented as current. |
| `review-required` | Missing/stale bindings, missing saved layout, browser pairing, unprotected/old points, changed displays, unsupported tiled geometry or other uncertainty needs resolution. |

All dispositions are proposals. Neither `review_required=false` nor successful
validation grants execution authority or reports full recovery. C12/W05/W06/W07
provide journaled actions, reviewed recipes and verified outcomes later.

Workspace/monitor names must resolve uniquely. Numeric IDs are not reused as
identity across compositor epochs. Changed scale, transform, position, dimensions,
missing displays and moved workspace/display associations require review; W04
does not guess a fallback display. Floating coordinates are logical global
coordinates; physical monitor dimensions are adjusted for scale and transform.
The full display rectangle is checked, but reserved work areas, decorations,
tiled split trees and application usability are not verified.

Only current explicit bindings authorize an observed window. A saved window
reassigned to another task is left open with an opaque ownership warning; no
foreign identity or live metadata appears. Titles, classes and PIDs are omitted
from the comparison and its digest. Unowned windows in relevant workspaces are
counted without details. Windows owned by other tasks are excluded from that
count. Ordinary title similarity, cwd, pane similarity or browser self-restore
never creates ownership. Browser surfaces require C13's exact profile/window
association. Unowned Heimdall UI/radiator/pairing surfaces receive no actions.

Freshness uses the existing double inventory with buffered, unsequenced event
hints. The result records its capture interval and known gaps; it cannot prove
an atomic compositor snapshot or detect silent upstream event loss. Herdr
observations use four concurrent slots and one three-second deadline, after the
bounded compositor read. All external I/O occurs outside the task writer.
Relevant task, source, ownership and retained-point inputs are rechecked after
observation; a concurrent change refuses the mixed result.

Previews expire after 30 seconds. A daemon-lifetime HMAC seals the complete
artifact, including its deadline and proposed rows. Validation rejects modified,
expired, clock-reversed and previous-daemon artifacts, then observes again and
recomputes the scoped semantic digest. Observation timestamps, elapsed point age
and repaired-gap counters do not invalidate an otherwise unchanged comparison;
an explicit age threshold does. Relevant ownership, display or placement changes
do invalidate it. A successful validation is a new observation, not a lock on
the desktop. Recreate a preview after restart or expiry.

In the TUI, `p` opens this comparison. `r` refreshes it, `b` opens explicit Herdr
binding, and `s` opens a capture confirmation. Enter captures/pins a fresh point
using the displayed input preconditions and the retained exact-request journal.
Partial capture preserves the last complete head. Escape cancels the
confirmation; opening or refreshing the dialog does not capture anything.

Acceptance includes deterministic planner and stale/forged/expired/restart
tests, cross-task negatives, read-only IPC auditing, unchanged durable state,
display/epoch/identity failures, and compiled CLI/TUI/PTY workflows. No live
application movement, launch, close, reboot recovery or VM power loss is claimed.

W05 now consumes an explicitly submitted, current preview for [journaled workspace operations](WORKSPACE-OPERATIONS.md). The comparison itself remains read-only. The TUI adds `o` open/focus and `c` graceful-close confirmations.
