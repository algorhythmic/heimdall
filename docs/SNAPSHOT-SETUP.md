# Durable workspace snapshots (W03)

W03 saves immutable payloads for explicitly bound task surfaces in the same SQLite
database as their events. Manual points, current heads, explicit autosave policies,
retention and diagnostics are available through the local CLI. Snapshot capture
performs no application action. Recovery preview and execution follow in W04–W07.

Start with [Hyprland source selection and viewport binding](HYPRLAND-SETUP.md).
The live adapter supports local Hyprland 0.56.2. Historical snapshot inspection,
replay and database backup work without a compositor, including on Windows.
Existing browser, scoped client and MCP credentials gain no snapshot access.

## Save a manual point

```sh
./bin/heimdall snapshot status project
```

Copy the returned task revision, manifest/source IDs, `input_digest`, and current
head ID into a saved request. Use `previous: "none"` before the first complete
point; otherwise use `head.id`. Assign a new 32-character lowercase hex request ID.

```json
{
  "version": 1,
  "id": "11111111111111111111111111111111",
  "op": "capture",
  "target": "project",
  "previous": "REPLACE_WITH_HEAD_ID_OR_none",
  "expected_task_revision": 1,
  "manifest_id": "REPLACE_WITH_MANIFEST_ID",
  "source_id": "REPLACE_WITH_SOURCE_ID",
  "input_digest": "REPLACE_WITH_INPUT_DIGEST"
}
```

```sh
./bin/heimdall snapshot capture project --file capture.json
./bin/heimdall snapshot list project
./bin/heimdall snapshot show project --id SNAPSHOT_ID
```

The daemon observes outside the task writer, then rechecks source freshness,
current task/manifest/source/binding identity and the prior head inside the commit.
Changing a binding or task while the read is pending refuses publication. Exact
retries return the original receipt without observing again or restoring a pin.
Keep the original request file after an uncertain response.

A manual capture is pinned automatically. A `complete` point requires every
surface in the manifest to have an explicitly owned, currently observed window,
and at least one surface. It atomically becomes the target's current head. A fresh
but incomplete manual capture is recorded as `partial`, stays pinned for inspection,
and never replaces the last complete head. Unavailable source coverage refuses a
capture entirely. Partial points do not silently remove desired membership.

Each payload records the capture start/end interval, read method and known event-gap count. These are bounded observations from an unsequenced, non-atomic source, not complete event delivery guarantees.

Payloads contain only the selected task's surfaces and their referenced workspace
names, plus display geometry. Unowned window titles and unrelated workspace names
are excluded. Terminal session IDs remain declared references; compositor snapshots
do not prove pane attachment or save terminal scrollback/process state. File bytes,
editor buffers/undo, browser profile state and unsaved application content are not
captured. No executable is inferred from a title or class.

## Enable automatic capture explicitly

Autosave is disabled until a policy is accepted for a task. Copy current revision,
manifest and source IDs from status, and the previous policy ID if one exists:

```json
{
  "version": 1,
  "id": "22222222222222222222222222222222",
  "op": "policy",
  "target": "project",
  "previous": "none",
  "expected_task_revision": 1,
  "manifest_id": "REPLACE_WITH_MANIFEST_ID",
  "source_id": "REPLACE_WITH_SOURCE_ID",
  "policy": {
    "enabled": true,
    "debounce_seconds": 2,
    "max_dirty_seconds": 30,
    "retain_count": 32
  }
}
```

```sh
./bin/heimdall snapshot policy project --file policy.json
./bin/heimdall snapshot status project
```

The daemon checks the shared observer cache once per second. A changed complete
workspace is captured after the configured quiet period, or after the maximum
dirty interval while changes continue. The default example starts at 30 seconds;
limits are debounce 1–30 seconds, maximum dirty 5–300 seconds and at least the
debounce interval. Capture requires fresh, consistent source coverage; outage,
continuous inconsistent readback, or a stale policy may exceed the configured
interval and is reported explicitly. It is not a real-time deadline guarantee.

Unchanged content creates no snapshot event. Status separates last observation,
last attempted publication, dirty time, capture issue, saved-point age, and retained
head payload availability. A successful read is not a new saved point. Window
metadata and geometry changes count as content changes; focus and unowned titles
do not change the task payload.

Policies pin their accepted task revision, manifest and source selection. A changed
revision/manifest/source makes the policy stale; review and accept a new policy.
To stop automatic capture, submit a new policy envelope with `enabled: false`,
`previous` equal to the current policy ID, and the current task revision. Keep the
other policy fields explicit. Stopping the observer also makes capture unavailable.
Neither operation deletes prior points. Restart restores accepted policies, but
waits for fresh observations before publishing anything. Shutdown never publishes
an empty replacement.

## Retention and manual pins

`retain_count` is the recent history allowance (2–256), with explicit pins retained
in addition. The current complete head is always protected. Manual points are
pinned automatically; up to 64 active pins per task and 32 enabled task policies
are supported. A manual partial point does not consume the rolling history allowance.
Retention runs as part of new capture publication; lowering a policy's allowance
takes effect on the next capture. Explicit pruning is available immediately.

To release a manual point, save this envelope using its current pin ID from status:

```json
{
  "version": 1,
  "id": "33333333333333333333333333333333",
  "op": "unpin",
  "target": "project",
  "previous": "REPLACE_WITH_PIN_ID",
  "snapshot_id": "REPLACE_WITH_SNAPSHOT_ID",
  "reason": "This manual point no longer needs retention"
}
```

Run `snapshot unpin project --file unpin.json`. To pin an available historical
point, use `op: "pin"`, a new request ID, and `previous: "none"`. Pinning cannot
resurrect a pruned or damaged payload. To prune released history explicitly:

```json
{
  "version": 1,
  "id": "44444444444444444444444444444444",
  "op": "prune",
  "target": "project",
  "previous": "none",
  "snapshot_ids": ["REPLACE_WITH_SNAPSHOT_ID"],
  "reason": "Discard unneeded historical payloads"
}
```

Run `snapshot prune project --file prune.json`. A protected current head or pin
refuses the entire operation. At most 256 payloads are pruned in one record. The
same transaction appends a retention event and removes the selected payloads;
metadata remains identifiable with `pruned: true`. The payload cannot be used for
restoration. Replay preserves that unavailability and never recaptures a desktop.

Current heads preserve the point's accepted manifest reference. Recovery operations
do not exist yet; C12/W05 must protect their referenced snapshots through the shared
retention predicate before dispatch and release protection only after completion.
No claim of protecting a running recovery operation is made by this stage.

Each payload is limited to 384 KiB, and retained payload bytes are capped at 128 MiB
per database. New publication is refused if retention cannot free enough space,
for example because points remain pinned. Payload pruning does not erase events,
command receipts, metadata history, or confidential content already present in a
backup. It is retention, not secure erasure. Use `snapshot list --limit N --before
EVENT_ID` to page backward using the last item's `event_id`.

## Storage cost and interruption boundaries

Large payloads are stored once in `workspace_snapshot_payloads`. Small historical
metadata has an indexed table; the JSON task projection contains only current
heads, policies and active pins. Payload, metadata, event, receipt and head publish
through the same writer transaction. Backup includes retained payloads. Schema 15
preserves all earlier event versions and authority, makes a pre-schema-15 backup,
and is refused by schema-14 binaries; see [backup/rollback instructions](CONTINUITY-SETUP.md#backup-upgrade-and-restore).

A local Linux/Btrfs measurement simulated 2,880 captures (24 continuously dirty
hours at 30 seconds) with eight surfaces and 16 retained points. The projection grew
by 810 bytes; retained payloads used 129,728 bytes. Writer p50/p95/p99 were
2.93/4.20/7.13 ms (maximum 125.74 ms), mean point lookup was 0.20 ms, and replay was
660 ms. The database reached 14.95 MB because events, receipts and metadata remain
historical. This is roughly 5 MB for eight continuously dirty hours per comparable
workspace; idle unchanged workspaces add no snapshot history. Other hosts, larger
task histories and multiple active workspaces need their own measurement. The
[raw Btrfs report](benchmarks/w03-snapshot-linux-btrfs.json) and
[tmpfs smoke measurement](benchmarks/w03-snapshot-linux-tmpfs.json) record the conditions.

Process-kill tests at payload insertion, before commit and after commit pass on
Btrfs. Missing/corrupted payload, failed projection write and SQLite allocation
failure tests preserve the last committed point. A compiled synthetic compositor
workflow verifies empty-source restart, exact retries, pruning, and backup restore.
These tests do not simulate power loss to a VM or physical storage controller.
No VM runtime is installed in this development environment; controlled VM power
loss and actual compositor/reboot recovery remain the later release acceptance
gates in [the roadmap](design/history/REVISED-ROADMAP-IMPLEMENTATION-PLAN.md).

Reproduce the volume and commit-failure checks on the desired filesystem:

```sh
mkdir -p .tools/snapshot-disk-tests
TMPDIR="$PWD/.tools/snapshot-disk-tests" HEIMDALL_SNAPSHOT_VOLUME=2880 \
  go test ./internal/store -run 'TestSnapshotDailyVolume|TestSnapshotProcessKillPublicationBoundaries|TestSnapshotAtomicFailuresAndCorruption' -count=1 -v
node scripts/snapshot-smoke.cjs
```

This is saved workspace visibility. A payload's presence does not establish that
an application can be relaunched or that a recovered workspace has been verified.
