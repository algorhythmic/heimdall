# P02 compatibility fixtures

- `schema9.sql` is a stopped dump of the actual P01 synthetic artifact smoke
  snapshot from `.tools/artifact-test-7C9PLR/snapshot.db`, captured before the
  schema-10 implementation. It contains P01 artifacts, checkpoint pins, commands
  and exact receipts, with marker 9. Synthetic paths are retained as historical
  identity; replay must not access them. The source files no longer exist.
- `events-v1.json` was captured from the compiled P02 smoke in
  `.tools/progress-test-DK2bCb`. It includes draft/reviewed/accepted/rejected
  decisions, decision supersession, artifact acceptance and version replacement.
  It contains no credentials or retained file contents. Accepted decision
  projection is derived from the accepted progress review, not an added field in
  the legacy `decision.accepted` payload.
- `requests.json` freezes representative decision, artifact and review request
  envelopes for `schemas/progress-request-v1.schema.json`.

Store tests preserve schema-9 events, projections and exact receipts through
upgrade, compare the pre-upgrade backup, and replay all event families without
filesystem observation. The actual stopped schema-6/7/8 fixtures remain in their
original directories and are tested through marker 11.

- `schema10.sql` is the stopped actual P02 CLI smoke database from
  `.tools/progress-test-DK2bCb/snapshot.db`, captured before adding GUI review.
  It preserves v1 proposal/review events and exact receipts under marker 10.
- `events-v2.json` is the compiled GUI fixture from
  `.tools/progress-gui-test-fq80V0`. It includes UI-attributed decision acceptance,
  stale decision rejection and exact artifact acceptance. It retains only the
  non-secret session authority snapshot; no cookie, CSRF token or bootstrap code
  appears in it. Replay succeeds after the session and original files disappear.
