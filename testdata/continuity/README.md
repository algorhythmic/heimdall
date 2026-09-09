# Continuity compatibility fixtures

`schema6.sql` is a stopped SQLite dump created through the actual Heimdall
`60175d1` (0.7.0) CLI/daemon on Linux with Go 1.27.1, before the resume feature
was added. It contains two synthetic tasks, a reviewed empty resource scope,
an accepted decision, a checkpoint and a read-only grant. Commands use fixed
request IDs 1–6, formatted as 32 hexadecimal digits, at September 8, 2026,
18:00 UTC. The synthetic grant expires September 20, 2026, 18:00 UTC.

The dump was produced using Python SQLite's `iterdump()` after graceful daemon
shutdown and a successful `integrity_check`; its schema marker was explicitly
included in the SQL export. It contains no live credentials, absolute worktree
paths or retained user content. The randomly generated test credential was
discarded; only its verifier is present. Fixture SHA-256:

    d060a9fb7291d929ae4d99492e5d24d8e6a7f7fc12db3019526a6d0b63edcc09

`internal/store/schema6_fixture_test.go` loads this dump into a temporary
database, opens it through the current store, verifies continuity and grant
boundaries, and checks replay and command receipt retention. Schema-7 tests also verify a consistent pre-upgrade marker-6 backup and exact event/receipt preservation. Keep this fixture
unchanged as database migrations are added; changing a current projection's
schema marker is not equivalent to testing this historical baseline.

`events-v1.json` and `requests.json` retain their existing event/wire conformance
roles. They are independent of this stopped-database fixture.
