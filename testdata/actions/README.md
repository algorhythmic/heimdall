# Shared action compatibility and conformance

- `schema15.sql` is a stopped database from the actual W04/schema-15 binary
  (`8bc43799c257dbecd2762cf86ecea50d3167984ec303f9d3902c25958855dfff`),
  created by `.tools/actions-test-ukz1Jq/schema15.db`. Its single legacy browser
  success has no invented task or verification record after upgrade.
- `schema16.sql`, `events-v1.json` and `requests-v1.json` come from the compiled
  synthetic action workflow in `.tools/actions-test-MD4ndS`. They retain a queued
  intent, delivery, daemon interruption, in-flight cancellation and late API
  report. The final execution is API-reported; verification remains pending and
  earlier uncertainty/cancellation remains visible.
- `wire-v1.json` is shared by Go and browser-JavaScript conformance tests. The
  matching TypeScript declarations describe execution/verification separately
  and preserve task/manifest/surface/action/attempt/digest identity.

These fixtures contain only synthetic tasks, browser identities and URLs. No
real browser tab content, CLI credential, native socket or executable recipe is
included. Event replay must not contact a browser or create a new attempt.
