# Review verification — 2026-09-04

Environment: Windows/amd64; project-local `go1.27.1`. No live Linux desktop was available.

| Check | Result |
|---|---|
| Braid `go test ./...` | Passed, CLI and library; cached results |
| Real source `go run ./cmd/braid serve --stdio` | Passed via `verify-braid.ps1`, independent temporary database |
| Upsert and query | Correct task ID, one item, explicit cost 8, explanation present |
| Malformed request | Error response; subsequent valid query succeeds |
| Repeated query at fixed `at` | Result bytes identical after excluding echoed request ID |
| Replay after direct upsert | Query returns no nodes, confirming the integration caveat |
| Supplied originals | SHA-256 matches preserved copies |
| Three example YAML files | Parsed successfully; task IDs/types/statuses, parent cycles, subtask references/cycles, completion kinds/modes checked |
| Local documentation links | Resolved successfully in the packet audit |

Source handoff SHA-256: `9D6EB87D7578779391713AD3BED37776BE84598C4D19316E7A33D1C9D7E16432`.

Source discussion SHA-256: `A33C1BCF418F87A20427D8084890BDA8A1879E28DE87C0933C49CB8F5D00309A`.

Scratch directory for the successful protocol test: `%TEMP%/heimdall-braid-contract-421878b6f0414b5e89aa8d4da6b37e48`. No existing Braid database was used. The script retains its scratch directory for inspection and creates a fresh one on every run.

These checks establish the inspected Braid transport behavior. They do not establish retrieval accuracy, a working Heimdall implementation, browser content compatibility, desktop-agent parity, or compositor/mail correctness. Those acceptance gates remain in [IMPLEMENTATION.md](IMPLEMENTATION.md).

The example audit is a temporary review helper, not Heimdall's future strict schema validator. Full unknown-key, date, template-transition, reference-scope, and event-payload validation is still implementation work. Braid source fingerprints are recorded in `BRAID-SOURCE.sha256` for comparison before implementation if the concurrent project changes.
