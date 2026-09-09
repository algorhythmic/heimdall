# Journaled workspace operations (W05)

W05 adds explicit workspace `open`, `focus`, and `close`, with status,
cancellation, and observation-only reconciliation. It coordinates the existing
shared action journal. [W06 application recipes](APPLICATION-RECOVERY.md) now add
terminal launch, Herdr direct attach/detach, paired-browser operations and saved-file
Neovim views. Unsupported or unverified application state remains explicit.

The supported native baseline is Linux Hyprland **0.56.2**, commit
`efb50993780079460b0cbed1363e2166a2de1d9f`. Both `hyprlang` and `lua`
configuration providers have fixed dispatcher encodings. Windows retains the
records and replay behavior but has no native desktop dispatcher.

## Use

Select a desktop source, accept a task manifest, bind the exact owned windows,
and save a point as described in [Hyprland setup](HYPRLAND-SETUP.md) and
[snapshots](SNAPSHOT-SETUP.md). Review a fresh comparison, then explicitly submit
the selected surfaces within its 30-second review lifetime:

```sh
./bin/heimdall workspace diff alpha --output /tmp/alpha-review.json
./bin/heimdall workspace open alpha --file /tmp/alpha-review.json --surfaces all
./bin/heimdall workspace operations alpha
./bin/heimdall workspace operation alpha --id OPERATION_ID
./bin/heimdall action show alpha --id ACTION_ID
```

`--surfaces ID,ID` selects a subset. `open` focuses surviving owned views and can
recreate missing views with an explicitly reviewed application recipe. `focus`
requests focus. Neither operation claims complete layout or application-state recovery.

```sh
./bin/heimdall workspace close alpha --dry-run
./bin/heimdall workspace diff alpha --output /tmp/alpha-close.json
./bin/heimdall workspace close alpha --file /tmp/alpha-close.json --surfaces all
./bin/heimdall workspace cancel alpha --operation OPERATION_ID --expected-revision N --reason 'Stop new input'
./bin/heimdall workspace reconcile alpha --operation OPERATION_ID --expected-revision N --reason 'Observe the existing attempt'
```

Use the global `--request-id ID` to preserve a CLI retry identity. Repeating the
same accepted request returns its original receipt without observing or acting.
An expired, changed, forged, or previous-daemon preview requires fresh review for
a new operation. Read-only preview is not dispatch authority.

In the TUI, `p` opens the workspace comparison. `o` reviews an open/focus request,
`c` reviews graceful close, and Enter submits the frozen request. Escape cancels
the confirmation. `r` refreshes the comparison and recent operation outcomes;
`s` saves a point. Cancellation, reconciliation, and explicit swaps are available
through the CLI. Retained TUI retries use the original request ID and body.

## Capacity and swaps

The default resident limit is three. A fresh census covers every active owned
viewport binding, including tasks that were already open before W05. Existing
overflow is represented rather than evicted. Missing coverage, old source
identities, and unresolved actions retain capacity. Focus and close remain
available when already over the limit.

Open reserves capacity in the same transaction as its intent and action IDs.
To replace a resident, independently review and name that outgoing task:

```sh
./bin/heimdall workspace diff alpha --output /tmp/incoming.json
./bin/heimdall workspace diff beta --output /tmp/outgoing.json
./bin/heimdall workspace open alpha --file /tmp/incoming.json --surfaces all \
  --swap beta --swap-file /tmp/outgoing.json
```

The operation contains both scopes. It closes all reviewed outgoing surfaces
before permitting incoming input. A close ACK cannot transfer the slot: fresh
complete same-epoch absence and settled outgoing attempts are required. A
refusing or unsupported outgoing application stops incoming input and keeps
the remaining resident. No resident is selected automatically.

## Close and interruption

Before native input, one transaction stores the operation, one
`workspace.diffed` record, a scoped operation snapshot, and queued shared action
intents. Complete capture publishes a new head; partial/empty capture preserves
the last complete head. Unfinished operations protect the selected points and
their close snapshot from pruning. Desired manifests are preserved.

W05 graceful close supports `native` surfaces without session bindings. W06 adds
explicit terminal close policies, observed Herdr-session preservation and scoped
browser tab closure. Editor windows remain open to preserve unsaved buffers. Unowned
windows are left open. There is no process kill, arbitrary command, or
active-window fallback.

Preparation performs authenticated socket and inventory reads outside the
writer. It opens a single-use, two-second prepared connection. A checked
transaction then persists `dispatching` before dispatcher bytes can be sent.
The 30-second action deadline bounds new delivery; a prepared connection or
possibly dispatched attempt is never retried automatically.

Receipt facts (`submitted`, `acknowledged`) and verification are separate.
Focus uses two `activewindow` queries alongside double inventory, never focus
history rank. Close compares complete fresh same-epoch inventory with the
exact bound stable ID. Up to eight subsequent readbacks allow an application
to process a graceful request; they send no input. An observed refusal leaves
partial residency. Lost ACK/restart remains an uncertain execution history;
later exact absence can verify closure without changing that history.
`workspace.closed` is emitted only for observed closure of the reviewed scope.
Application data preservation and task completion are never inferred.

Cancellation stops new dispatch and retains in-flight effects for observation.
Reconciliation resets the bounded readback budget for the same attempts; it
cannot create an attempt or repeat input. Replay, status, and backup restoration
perform no desktop actions. Source loss requires explicit reselection/review;
old identities never migrate silently to a new compositor.

## Protocol and verification

W05's schema 19 added operation/residency records, action intent v3 and native
transition v4, plus operation snapshot metadata v2. Browser intent v1/v2 and
their histories remain unchanged. W06's schema 20 adds reviewed recipes and
application actions; see [application recovery](APPLICATION-RECOVERY.md) and the
[operation request schema](../schemas/workspace-operation-request-v1.schema.json).

The fixed encodings follow the pinned upstream
[Lua dispatcher implementation](https://github.com/hyprwm/Hyprland/blob/efb50993780079460b0cbed1363e2166a2de1d9f/src/config/lua/bindings/LuaBindingsDispatchers.cpp),
[explicit selector resolution](https://github.com/hyprwm/Hyprland/blob/efb50993780079460b0cbed1363e2166a2de1d9f/src/config/lua/bindings/LuaBindingsInternal.cpp),
and [native close action](https://github.com/hyprwm/Hyprland/blob/efb50993780079460b0cbed1363e2166a2de1d9f/src/config/shared/actions/ConfigActions.cpp).
Only fixed focus/close/move constructors are encoded; there is no general Lua
or shell input API. Move requires an existing uniquely named workspace and a
restricted name alphabet. The W05 user flow does not yet request placement.

Tests cover capacity/swap races, stale/forged reviews, immutable capture before
input, refusal, cancellation, lost ACK, no retry after restart, independent
focus/absence, and inert replay. Compiled Linux acceptance uses isolated Unix
sockets and kills the daemon after a synthetic close before ACK. An opt-in
native test verifies a disposable GTK window on the actual selected Hyprland
session. W06 acceptance additionally covers real application launch, Herdr
detach and browser duplicate prevention. Full attachment/display recovery,
reboot and VM power-loss gates remain with W07–W08.
