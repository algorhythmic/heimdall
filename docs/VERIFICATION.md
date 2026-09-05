# Verification — MCP daemon 0.5.0, extension 0.2.0

Verified 2026-09-04 on Windows/amd64 with Go 1.27.1 and the pinned module graph.

## GitHub publication checks

The [first GitHub Actions run](https://github.com/algorhythmic/heimdall/actions/runs/33949655377) passed on Windows and Ubuntu for commit `222a4957bd8d608a9ca823f35d54e085a23cadad` (2026-09-04 Pacific / 2026-09-05 UTC). Both jobs ran Go tests, vet, build and all seven extension unit tests. Windows also built the executable and passed compiled continuity and MCP smoke checks from a fresh checkout. This establishes Linux CI test execution, not Linux desktop/native-host deployment acceptance.

The publication review checked the updated documentation links and Git whitespace, preserved original design-source hashes and dependency notices, and excluded generated binaries, scratch directories, databases and credential files. The initial import consists of an implementation commit and a documentation/progress commit; a subsequent documentation-only update records this CI result.

## MCP and checkpoint-write verification in 0.5.0

- Final checks passed: full Go suite, vet, Windows CGO-disabled build, Linux/amd64 cross-build, core compiled smoke and native-host smoke. The final stdio MCP scratch directory is `.tools/mcp-test-jSmjxP`. An actual 0.4.0 database with contracts/checkpoints and a read credential upgraded and restored successfully in `.tools/continuity-test-KWdkGe`; its existing credential still refused checkpoint writes after upgrade. All test processes exited.
- Official Go MCP SDK v1.7.0 is pinned; the Go baseline rises to 1.25. Module checksums and dependency root notices are retained. Tests use an official SDK client negotiating protocol 2026-07-28 through the adapter and scoped HTTP daemon.
- The compiled executable passes stdio initialization/discovery and all four tools using legacy 2025-11-25. Checks cover read/write separation, authenticated grant provenance, exact retries, stale-head conflict, wrong-project denial, the sole-writer lock, daemon loss/restart, replay, revocation after restart, unchanged task completion and absence of tokens from events. Reproduce with `node scripts/mcp-smoke.cjs`.
- Store tests verify authorization before cached receipt lookup and rollback when authority is lost before commit. Domain tests verify expired/revoked/cross-grant retries, unauthorized contract acceptance and unchanged command bodies. Golden events include v1/v2 grants and CLI/client checkpoints, with unknown payload versions rejected.
- The adapter bounds stdio lines at 128 KiB and tool inputs at 64 KiB; daemon responses retain the 512 KiB cap. Explicit zero limits are not silently replaced with defaults. The daemon alone reads/writes its database, and replay never calls MCP or observes resources.

User-host registration, Linux desktop/native-host deployment and race-detector acceptance remain unverified. Remote CI results are recorded above. Progress summaries remain claims; evidence evaluation, Braid, the task GUI and automatic execution are not claimed.

## Scoped-read verification in 0.4.0

Final checks: full Go tests and vet passed; Windows and Linux/amd64 CGO-disabled builds succeeded. Linux is cross-build-only. Core and native-host compiled smoke checks passed. The actual 0.2.0 upgrade/rollback also passed in .tools/continuity-test-hFP4Sx.

- Full Go tests pass, including daemon-level grant issuance/revocation and client route tests. They cover wrong-project, sibling and ancestor denial; forged/duplicate authority fields; credential class separation; expiry; revoked/replayed/restarted grants; pagination size and cross-scope/cross-principal cursor rejection; changed-snapshot refusal; and missing observation permission.
- Version-2 contracts reject unreviewed resource IDs and checkpoints reject changed contract scope. Version-1 contracts remain replayable. Standalone golden persisted events cover both contract versions, decisions, resources, checkpoints, grants and revocation without filesystem reads.
- The compiled 0.3.0 executable created an actual old contract/checkpoint fixture. Build 0.4.0 preserved it, reported unreviewed scope, accepted an explicitly reviewed replacement, and retained replay/restart equality. Its pre-schema-4 snapshot restored successfully with 0.3.0. Inspected scratch directory: `.tools/continuity-test-Ein3yY`.
- Compiled CLI checks exercised private credential creation, exact issuance retry, authorized task/context/history reads, wrong-project refusal, credential-free public endpoint rediscovery after restart, revocation, and denial after retrying a revoked issuance. Raw tokens were absent from event output. Backup/restore/replay checks also passed.
- Reproduce with `node scripts/continuity-smoke.cjs .tools/legacy/heimdall-0.3.0.exe --legacy-continuity` when that archived executable is available. The default script uses a fresh current-version store and also runs in the Windows CI job; remote CI has not been executed here.

This slice does not implement MCP, client checkpoint/progress writes, proposed/rejected decision review, Git identity or the task GUI. Read grants are an application/API boundary; they do not isolate a local process that can independently read the unrestricted CLI credential. See [SCOPED-ACCESS.md](SCOPED-ACCESS.md) for restore/revocation semantics and limits.

## Continuity verification in 0.3.0

- Go tests pass for the new continuity service and all existing packages; `go vet ./...` passes. Request fixtures cover all five operations, unknown/duplicate fields, unsupported versions and invalid revision preconditions.
- Service tests cover exact retries after resources disappear, replay equality, eight competing checkpoint writes with one accepted head, parent contract drift, file drift, binding removal, task revision conflicts, browser/observer authority rejection and explicit insufficient context budgets.
- Windows canonical-root tests require access to parent directories that the development sandbox restricts; these tests and the compiled continuity smoke passed with normal filesystem access. Symlink refusal is tested when Windows permits fixture creation; the test explicitly skips on hosts without that permission.
- The actual archived 0.2.0 executable created the compiled upgrade fixture. The new binary retained its task state, created a pre-marker-3 backup and rejected old-binary reuse. Restoring that pre-upgrade snapshot into a fresh directory worked with 0.2.0.
- The compiled continuity CLI passed contract/resource/checkpoint commands, exact retry, context drift, small-budget refusal, replay, forced-stop/restart, exclusive live backup and fresh-directory restore/replay. Reproduce with `node scripts/continuity-smoke.cjs [path-to-old-executable]`. Final inspected scratch directory: `.tools/continuity-test-KxW8Yk`; historical get-by-ID also passed.
- Rebuilt Windows and Linux/amd64 CGO-disabled artifacts; Linux remains cross-build-only. Existing core compiled smoke, native-host compiled smoke and all seven extension unit tests pass against this iteration. Browser worker code and packaging were not changed; the prior real-Chromium checks below describe 0.2.0 acceptance, not a new normal-profile registration check.

No MCP, GUI, Braid consumer, verified evidence evaluator or execution-host acceptance is claimed. Full persisted-event golden fixtures, scoped/paginated reads and remaining contract/decision details remain in the backlog. Backup is database-only and retains user content; it excludes endpoint files but is not a redacted export.

## Earlier core/browser acceptance

The following checks were performed for 0.2.0; core/native regressions noted above were rerun for 0.3.0.

| Check | Outcome |
|---|---|
| `go test ./...` | Passed for CLI, browser protocol/service, capture, core, daemon, model, native bridge and store |
| `go vet ./...` | Passed |
| Windows `CGO_ENABLED=0 go build -trimpath` | Succeeded |
| Linux/amd64 `CGO_ENABLED=0` cross-build | Succeeded; not executed on Linux |
| Compiled Windows binary smoke | Passed: init, daemon start, import/add/update, capture, step completion, proposal acceptance, replay, forced-stop/restart |
| Replay and restart | Serialized state byte-identical in fixture and compiled-binary checks |
| File edit race | Concurrently recreated task path preserved; original retained; pending view reported |
| Writer lock | Second writer rejected; lock released by process termination |
| Completion negatives | Empty aggregates, stale proposals, repeated rejected evidence, and silence without mail coverage cannot complete tasks |
| Input/authentication | Invalid YAML/schema/cycles/anchors rejected; token, origin, Host and unknown request fields checked |
| Browser credentials | Browser token rejected from task/state/control APIs; CLI token rejected at browser ingress |
| Native framing | Bounded frames, truncated input, short writes, exact caller origin and browser-only proxy route checked |
| Browser lifecycle | Explicit pairing, no unpaired metadata retention, retries, stale sequence/connection/epoch rejection, unowned-tab refusal, cancellation on unpair, expiry and late results, replay equality |
| Extension unit tests | Seven Node tests passed: URL/privacy/size filtering, duplicate/interrupted operations, stale/expired/paused/unowned targets, URL changes and partial API failures |
| Real Chromium extension | MV3 worker loads with expected ID; popup renders; actual open, inventory, navigate, focus, move and close APIs pass; IndexedDB persistence and stale URL refusal checked |
| Compiled native smoke | Prepared helper executes over framed stdio; handshake/pairing, inventory, CLI reverse command/result, rotated credentials after daemon restart and replay pass |
| Worker integration | Real Chromium worker + actual native framing + compiled daemon: automatic inventory, open/focus/close, pause/resume, offline IndexedDB buffering, reconnect after daemon restart pass. OS native-host discovery is replaced by a test port. |
| Database upgrade | Legacy projection/event retention and replay pass on marker 1→2 upgrade. Previous core executable explicitly rejects a migrated test database with `unsupported database schema version 2`. |
| Braid source | Compared against preimplementation source fingerprints; unchanged |

The compiled-binary smoke used the synthetic fixture in a fresh temporary directory and left no daemon running afterward. Reproduce with `scripts/smoke.ps1`; it preserves its scratch directory for inspection.

Native-host registration in a normal Chrome/Edge profile, Web Store packaging/signing, Linux desktop deployment, Hyprland, herdr, conversation content, mail, agent hooks, inference, Braid consumer integration, and the race detector remain unverified. The Linux artifact is a cross-build, not a deployment acceptance result. See [STATUS.md](STATUS.md) before treating any target-spec capability as implemented.

Browser checks use Node 24.14.0 and isolated Playwright Chromium 151.0.7922.34. They do not register a native host or load the extension in the user's normal profile. Test processes close in `finally`; scratch directories remain available for inspection. Reproduce `node --test extension/test/*.test.js`, `node scripts/native-smoke.cjs`, `node scripts/browser-smoke.cjs`, and `node scripts/worker-smoke.cjs` from the project root. The last two require Playwright and its Chromium runtime available in the development environment. Native/worker smoke scripts target the Windows build. `scripts/package-extension.ps1` emits the unpacked-extension ZIP without test files.

Build SHA-256 hashes for the local development artifacts are recorded in [build-checksums.txt](build-checksums.txt). Remote CI success is recorded above; CI builds are not published release artifacts and are not covered by those local hashes.
