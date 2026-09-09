# C13 browser/compositor pairing fixtures

`events-v1.json` and `schema18.sql` come from the compiled schema-18 executable,
extension 0.5.0 and actual Chromium 151.0.7922.34 native messaging in an isolated
profile (`.tools/browser-pairing-test-LSuK2p`). The compositor is the synthetic
read-only IPC fixture. All tasks, pages and window metadata are synthetic.
No normal user browser profile, desktop inventory or credential is included.

The scenario pairs a newly opened browser window before navigation and then
explicitly reassociates its already-owned tab using a temporary marker. Both
phases, independent verification and viewport bindings survive pure replay.
The schema-17 predecessor fixture lives in `../browser-verification/schema17.sql`;
tests prove it gains no native association on upgrade.

Golden tests reject changed actors, duplicate nonce matches, missing probes,
wrong native identities and unissued continuation IDs without projection changes.
