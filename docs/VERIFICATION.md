# Verification — terminal/editor continuity, initial W01/T02/P01/P02/P03/P04/W02, schema 14

## W02 read-only Hyprland observation — 2026-09-09

- Full Go tests and vet pass with Go 1.27.1. Race checks pass for the Hyprland observer, workspace service, store and daemon. Linux build and Windows/amd64 cross-build pass; Windows was not executed locally. Linux SHA-256: `947cee4df3c5e0c2f4be1d402e2ab83477dfc88ec73b38cdc59996b4fc2515e0`; Windows: `d3b703571655dd6aa753d722d08fef2616e23fb780a92ecec91f6ae43a517ed5`.
- Unit and golden-event tests cover duplicate title/class, native stable-ID/address ambiguity, named workspace movement, address reuse, changed/rebound pane declarations, stale task/manifest/source heads, browser pairing refusal, immutable ownership and exact retries. Forged actor/version/head/scope/epoch records and duplicate events are refused without state changes; the event cursor advances.
- Event tests cover buffered bootstrap changes, malformed and overlong frames, disconnect/reconnect, source epoch change, inconsistent/empty/oversize inventory, title/control bounds, and periodic reconciliation after a silently omitted event. Coverage explicitly remains unsequenced and non-atomic. A malformed-frame test exposed an open-stream cleanup bug, fixed by closing the event connection when its reader ends. Title truncation is bounded to the retained prefix even for large native replies.
- Installed read-only IPC probe passed on Hyprland 0.56.2, commit `efb50993780079460b0cbed1363e2166a2de1d9f`: two monitors, two workspaces, nine mapped windows. Only counts were printed. No window was focused, moved, closed, launched, or automatically assigned. Peer/socket-generation tests also verify replacement by the same process changes identity and the wire vocabulary contains only four read commands.
- Final compiled acceptance passed in `.tools/viewport-test-PgpAif`: synthetic same-user compositor IPC, explicit selection/binding, movement, event gap recovery, browser/scoped credential denial, SIGKILL/restart on the same source, inert replay, exact receipts, backup restore, address reuse and explicit unbind/stop. The original golden event fixture came from `.tools/viewport-test-gOuK1G`; both use only synthetic tasks/windows.
- Actual retained schema-13 binary upgrade/refusal/rollback passed in `.tools/continuity-test-kdeUgb`, with old-grant authority preserved. Stopped schema-6 through schema-13 fixtures preserve projections, events and exact receipts. Dependency regression passed in `.tools/dependency-test-2oE63b`; TUI snapshots and real-PTY regression passed in `.tools/tui-test-ajur1T`.
- P04 commit `8a9593d` passed both published CI jobs ([run 34324113854](https://github.com/algorhythmic/heimdall/actions/runs/34324113854)). W02 inventories are bounded daemon memory, not durable restore points. Browser profile pairing, actual compositor restart/reboot recovery and native window manipulation are not claimed. Next is W03 durable autosnapshot storage and its failure/retention/volume gates.

Current R0/T01–T03 and initial W01/T02/P01/P02/P03/P04 work was verified locally on Linux/amd64. Historical results below retain their original milestone scope. Published checks run on Windows and Ubuntu; [GitHub Actions](https://github.com/algorhythmic/heimdall/actions) records subsequent runs.

## P04 task dependencies and scoped summaries — 2026-09-09

- Full Go tests and vet pass with Go 1.27.1. Race checks pass for continuity, core, store, daemon and TUI. Linux build and Windows/amd64 cross-build pass; Windows was compiled, not executed locally. Linux SHA-256: `d13ccfcdfe9dd10c06e8e069be96c0e79ae542fdab7edb383a68bd56a5215808`; Windows: `f310671a14d6db5d0b68c5f3bd02e805fceb693121f6e8a99a4792da246e4341`.
- Tests cover direct/transitive dependency cycles, endpoint revision conflicts, exact prior heads, immutable add/remove/retry history, completion/reopen-derived statuses, and cycles introduced by reparenting. A valid atomic multi-task reparent with a transient intermediate cycle commits and replays; invalid final graphs roll back. Request/golden-event fixtures reject forged actors, versions, revisions, IDs and graph changes and verify the event cursor advances.
- Scoped summaries have explicit checkpoint/due/ID ordering, bounded pagination and grant/scope/event-bound cursors. Moving a prerequisite out of a subtree removes its ID/title/reason/history/status; invisible edges collapse to one placeholder. Revocation denies summary/dependency reads. Browser/scoped tokens cannot reach CLI mutation or history routes. Existing step-after semantics and a real task named `all` remain intact. Summary rendering changes no state.
- Final compiled P04 acceptance passed in `.tools/dependency-test-EcPGbR`: CLI relations, cycles/stale revisions/removal, saved-checkpoint and pending-decision summaries, scope/pagination/revocation, read-only queries, SIGKILL/restart exact receipts, inert replay and restored backup. No external adapters or agents are dispatched.
- Actual schema-12 binary upgrade/refusal/rollback passed in `.tools/continuity-test-5cQbYP`, including old-grant limits. Stopped schema-6 through schema-12 fixtures preserve historical events, projections and exact receipts. P03 compiled regression passed in `.tools/preservation-test-QQQME6`; TUI snapshot and real-PTY regression passed in `.tools/tui-test-ryOsJD`. TUI simulation also checks the new dependency status line.
- P03 commit `283e0ce` passed both published CI jobs before P04. P04 planning relations do not add completion evaluators, resource observations, proposal-write grants, ranked planning or automatic continuation. These remain separate contracts/work. R3/W02 read-only workspace observation is next.

## P03 manual preservation — 2026-09-09

- Full Go tests and vet pass with Go 1.27.1. Linux build and Windows/amd64 cross-build pass; Windows was compiled, not executed locally. Linux SHA-256: `5ec9584c19e396516199c3dde6c5fc2c84d65ef9ab4d975efc16e13cf4b124f2`; Windows: `4af1949d3eb81bf04c6ff6f7baa5f70287a3bc99ef8ef7a6aae2f6100475eb3a`.
- Tests cover exact saved-checkpoint artifact pins, changed preview and receipt-head refusal, distinct source/mirror/committed/remote facts, unrelated dirty/staged changes, rebase conflicts, refused credential/database names and symlinks, changed and missing originals, reported copy/push/offline failures, and remote readback from a synthetic bare repository. Repository-configured filters, diff/fsmonitor/SSH/uploadpack helpers are not run. A concurrent commit during mirror observation produces `uncertain`.
- Request fixtures reject unknown/duplicate/version/authority fields and caller-supplied observations. Golden event fixtures reject malformed stages, digests, task scope/revisions and chains. Earlier stopped schema-6 through schema-11 fixtures preserve state, history, exact receipts and pre-upgrade backups. Browser and scoped client credentials cannot reach preservation routes.
- Final compiled P03 CLI acceptance passed in `.tools/preservation-test-KnCBTp`: no writes from preview, exact pins, manual mirror/commit/local-remote push followed by independent readback, unavailable remote, deleted source, portable export, wrong-target refusal, SIGKILL/restart, exact retries, inert replay and restored backup after removing the original clone. No user private clone or network remote was modified or queried.
- Race checks pass for continuity, dotprivate observation, store and daemon. The actual retained schema-11 binary passes upgrade/refusal/rollback in `.tools/continuity-test-7avIIb`; pre-upgrade read credentials keep their original authority. P02 compiled regression passed in `.tools/progress-test-rpY4qB`; TUI snapshots and real-PTY input/restoration passed in `.tools/tui-test-aiKtGh`. The TUI commit `e1ff3e3` passed both published GitHub CI jobs before P03.
- This delivers the roadmap's manual fallback, not programmatic dotprivate dispatch or full P03 automation. Installed dotprivate 0.1.0 matches the reviewed sibling source at revision `51d0fce` (SHA-256 `66fd872c909f7a33ea941d31012750e60647e5a87f4e75a7ee55431f56b4c8ae`); its broad staging boundary remains. Network/authenticated remote behavior, private-clone writer serialization, cancellation of dispatched operations and unattended retries wait for the applicable shared-action/upstream work. Operator reports about events during a manual copy are distinct from readback evidence.

## TUI design replacement — 2026-09-08 Pacific / September 9 UTC

- Full Go tests and vet pass with Go 1.27.1. Race checks pass for `internal/tui`, `internal/continuity`, `internal/store` and `internal/daemon`. Linux build and Windows/amd64 cross-build pass. Windows was compiled, not executed. Linux SHA-256: `1b00ab7d9c3c4bbac98b3af493c34df6a76a54e179734fbf831c3e8e8774cf15`; Windows: `26f42e60c9a4b8a8ff9ae91e571a137804e0be18848db13c96b84ddaf9565848`.
- The browser frontend, generated assets and browser sign-in/session/review routes are removed. Go simulation tests cover Unicode/control escaping, task filtering and keyboard focus, narrow terminals, late responses, read-only views, frozen review preconditions, lost-response exact retry, durable draft success/conflict/reopening and edited draft identity refusal. Historical UI-v2 event fixtures still replay under schema 11; no new browser reviews can be issued.
- `node scripts/tui-smoke.cjs` passes in `.tools/tui-test-xe4oo0` with the compiled Linux binary. It verifies wide/compact text snapshots, exact task/step selection, no events from reads, retired browser routes and pure replay. Its real-PTY gate rejects a completion proposal, accepts a decision with a typed note, saves a checkpoint through the form, opens workspace preview, resizes to compact and quits. Terminal attributes and the alternate screen are restored. All data is synthetic.
- The compiled P02 regression passes in `.tools/progress-test-2QS17c`, including live artifact drift and restart/restore. Actual retained schema-10 upgrade, old-binary refusal and pre-upgrade rollback pass in `.tools/continuity-test-zLKqPb`. Full Go fixture checks preserve schema-6/7/8/9/10 history and original grants.
- Neovim 0.12.5 with the installed lazy.nvim loader passes in `.tools/neovim-test-lURPwA`. Review launches a terminal job with the selected target and argv-only data-directory arguments; checkpoint conflict/restart, proposal inspection and bounded subprocess tests still pass. The editor harness stubs terminal launch; actual TUI input is covered by the separate PTY gate. No editor configuration or clipboard changed. All seven extension unit tests pass.
- WCU inspected the actual compact TUI in an isolated Ghostty 1.3.1 window on Hyprland. The initial inherited `NO_COLOR=1` correctly disabled color; a separate preview with that variable unset rendered the intended amber/green/gray palette. WCU runtime `fb4ac4da6ab03192d99ca4b8e26963a3eb60e9f8e5748350adbc0badad1957ab`, contract `wcu-tools-2`, context schema 1; loaded skill SHA-256 `a09384f3c2baec4bf4e9a9053b5c131d3895babcece3cf9cefc2d94d6e25210e`, distinct from the published skill hash `b1bb77b49d09948340babaa9603962d27195eec86615728b26feede6e5c345bf`. No global desktop or terminal configuration changed. Native modal input was not claimed; wide layout and dialogs have simulation/PTY coverage.

See [TUI setup](TUI-SETUP.md) and [design mapping](design/TUI.md). The reference's native bar, live agent telemetry, durable compositor snapshots and application recovery remain separate roadmap work. Earlier browser-GUI sections below record historical checks, not the current interface.

## P02 GUI review and Neovim handoff — 2026-09-08 Pacific / September 9 UTC

- Full `go test ./...`, `go vet ./...`, Linux build and Windows/amd64 cross-build pass with Go 1.27.1. Race checks pass for `internal/continuity`, `internal/store` and `internal/daemon`. The Windows artifact was compiled, not executed. Linux SHA-256: `775e105658d3a1b7b1b69389a32e0c859a2e83a82b07748866a94c2de690136a`; Windows: `d764c1230fbb6560570c0e9385f305a9d0b87338b81ebdecd6794f890e329015`.
- Go acceptance covers explicitly enabled GUI authority, frozen root/resource scope including historical artifact pins, expiry/logout denial before receipts and commit, exact retries, rollback, live artifact revalidation and scope-safe inspection. UI reviews record version-2 non-secret session provenance; CLI review v1 refuses the new authority field, including explicit null. Golden v2 events replay without live credentials or original files and do not complete tasks or steps.
- The pinned TypeScript build passes. Generated embedded JavaScript SHA-256: `949792c7bb15d61c130a764364356effa61d9d3822db819d7b43bb1c8e8b99b6`. `node scripts/progress-gui-smoke.cjs` passes in `.tools/progress-gui-test-CauMdb`: default-session denial, explicit enablement, literal untrusted text, notes surviving polling, desktop/mobile review, uncertain-response close/reopen and exact retry without duplicate events, stale decision rejection, changed-file acceptance refusal, explicit recheck, late-response/task-switch handling, foreign scope and signed-out denial. Restart, pure replay and fresh-directory backup restore pass after original files and sessions are removed. Desktop/mobile screenshots from `.tools/progress-gui-test-fq80V0` were visually inspected for readability and layout.
- Schema-6/7/8/9/10 fixture upgrades preserve historical events, projections, receipts and grant limits under marker 11. The new stopped schema-10 fixture and UI v2 golden events have [recorded provenance](../testdata/progress/README.md). Compiled upgrade/newer-schema refusal/rollback passes against the retained actual schema-10 binary (`.tools/continuity-test-xJaSeA`) and 0.7.0/schema-6 (`.tools/continuity-test-xVWEej`). The schema-10 binary SHA-256 is `027a5e6030db96762b27e537ec08b6d0e0340dfab7fd376156db511d05204f37`. Only synthetic test databases were upgraded.
- Neovim 0.12.5 acceptance and the installed lazy.nvim loader pass in `.tools/neovim-test-qIEVJb`. New checks cover read-only proposal inspection, escaped proposal text, delayed inspection after task switches and the explicitly enabled GUI handoff. Original checkpoint drafts, conflicts, restart retries and credential-free buffers/history still pass. Configuration/state/cache are isolated; clipboard and browser opening are stubbed, and plugin installation/update is disabled. No global editor configuration changed and no fresh native-editor visual inspection is claimed.
- Compiled affected regressions pass: CLI progress (`.tools/progress-test-IVazJD`), evaluator evidence (`.tools/evidence-test-B8d0Cp`) and existing Chromium completion review (`.tools/gui-test-RP0bhn`). CI now runs the planning-review browser gate alongside the existing GUI gate.

Use `heimdall ui ROOT_TASK --progress-review` or `:HeimdallProgressReview` to enable planning review; `:HeimdallProgress [ID]` inspects a proposal. See [progress setup](PROGRESS-SETUP.md) for scope, retries and schema-11 recovery. GUI proposal authoring, native editor review forms, scoped agent proposal-write grants and file preservation remain open.

## P02 CLI progress/decision review — 2026-09-08 Pacific / September 9 UTC

- Full `go test ./...`, `go vet ./...`, Linux build and Windows/amd64 cross-build pass with Go 1.27.1. Race tests pass for `internal/continuity`, `internal/store`, `internal/checks` and `internal/daemon`. The Windows artifact was compiled, not executed. Current Linux SHA-256: `027a5e6030db96762b27e537ec08b6d0e0340dfab7fd376156db511d05204f37`; Windows: `5ef6212f56c741e13913a6f9ab9a9cb808d1af54e50213edadf35405674ea049`.
- Go tests cover draft/reviewed/accepted/rejected proposals, immutable reviews, exact digest/head conflicts, concurrent acceptance/rejection, decision supersession, inherited scope, changed contract/task/ancestor/decision/resource state, missing/changed artifact bytes and permissions, replacement artifact versions, mandatory context budgets and pure replay. Browser and issued credentials fail the new routes. Scoped context exposes text/status without expanding artifact/Git authority.
- Completion compatibility tests show that proposing or reviewing does not accept a decision or complete work. Accepted decisions invalidate earlier evaluator evidence and refuse stale step-completion ratification. Artifact review leaves task status, revision, evaluator evidence and completion proposals unchanged. Existing authorized manual completion still works.
- `node scripts/progress-smoke.cjs` passes with Node 24.20.0 in `.tools/progress-test-DK2bCb`: real CLI proposal/review/rejection/supersession, multiline text, explicit targets, exact digests, bounded read-only list/show/resume, non-Git artifact drift and lifecycle, SIGKILL/restart exact receipts, removed original files, inert replay and fresh-directory backup restore. Its golden events and a stopped actual P01 schema-9 snapshot are retained in [progress fixtures](../testdata/progress/README.md). CI runs the decision smoke on both platforms, with live artifact cases gated to Linux.
- Schema-6/7/8/9 fixture upgrades preserve old events, state, receipts and grant limits under marker 10. Compiled upgrade/refusal/rollback passes against the actual retained P01/schema-9 binary in `.tools/continuity-test-rTBEbe` and 0.7.0/schema-6 in `.tools/continuity-test-I6cp1n`. P01 binary SHA-256: `e3a9b4ccebc680bf54e30925b6151ef4f9e20a60bd075982abd04da99545d616`. Only synthetic test databases were upgraded.
- Affected compiled regressions pass: artifacts (`.tools/artifact-test-1NPFMc`), resume (`.tools/resume-test-dVS4Gp`), workspace (`.tools/workspace-test-0xdzjF`), evidence (`.tools/evidence-test-Sr2oWQ`), MCP (`.tools/mcp-test-5Rr8pJ`) and existing Chromium GUI acceptance (`.tools/gui-test-p6HzfD`). Scoped feed tests detect progress events without leaking foreign-target activity. No TypeScript/assets changed or new GUI review controls were delivered; no fresh WCU or installed-editor inspection is claimed.
- Broadening race checks exposed the existing subprocess evaluator fixture's one-second successful-exit timeout: the race runtime waits on exit, and the deliberately minimal child environment omits `GORACE`. The fixture now allows five seconds for non-timeout cases and keeps one second for the intentional sleep timeout. Production evaluator timeouts and environment are unchanged; the full affected race suite passes after this test-only correction.

Usage is in [progress setup](PROGRESS-SETUP.md). Review observations are bounded and do not lock the filesystem. Artifact acceptance retains historical identity, not file contents. GUI/editor review controls, agent proposal-write grants, file preservation, workflow timing measurements and workspace recovery remain open.

## Herdr socket replacement fix — 2026-09-08 Pacific / September 9 UTC

The first schema-9 publication [failed on Ubuntu](https://github.com/algorhythmic/heimdall/actions/runs/34311175097) in `TestReportRefusesSocketReplacementBeforeWrite`; matrix fail-fast cancelled Windows. The replacement fixture runs in the same process as the original listener. Reusing its filesystem inode made PID/start time and device/inode insufficient to distinguish the new socket, allowing stale metadata through. This was an adapter identity defect. Local `/tmp` is tmpfs and showed no immediate inode reuse in 200 listener creations, explaining why the existing test did not expose it locally.

The source fingerprint now includes the socket inode's full change timestamp, with a new fingerprint domain. The same identity is compared before/after connection. A deterministic reused-inode regression fails against the old calculation and passes with the fix; ordinary access-time changes remain irrelevant. Both it and the original real socket-replacement test pass 100 repetitions. The full Go suite, vet, Linux build and Herdr/workspace race checks pass. Installed Herdr 0.8.2/protocol-20 acceptance passes in `.tools/herdr-test-NBuIeR`, including pane moves, metadata readback/expiry, server replacement, explicit rebinding, exact retries and inert replay.

Pre-fix Herdr bindings require explicit live rebinding. Stored records and receipts remain unchanged and replayable under schema 9; no user configuration or database was migrated during validation. See [Herdr setup](HERDR-SETUP.md) for reconciliation commands.

## P01 local artifact versions — 2026-09-08 Pacific / September 9 UTC

- Full Go tests and vet pass with Go 1.27.1; race tests pass for `internal/continuity` and `internal/store`. Linux build and Windows/amd64 cross-build pass. Fresh artifact observations are Linux-only; the Windows artifact was not executed. Current Linux binary SHA-256: `5791ab3ac72d28212a930f0b55790b348cede4054ae7b00f6c484eb5803eb113`.
- Go acceptance covers non-Git exact byte digests, Git unborn/clean/staged/changed input, linked worktrees, helper/filter non-execution, exclusion/traversal/symlink/nonregular/size refusal, same-file task/environment separation, four competing version writers with one winner, stale checkpoint refusal, immutable pins and scope/grant boundaries. Golden artifact/v3 checkpoint events replay without filesystem access; malformed versions, unknown fields, forged provenance, duplicate records and legacy payload extensions fail without mutating state. JSON request fixtures pass Go decoding and `fastjsonschema` validation.
- `node scripts/artifact-smoke.cjs` passes in `.tools/artifact-test-7C9PLR`: compiled daemon/CLI record/list/show/check, v3 checkpoint and v2 draft pins, read-only budgeted resume, content change, missing original and explicit relocation. SIGKILL/restart, exact historic receipts, replay and fresh-directory database restore pass after the synthetic original files are deleted. Task completion stays unchanged. File bytes are not retained or restored.
- Actual stopped schema-6, W01 schema-7 and T03 schema-8 fixtures upgrade to marker 9 with original events, receipts, projections and pre-upgrade backup integrity preserved. [Fixture provenance and checksums](../testdata/artifacts/README.md) record the genuine schema-8 source. The retained schema-8 binary passes compiled upgrade/read-grant retention/newer-schema refusal/rollback in `.tools/continuity-test-9KkP8q`; the retained 0.7.0/schema-6 binary passes the same direct upgrade gate in `.tools/continuity-test-nGwPSR`. No user database was migrated.
- Neovim 0.12.5 and the installed lazy.nvim loader pass in `.tools/neovim-test-hpD4mQ`, including P01 pinned artifact display, request-v2 draft submission and refusal of edited artifact references. Original v1 draft/conflict/editor-daemon restart tests still pass. Configuration, clipboard and browser opening are isolated/stubbed as described in the T03 gate. No fresh WCU visual inspection is claimed for P01.
- Compiled legacy resume (`.tools/resume-test-HkX7gy`), workspace/recovery of recorded identity (`.tools/workspace-test-CUzjel`) and MCP (`.tools/mcp-test-W458Wd`) regressions pass. Chromium GUI acceptance with an actual v3 artifact checkpoint passes in `.tools/gui-test-icfvgu`: the session retains opaque pins, receives no expanded artifact metadata, renders the CLI-check requirement, and preserves its existing explicit completion-review behavior. Desktop/mobile screenshots are retained in that scratch directory.
- The portable CI matrix adds the artifact smoke only on Linux. It does not claim Windows/macOS observation or installed desktop restoration. No TypeScript production change or GUI asset rebuild is required for the opaque checkpoint references. File preservation, P02 decision review, remote/platform expansion, user timing measurements and workspace recovery remain open.

```sh
go test ./...
go test -race ./internal/continuity ./internal/store
go vet ./...
go build -trimpath -o bin/ ./cmd/heimdall
node scripts/artifact-smoke.cjs
node scripts/continuity-smoke.cjs /path/to/heimdall-schema8 --legacy-continuity --legacy-grants
HEIMDALL_LAZY_PATH=/path/to/lazy.nvim node scripts/neovim-smoke.cjs
```

See [artifact setup](ARTIFACT-SETUP.md) for commands, observation bounds, version compatibility and backup recovery.

## T03 Neovim integration — 2026-09-08 Pacific / September 9 UTC

- The final installed editor gate passes in `.tools/neovim-test-tMMib3` with **Neovim 0.12.5**, Node 24.20.0 and the schema-8 binary. It uses real CLI/daemon operations and isolated config/state/cache. Two tasks share a repository; explicit picker selection and delayed-response delivery preserve the chosen task. Resume/selection do not write events.
- Draft acceptance covers saved submission, competing checkpoint conflict, wrong selected target, unsaved edits, renamed buffers, changed on-disk content/preconditions, daemon SIGKILL, fresh editor/daemon restart, retained files and exact retry without a second checkpoint. Artifact tests open a filename containing Ex metacharacters literally and refuse outside paths, symlink paths and excluded `.git` content. Modelines remain disabled. Task completion and replay remain unchanged.
- The Herdr display check uses a stopped **synthetic protocol-20 socket** with independently observed local PID/cwd/Git identity, then verifies the editor renders `disconnected`. This is editor acceptance of T02's structured status; actual Herdr runtime movement/restart/expiry acceptance remains in the T02 section below.
- GUI handoff resolves a child to its root workstream, copies only the single-use GUI sign-in code and opens the credential-free URL. Clipboard writes and browser opening are intercepted in this gate; the code is absent from buffers and message/notification history. Missing clipboard support is refused without a code display. The editor adds no completion ratification or credential capability. Actual browser sign-in and completion review retain the separate compiled GUI acceptance recorded below.
- The shipped lazy.nvim example was exercised with installed loader revision `85c7ff3711b730b4030d03144f6db6375044ae82`. Installs, updates, project-local specs and package/rock loading were disabled. Commands trigger lazy loading and setup correctly. This does not certify every LazyVim distribution plugin or the user's full configuration. Neovim 0.11+ is the API requirement; only 0.12.5/Linux was run.
- WCU visually verified the isolated Ghostty/Neovim view loaded through that example: explicit alpha identity, escaped literal task text, accepted direction, saved checkpoint/next action, matching resource and check timestamp. The scratch viewer auto-closed; no global editor configuration or clipboard was changed during verification. Subsequent renderer cleanup removes blank acceptance text and adds explicit step/check labels; final headless acceptance passes.
- WCU runtime revision remains `fb4ac4da6ab03192d99ca4b8e26963a3eb60e9f8e5748350adbc0badad1957ab`, `wcu-tools-2`, context schema 1. Loaded skill SHA-256: `a09384f3c2baec4bf4e9a9053b5c131d3895babcece3cf9cefc2d94d6e25210e`; runtime-published skill SHA-256: `b1bb77b49d09948340babaa9603962d27195eec86615728b26feede6e5c345bf`. These are recorded separately. No WCU keyboard/pointer input was needed for this editor view.
- The subprocess gate checks oversized output, malformed JSON and timeout through actual child processes. Final full Go regressions and Lua formatting checks pass. T03 changes no Go/backend code, schema, routes or existing grants; earlier migration/race results retain their T02 scope. The editor gate is separate from portable CI and does not download an editor.

```sh
node scripts/neovim-smoke.cjs
# Optional installed loader; no network or plugin installation:
HEIMDALL_LAZY_PATH=/path/to/lazy.nvim node scripts/neovim-smoke.cjs
```

See [Neovim setup](NEOVIM-SETUP.md) for commands, clipboard handoff and retained-draft recovery. User workflow timing, broader editor-configuration acceptance, P01 artifact versions and workspace recovery remain open.

## T02 local Herdr integration — 2026-09-08 Pacific / September 9 UTC

- Full Go tests and vet pass with Go 1.27.1. Race checks pass for `internal/adapters/herdr` and `internal/workspace`. Linux build and Windows/amd64 cross-build pass; live Herdr support is Linux-only and the Windows artifact was not run. Protocol fixtures own their Git repository boundary so unrelated ancestor `.git` entries cannot mask the intended assertions.
- The installed **Herdr 0.8.2 / protocol 20** gate passes in `.tools/herdr-test-7BaXkn`: two tasks sharing a repository retain separate surfaces/panes; canonical cwd/worktree/common-directory identity is checked; moving a pane changes its public ID while preserving its terminal ID; cross-task adoption and stale publication are refused. Explicit rebinding preserves the logical surface. A stopped/replaced server produces disconnected/stale checks and requires a new binding.
- Pane metadata and the optional pane-labelled workspace summary pass write/readback and 30-second expiry checks. The bounded expiry gate waits for both independent publications to disappear. Committed retries and event replay do not re-emit metadata, including after source replacement. Heimdall SIGKILL/restart preserves state. These are display-only effects: there is no process restoration, automatic refresh or execution-action journal. A failed database commit after a metadata write can reapply the same scalar display values.
- Adapter/service/reducer tests cover socket replacement, peer/source identity, unsupported protocol/version, response size, unstable process/cwd/pane observations, canonical linked Git worktrees, malformed Git metadata, metadata readback failure, immutable ownership, forged v1 observations, exact retries and replay. Direction tests reject checkpoint next actions after recorded task, contract or decision drift. Browser and issued scoped credentials explicitly fail all three Herdr routes.
- The original stopped schema-6 fixture and an actual stopped W01 schema-7 fixture upgrade to marker 8, preserving events, receipts, projections and authority. Fixture provenance is recorded in [testdata/workspace](../testdata/workspace/README.md). Compiled compatibility checks pass with the retained 0.7.0/schema-6 binary (`.tools/continuity-test-V7jzpv`) and the retained W01/schema-7 binary (`.tools/continuity-test-TR7VV4`): grants retain read-only authority, old binaries refuse schema 8, and each pre-upgrade backup opens with its original binary.
- Compiled workspace, resume, scoped MCP and Chromium GUI regressions pass against schema 8. The Herdr installed gate remains separate from portable CI because CI does not install Herdr. All application state used in these checks is synthetic.
- WCU visually verified the optional sidebar in an isolated Herdr configuration: `w1:p1 | Heimdall alpha`, `Review the plan` and a check timestamp rendered in Ghostty on Hyprland. Default plain-shell pane metadata was not visible, which led to the explicit `--workspace-summary` option and [configuration example](../examples/herdr-sidebar.toml). The test viewer closed and owned scratch servers were stopped; no global Herdr configuration was changed.
- WCU runtime revision was `fb4ac4da6ab03192d99ca4b8e26963a3eb60e9f8e5748350adbc0badad1957ab`, tool contract `wcu-tools-2`, context schema 1. The loaded skill SHA-256 was `a09384f3c2baec4bf4e9a9053b5c131d3895babcece3cf9cefc2d94d6e25210e`; the runtime reported published skill SHA-256 `b1bb77b49d09948340babaa9603962d27195eec86615728b26feede6e5c345bf`. These differed and are recorded separately; only advertised/common capabilities were used.

Reproduce with Go and Node 24 on PATH, plus installed Herdr 0.8.2 for its gate:

```sh
go test ./...
go test -race ./internal/adapters/herdr ./internal/workspace
go vet ./...
go build -trimpath -o bin/ ./cmd/heimdall
node scripts/herdr-smoke.cjs
node scripts/workspace-smoke.cjs
node scripts/resume-smoke.cjs
node scripts/mcp-smoke.cjs
node scripts/gui-smoke.cjs
# Optional: independently retained binaries, never relabelled fixtures.
node scripts/continuity-smoke.cjs /path/to/heimdall-0.7.0 --legacy-continuity --legacy-grants
node scripts/continuity-smoke.cjs /path/to/heimdall-w01-schema7 --legacy-continuity --legacy-grants
```

See [Herdr setup](HERDR-SETUP.md) for exact selectors, migration/rollback and the current version/platform limits. T03 editor commands, observed workspace snapshots and recovery remain open. Earlier milestone sections below retain their original implementation and acceptance scope.

## W01 identity foundation — 2026-09-08

- Full Go tests, vet and Linux build pass with Go 1.27.1. New CLI, service, model and reducer tests cover manifest and binding revisions, concurrent head changes, same-cwd task separation, cross-task/surface lookups, runtime-pane collisions after declared moves, permanent ownership of retired surface IDs, explicit unbind/rebind, stale task/manifest diagnostics, input retention, strict payloads, unknown versions and no mutation from rejected events. Declared POSIX and Windows paths are validated without filesystem access.
- Request and golden event fixtures freeze all three operations. Request fixtures pass the Go decoder and the installed `fastjsonschema` validator against the published JSON Schema. Schema request/record limits and current/individual read limits are enforced; there is no history enumeration or desktop payload retention in this slice.
- The unchanged stopped schema-6 SQL fixture upgrades to marker 7 while preserving exact events, command hashes/results, checkpoint heads and original read-grant permissions. Tests verify the pre-schema-7 backup is still marker 6, has an intact original projection and passes SQLite integrity checking. An obstructed backup directory prevents migration and leaves marker 6 and command receipts intact.
- `node scripts/workspace-smoke.cjs` passes through the compiled binary: explicit same-cwd task declarations, cross-target denial, pane collision, head conflict, read-only/unverified views, SIGKILL/restart, exact retries, pure replay, explicit source-epoch replacement retaining the logical surface ID, immutable binding history, task staleness and restore into a fresh directory. It is now in the Windows/Ubuntu CI matrix; only the local Linux result is claimed here.
- `node scripts/continuity-smoke.cjs .tools/heimdall-0.7.0 --legacy-continuity --legacy-grants` passes using the unchanged 0.7.0 binary saved before implementation. Its synthetic task/contract/checkpoint/read grant survives upgrade, read credentials still refuse writes, the old binary refuses marker 7, and the pre-upgrade backup opens correctly with the old binary. The schema-7 live backup also restores and replays correctly.
- Compiled resume, scoped MCP and Chromium GUI regression smokes pass against the schema-7 build. Browser and issued scoped credentials cannot reach the new CLI workspace routes. Existing checkpoint/contract/grant versions and their authority are unchanged.
- All checks use isolated synthetic data. No native UI behavior changed in W01, so an additional WCU run was unnecessary. Generic locators remain declarations: canonical Git/worktree checks, actual herdr assigned IDs, live source restart/movement detection, metadata publication, observed desktop snapshots and recovery actions have **not** been implemented or verified. Earlier R0/T01 desktop inspection below retains its original scope.

Reproduce with Go and Node 24 on PATH:

```sh
go test ./...
go vet ./...
go build -trimpath -o bin/ ./cmd/heimdall
node scripts/workspace-smoke.cjs
# Optional compatibility gate requires a separately retained 0.7.0 binary:
node scripts/continuity-smoke.cjs /path/to/heimdall-0.7.0 --legacy-continuity --legacy-grants
```

## Linux baseline and terminal continuity — 2026-09-08

- Full Go tests, vet and Linux build pass with Go 1.27.1. TypeScript build/check and extension tests pass; embedded generated JavaScript is unchanged. Compiled core, native helper, continuity, MCP, evidence, resume, GUI, browser and worker checks pass against synthetic data. The smoke harness uses platform-specific executable names, optional `HEIMDALL_BIN`/`HEIMDALL_TEST_TMP`, and bounded child shutdown that closes native stdin. A temp path containing spaces and an explicit baseline executable were tested.
- Test host: Omarchy 4.0.3, kernel 7.1.9-arch1-2, Hyprland 0.56.2, Ghostty 1.3.1. Go 1.27.1, Node 24.20.0, TypeScript 7.0.2, Playwright 1.62.1 and its Chromium 151.0.7922.34 were used. Go and Node archives were checked against official SHA-256 manifests and installed only under ignored `.tools/` paths. Playwright uses its Ubuntu 24.04 fallback build on this Arch-based host.
- The installed Node 26.8.1 binary is 150,425,704 bytes and exceeds the evaluator's existing 128 MiB executable limit. Its evidence smoke correctly returned `unknown/executable_unavailable`. The suite passes with CI-matching Node 24; the evaluator boundary was retained. Initial listener failures were sandbox restrictions, resolved by running the isolated tests with loopback access.
- `resume` tests cover missing/stale context, full-response budget accounting, unchanged legacy context JSON, no event writes, relevant review counts and terminal control/bidi escaping. Draft/submit tests cover blank-summary refusal, exclusive draft creation, explicit target, original preconditions, exact retry and unchanged files on competing-head or transport failure. The compiled resume scenario uses two tasks observing the same worktree, modifies a resource, kills the daemon, and retries after restart without a duplicate checkpoint.
- The [schema-6 fixture](../testdata/continuity/README.md) was produced through the unchanged `60175d1` binary. It preserves tasks, accepted direction, checkpoint, a read-only grant and command receipts through current open/replay. This slice adds no database migration or grant capability.
- WCU inspected the actual Ghostty output on Hyprland: alpha showed its reviewed changed artifact and matching resource, while beta retained its own next action and showed resource drift. The inspection prompted a shorter review section and removal of opaque resource IDs from ordinary warnings. The test window contained only synthetic task data; no workspace restoration, herdr binding or automatic execution was exercised.
- WCU runtime revision: `fb4ac4da6ab03192d99ca4b8e26963a3eb60e9f8e5748350adbc0badad1957ab`, tool contract `wcu-tools-2`, context schema 1. Loaded skill SHA-256: `b1bb77b49d09948340babaa9603962d27195eec86615728b26feede6e5c345bf`, matching the published skill. The installed runtime was inspected separately from the sibling source checkout.

Reproduce with Go and Node 24 on PATH:

```sh
go test ./...
go vet ./...
go build -trimpath -o bin/ ./cmd/heimdall
node --test extension/test/*.test.js
node scripts/core-smoke.cjs
node scripts/native-smoke.cjs
node scripts/continuity-smoke.cjs
node scripts/resume-smoke.cjs
node scripts/mcp-smoke.cjs
node scripts/evidence-smoke.cjs
node scripts/gui-smoke.cjs
node scripts/browser-smoke.cjs
node scripts/worker-smoke.cjs
```

Build the UI and install Playwright Chromium using the README instructions before browser checks. The CI configuration now runs core/native/continuity/resume/MCP/evidence/GUI compiled checks on both Windows and Ubuntu. Browser and worker scripts are portable optional checks; the worker still substitutes native-port discovery. Actual browser native-host registration, native workspace recovery, reboot/power-loss acceptance and user-measured productivity baselines remain open. Process-kill recovery is not power-loss evidence.

## GUI verification in 0.7.0 — 2026-09-05

- Full Go tests, `go vet ./...`, TypeScript compilation, and Windows/Linux-amd64 CGO-disabled builds pass. Database schema remains 6. No race-detector or Linux desktop acceptance is claimed.
- Go GUI tests cover single-use bootstrap, HttpOnly/Strict cookies, unauthenticated/cross-origin/missing-CSRF refusal, subtree isolation, cursor scope/expiry, exact completion retries, logout/revoked retries, and checkpoint/evidence feed changes without a document revision change.
- The final compiled Chromium smoke passed in `.tools/gui-test-FbiGpx`: real artifact evidence and accepted continuity context, isolated task scope, literal untrusted title, keyboard search, desktop 1440px/mobile 390px layouts, explicit completion review, logout and code replay denial. Both screenshots were visually inspected; the synthetic desktop capture is retained in [GUI setup](GUI-SETUP.md). All test processes closed.
- The final compiled evidence regression also passed in `.tools/evidence-test-c9HwxK`, including live stale-input denial, exact retry without test reexecution, invalidation/supersession, replay/restart and explicit reevaluation/ratification.
- CI now builds TypeScript and checks generated JavaScript on both platforms and runs the compiled Chromium GUI smoke on Windows. Remote results for this GUI checkpoint must be checked separately; the evidence-only run below does not certify GUI changes.

## Evidence verification in 0.6.0 — 2026-09-05

- The full Go suite and vet pass; Windows and Linux/amd64 CGO-disabled builds succeed. The compiled core and native-host regression smokes also pass. Focused tests cover live stale-file denial, partial coverage, forged results, wrong resources, contract changes, exact Git-root/commit identity, test pass/failure/mutating-input/output-cap/timeout outcomes, uncertain recovery, step ratification and replay equality. CLI/browser/scoped credential boundaries remain intact.
- The compiled evidence smoke passed in `.tools/evidence-test-SNpYn3`: configure a real Node test, record observer/executable/output provenance, retry without another invocation, refuse stale completion, invalidate/supersede, replay/restart, explicitly reevaluate and ratify.
- The actual archived 0.5.0 executable created a schema-5 continuity/read-grant fixture. The current binary upgraded it to schema 6, retained read-only permissions, backed up/restored it and rolled back from the pre-upgrade snapshot in `.tools/continuity-test-AUQj1v`.
- The compiled MCP regression smoke passed in `.tools/mcp-test-rxlPmp`. CI now includes the compiled evidence smoke on Windows as well as the existing Windows/Ubuntu checks. A new remote CI result must be checked for this commit; the older publication run below is not evidence for new code.
- No GUI implementation is included in this checkpoint. Native browser installation, runner integration, general checkpoint Git identity, raw-output retention and the other remaining acceptance gaps are not claimed.

## GitHub publication checks

The [0.6.0 evidence checkpoint](https://github.com/algorhythmic/heimdall/actions/runs/33952148968) passed on Windows and Ubuntu for commit `1fb006785506674095e756e8e5efa9dcb27313e7`. Windows included the compiled evidence smoke alongside continuity and MCP checks.

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
