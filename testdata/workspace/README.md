# Workspace version-1 fixtures

`requests.json` contains one synthetic example of each operation:
`workspace.accept`, `session.bind`, `session.unbind`. The CLI takes the nested
`manifest` or `session` input; HTTP takes the complete envelope. The public shape
is in `schemas/workspace-request-v1.schema.json`; Go additionally validates byte
limits, path syntax, state ownership and revision/head constraints.

`events-v1.json` freezes the corresponding payloads and event envelopes. The
reducer test seeds task `alpha` at revision 1 before applying these domain events.
Production transactions also include `command.accepted` receipts; service and
compiled tests exercise their exact preservation through replay and restart.
All paths and runtime identities are synthetic declarations and need not exist.

The actual stopped 0.7.0 database fixture remains in
`testdata/continuity/schema6.sql`. Its migration test verifies marker 7,
pre-upgrade marker-6 backup integrity, original events, exact receipts and
unchanged read-grant permissions. Do not regenerate it with a newer binary.

`herdr-requests.json` and `events-v2.json` freeze the initial T02 bind/publish inputs and observed session payload. Runtime reads stay separate from stored observations.

`schema7.sql` is a stopped dump of the W01 compiled workspace smoke restore, created before T02, with actual v1 manifests/bindings and immutable receipts. Its generating W01 binary SHA-256 was `a93c38e7ac6b45473d40e8460a22eddfefb1e4016b38b16bbfdd373e4bef6c31`. The SQL dump SHA-256 is `1a57bafebc2d4357f2569054bdf607e4e4df5a3c9369840db506f19012733ec6`. The source was stopped, passed SQLite integrity checking, and was exported with its actual marker 7. It contains only synthetic data and no endpoint credentials. Retain it unchanged through future migrations.
