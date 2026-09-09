# W02 compatibility fixtures

- `schema13.sql`: stopped schema-13 database created by the actual P04 binary in
  `.tools/dependency-test-EcPGbR/snapshot.db`. Contains synthetic tasks and scoped
  grant hashes only. Upgrade tests preserve history, state, receipts and backup.
- `events-v1.json`: compiled W02 synthetic compositor selection and explicit
  viewport binding in `.tools/viewport-test-gOuK1G`. Replay must not open its now
  absent temporary socket directory. No real desktop title or window is retained.
- `requests-v1.json`: full select/bind/unbind/stop envelopes from the synthetic
  compiled smoke. They demonstrate immutable request IDs and explicit heads;
  they are fixtures, not commands for a live desktop.

Source and binding records use version 1 under database marker 14. Existing
workspace/session records keep their versions. Inventories are not persisted by
this stage. Native read-only metadata verification reports counts only; it does
not update these fixtures from the user's desktop.
