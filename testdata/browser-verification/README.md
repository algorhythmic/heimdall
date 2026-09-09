# Browser verification fixtures

All tasks, pages and profiles are synthetic. No real browser profile, token,
website account, user document or desktop inventory is retained here.

- `schema16.sql`: SQLite dump from the actual C12 executable's
  `.tools/actions-test-C2rBbZ/schema16.db`. It contains a legacy browser operation
  and an unverified shared action with cancellation/uncertainty and a late result.
  Its schema marker is explicitly retained at 16.
- `schema17.sql` and `events-v1.json`: actual Linux Chromium 151.0.7922.34 native
  host acceptance from `.tools/browser-verification-test-dtVUZi`. The isolated
  profile exercised challenged readback, open/navigation/focus/move/close,
  duplicate URLs, an exact-redirect mismatch, daemon loss after a tab opened and
  retained-result recovery. A before-unload close remains uncertain/unknown.
  Localhost ports are historical fixture data, not services required by replay.

Event envelopes remain v1. C13 action transitions use payload v2; challenge and
readback payloads use v1. Golden tests derive verification again, reject forged
provenance/references/outcomes, and check the event cursor. Replay performs no
browser/native-host I/O. These fixtures do not claim compositor association,
normal-profile installation, Windows execution or browser/reboot restoration.
