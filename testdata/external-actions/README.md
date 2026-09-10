# S2a schema-20 predecessor

`schema20.sql` is the synthetic W05 schema-19 fixture from
`../workspace-operations/schema19.sql`, upgraded by an actual binary built
from P0 commit `5e0c790`, then dumped using SQLite. Its 99 events and receipts
are unchanged. There is no real desktop inventory, credential or user content.

The stopped-fixture test upgrades it to schema 21, verifies the pre-upgrade
backup, preserves event/receipt bytes, and compares replay with projection.
Actual-binary acceptance also checks that P0 refuses marker 21 and successfully
opens a restored pre-schema-21 backup at marker 20 in a fresh directory.
