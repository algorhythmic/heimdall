# P04 compatibility fixtures

`schema12.sql` is the actual stopped synthetic P03 acceptance snapshot from
`.tools/preservation-test-KnCBTp`, captured before schema 13. It retains all
checkpoint/preservation facts and exact receipts. Paths identify synthetic
originals/clones; replay must not access them. There are no live credentials or
retained file contents.

`requests.json` freezes v1 dependency add/remove envelopes. `events-v1.json` is the
compiled P04 fixture from `.tools/dependency-test-Mycg5u`, containing cross-project
relations, removal, completion/reopen and a revoked synthetic read grant. Tests
reject forged versions, actors, revisions, cycles and chains; historical task
state and exact receipts remain replayable.
