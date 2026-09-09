# W03 snapshot compatibility fixtures

- `schema14.sql`: stopped database from the actual W02 binary's compiled fixture,
  `.tools/viewport-test-PgpAif/snapshot.db`; source/binding records are synthetic.
- `schema15-retained.sql`: compiled W03 fixture with five historical points, two
  pruned payloads, three retained payloads, a current head, policy and manual pin.
  It comes from `.tools/snapshot-test-BHM7o7/snapshot.db`. Replay and backup tests
  preserve all retained payloads and historical unavailability without IPC.
- `events-v1.json`: matching event metadata; large desktop payloads are deliberately
  absent from events and live in the dedicated SQL payload table.
- `requests-v1.json`: saved CLI capture/policy/unpin/prune/partial-capture envelopes.
  They contain only synthetic task/window/source identifiers and temporary paths.

Payload and event family version 1 are introduced under schema 15. Historical
metadata identity/availability is enforced by the SQL index plus event reducer;
only current heads, active pins and latest policies belong in the JSON projection.
The synthetic source directory no longer exists and must not be consulted by
replay. No real desktop window title or application content is in these fixtures.
