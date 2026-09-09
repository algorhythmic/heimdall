# Read-only Hyprland observation (W02)

W02 supports local Linux Hyprland **0.56.2**, using its command and event sockets.
It records explicit source selections and task/surface window bindings. Live
inventories stay in bounded daemon memory. They are observations, not saved
restoration points. [W03 durable snapshots](SNAPSHOT-SETUP.md) now consume these observations; W04 recovery previews follow.

Build and start the daemon using [terminal setup](CONTINUITY-SETUP.md). Commands
below use `./bin/heimdall`; add `--data-dir DIR` when using a separate store.
Nothing is installed into the shell path or desktop configuration.

## Select a source

Inspect the explicitly chosen socket directory:

```sh
./bin/heimdall viewport probe --socket-dir "$XDG_RUNTIME_DIR/hypr/$HYPRLAND_INSTANCE_SIGNATURE"
./bin/heimdall viewport status
```

`probe` subscribes before reading two matching inventories. It does not select
or persist the inventory. Copy its `snapshot.host` and `snapshot.source_epoch`
into a saved `select.json` envelope. Use a new 32-character lowercase hex ID and
`previous: "none"` initially, or the exact `source_head` from status thereafter:

```json
{
  "version": 1,
  "id": "11111111111111111111111111111111",
  "op": "select",
  "previous": "none",
  "source": {
    "socket_dir": "/run/user/1000/hypr/REPLACE_WITH_SELECTED_INSTANCE",
    "host": "REPLACE_WITH_PROBED_HOST_HASH",
    "epoch": "REPLACE_WITH_PROBED_SOURCE_EPOCH"
  }
}
```

```sh
./bin/heimdall viewport select --file select.json
./bin/heimdall viewport inventory
```

The daemon reopens only this recorded source after its own restart. A compositor
restart, replaced socket, or different boot changes the epoch and requires a new
explicit probe/selection. It never switches to a new inherited environment value.

Selection permits a background read-only event subscription and inventory
reconciliation. Events trigger a debounced read, at most once per second; quiet
sources reconcile every five seconds. Each capture has a three-second budget.
`status` reads cached diagnostics; `inventory` requests a fresh global inventory.
These are unrestricted local CLI routes. Existing scoped clients, browser
credentials and MCP grants gain no desktop visibility or action authority.

## Bind an existing window

First accept a [desired workspace](WORKSPACE-SETUP.md) containing a stable
terminal, editor or native surface. From fresh inventory, select the exact
`identity` of the intended window. Class, title, PID and workspace names are
labels, never matching authority. Duplicate titles remain separate windows.

Save `bind.json` with the current task revision, manifest, source selection,
snapshot digest, desired surface and exact window identity:

```json
{
  "version": 1,
  "id": "22222222222222222222222222222222",
  "op": "bind",
  "previous": "none",
  "target": "project",
  "expected_task_revision": 1,
  "binding": {
    "manifest_id": "REPLACE_WITH_MANIFEST_ID",
    "surface_id": "REPLACE_WITH_SURFACE_ID",
    "source_id": "REPLACE_WITH_SOURCE_HEAD",
    "snapshot_id": "REPLACE_WITH_INVENTORY_SNAPSHOT_ID",
    "window": {
      "source_epoch": "REPLACE_WITH_SOURCE_EPOCH",
      "stable_id": "REPLACE_WITH_WINDOW_STABLE_ID"
    }
  }
}
```

```sh
./bin/heimdall viewport bind project --file bind.json
./bin/heimdall viewport list project
./bin/heimdall viewport list project --cached
```

The bind command reobserves the source and rejects changed inputs. Keep the
original request file for exact retries after an uncertain response. A retry
returns its original historical receipt; it does not bind a replacement window.
A task view returns only windows explicitly bound to that task's desired surfaces.
Unknown matches stay `unowned`. Bound windows are `observed`, `missing`, or
`unavailable`; task/manifest changes are separate issues.

For a terminal, optional `binding.session_binding_id` joins the current explicit
session binding on the **same surface**. It is a declaration link, not proof that
the window displays that pane. A moved/rebound pane invalidates that link visibly;
`session refresh` remains the separate Herdr liveness check. No class/title/cwd
matching joins Herdr panes to native windows.

Direct CLI browser bindings remain refused; use [C13 nonce/profile/window pairing](BROWSER-PAIRING.md). A
native window binding does not identify a browser profile or authorize tab actions.

To unbind, save a new envelope with `op: "unbind"`, `previous` equal to the binding
head, the current task revision, and `binding` containing only `manifest_id` and
`surface_id`. A surface cannot be removed from its manifest while still bound.
To stop observation, save a new envelope with only version, ID, `op: "stop"`, and
`previous` equal to `source_head`; run `viewport stop --file stop.json`.

## Identity, coverage and limits

The adapter verifies canonical same-user socket paths, non-writable group/other
permissions, peer credentials, boot and process start, and both socket generations.
The source epoch hashes these inputs. Window identity combines that epoch with
Hyprland's compositor-owned `stableId`, a monotonically assigned 64-bit counter
for each window object. Reusing a memory address cannot inherit the old binding.
Surviving windows can be rediscovered after a daemon restart on the same source.

The protocol has no sequence numbers or atomic desktop snapshot. Known stream
EOF, malformed/overlong frames and failed readback make coverage unavailable.
Two matching full inventories and a quiet observed event boundary establish a
fresh observation, not a proof that every upstream event arrived. Periodic
reconciliation repairs silently missed observable changes. Continuous changes may
prevent a fresh capture; no stale cache is promoted to fresh. Cached observations
expire after ten seconds. Last good data remains available in diagnostics while
freshness is false, but task views do not expose it as currently observed.

Limits are 256 mapped windows, 128 workspaces, 32 monitors, 2 MiB per native reply,
64 KiB per event line, 512-byte sanitized window titles/classes, and 384 KiB per
normalized snapshot. Duplicate IDs/addresses, unsupported versions, missing stable
IDs and inconsistent topology are refused. An empty monitor inventory is unavailable.

Window positions and sizes use Hyprland global logical coordinates. Monitor
width/height are physical pixels; scale and transform are retained separately.
Named/special workspace IDs and monitor mappings remain explicit. W02 issues no
focus, move, close, launch, restoration or filesystem-write command to applications.

The pinned implementation sources are
[HyprCtl window JSON](https://github.com/hyprwm/Hyprland/blob/v0.56.2/src/debug/HyprCtl.cpp)
and [window ID allocation](https://github.com/hyprwm/Hyprland/blob/v0.56.2/src/desktop/view/Window.cpp).
The [official IPC contract](https://wiki.hypr.land/IPC/) describes the event socket.

## Persistence and validation

Source selections and viewport bindings were introduced in schema 14; the current schema 15 adds durable snapshots. It preserves
old grants, receipts and task completion authority. Upgrade first publishes a
consistent `backups/pre-schema-15-*.db`; schema-14 binaries refuse migrated data.
Replay does not connect to a compositor. Restore the pre-upgrade backup into a
fresh stopped directory for rollback; see [backup instructions](CONTINUITY-SETUP.md#backup-upgrade-and-restore).

Run `go test ./...` and `node scripts/viewport-smoke.cjs`. The latter uses a
synthetic same-user compositor on Linux, and checks explicit unsupported-platform
behavior elsewhere. The optional `HEIMDALL_TEST_HYPRLAND_DIR` environment variable
enables `TestNativeReadOnlyMetadata`; it prints counts only. Current verification
covers local read-only Hyprland 0.56.2 observation and synthetic disconnect,
reused-address, moved-window/pane, restart, replay and backup cases. It does not
claim actual compositor restart, reboot recovery or native window manipulation.
