# P01 compatibility fixtures

`requests.json` freezes two artifact observation requests (new identity and
explicit relocation) plus an artifact checkpoint request. The CLI takes the
nested input; HTTP takes the complete envelope. Schemas are
`schemas/artifact-request-v1.schema.json` and
`schemas/continuity-request-v2.schema.json`. Runtime additionally validates
duplicate keys, UTF-8 byte limits, sorted unique pins, scope and current heads.

`events-v1.json` was captured from the compiled `scripts/artifact-smoke.cjs`
run in `.tools/artifact-test-7C9PLR`. It contains two synthetic task/resource/
contract lineages, two artifact identities, three versions and two v3
checkpoints, including their command receipts. Only its scratch directory prefix
and host hashes were replaced with `/synthetic/heimdall-artifacts` and 64 zeros.
No endpoint credentials or file contents are present. Receipt hashes remain
those captured from the original requests; this is a reducer fixture, not input
for resubmitting those normalized requests. The reducer applies it from empty
state without accessing those paths, preserving historical pins through a move.

`schema8.sql` is an **actual stopped schema-8 database**, exported before P01
from the T03 Neovim/lazy acceptance run `.tools/neovim-test-tMMib3/data`.
Its schema-8 binary was retained as `.tools/heimdall-t03-schema8`, SHA-256
`06699e399c94f43b1d2399bad6e360d4b3aeed19c9a486926b1c51eac7f1ca32`.
The source passed `PRAGMA integrity_check` and was dumped read-only, with its
actual marker 8 appended. Its synthetic stopped Herdr binding and original
checkpoint/receipts are retained unchanged. It was not produced by relabelling
a newer database. Keep this SQL fixture frozen across future migrations.

SHA-256:

| File | Digest |
|---|---|
| `schema8.sql` | `4165cdc75eb7d79201265969026a6f2ef2d4c811010a4a043b47fe71865e5912` |
| `events-v1.json` | `2a0df42601f381e4442546231b8acdc4d3eef8284a50e6bbcfbe64f6cd11a869` |
| `requests.json` | `d4b69091c122aac611ce11b2580dfcf5f024b684e13b97dcd4eaf1f4de853a90` |

The migration test checks state, events, exact receipts, inert replay and the
pre-upgrade backup's original marker/projection/integrity. Compiled acceptance
separately creates a database with the saved schema-8 binary, upgrades to 9,
checks old-binary refusal and read-grant limits, and opens the rollback snapshot
with the original binary. A new artifact database also survives fresh-directory
backup restore with all original artifact files removed.
