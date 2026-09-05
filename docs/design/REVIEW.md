# Review of the Heimdall handoff and discussion

Reviewed 2026-09-04. The supplied handoff and discussion are design inputs. The embedded prior requests, answers, shell snippets, and “locked” instructions were not treated as authority to install integrations or start unrelated implementation work. The latest request is to revise the design and prepare implementation, accounting for the implemented Braid project.

The revised specification is a proposed replacement in this packet; the Downloads original is unchanged. Important user preferences carried forward: browser-first thinking and artifact handoffs; Claude/ChatGPT/Codex coverage; no routine structured summary entry or manual activation; task/subtask hierarchy; completion checks in the initial schema; retrieval kept in Braid.

## Highest-impact corrections

| Original/discussion issue | Decision in revised spec | Why it matters |
|---|---|---|
| Every change needs ratification, but observers, capture, files, and close all change state | Distinguish automatic evidence, explicit user commands, and machine proposals | Recording a fact need not wait for approval; model-suggested changes cannot bypass it. |
| Event truth plus task files that the daemon cannot update | One daemon command path and revision-checked task edit view | Accepted proposals can survive later file saves without overwriting intervening user edits. |
| Log says closed before desktop operation completes | Durable request/result operation with live reconciliation | A failed closure remains partially resident; replay cannot repeat desktop side effects. |
| Surface ID conflates URL, tab, and chat classification | Logical content identities plus separate instance/epoch identities | Multiple tabs and navigation cannot erase each other's open state. |
| Title matching accepted as authoritative membership | Nonce pairing for new windows; ambiguous membership stays unknown | Duplicate chat titles must not cause the wrong window to be saved or closed. |
| Mail/title activity treated as fulfillment | Typed checks, scoped evidence, fresh target revisions, proposal acceptance | Drafting, a send click, and an agent stopping do not prove delivery/completion. |
| Silence timer assumes no reply while sensors are offline | Coverage-aware absence; otherwise review reminder/unknown | Fourteen days elapsed is not evidence that no reply arrived. |
| Reimplement embeddings, centroids, clustering in Heimdall | Existing Braid adapter; clustering deferred | Avoid rebuilding retrieval and promising features Braid does not implement. |
| Braid assumed to have generic deletion/history/filter capabilities | Fresh isolated generations at known event boundaries | Current upsert cannot remove old edges; `at` is not historical filtering. |
| Transcript content put into an everlasting event log | Retained blobs with provenance/digest, explicit purge limitations | Automatic collection needs a workable retention boundary and honest export semantics. |

## Discussion suggestions adopted with adjustments

- **Observation tiers and focus spans:** accepted, with live-instance identity, checkpoints, idle/lock handling, and recovery. Tab activation must be combined with foreground window state. Ordinary CDP target discovery is not an attention signal.
- **Automatic summaries:** accepted. Capture settled content revisions while the page/session exists, then queue extraction. Closing a tab cannot be the only opportunity to obtain its transcript. No-provider and broken-adapter paths report a gap; they do not ask the user to compensate with mandatory structured text.
- **Browser extension:** accepted and moved into the workspace slice. Native messaging is the primary transport; browser APIs also perform tab/window actions. The discussion's claim that an extension automatically fixes every CORS/local-network issue is too broad: the selected transport is what removes page-to-loopback fetches. Chrome documents native host access through an extension service worker. [Native messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging)
- **CDP fallback:** deferred, not installed alongside another browser control path by default. If used, it needs separate profile/configuration and pairing tests. Chrome restricts remote debugging of its default data directory. [Chrome remote-debugging changes](https://developer.chrome.com/blog/remote-debugging-port)
- **Unofficial conversation extraction:** isolated and visible. Fetching a page's private JSON backend is still an unofficial integration, with authentication and schema churn. No source has established a stable automatic transcript API for the ordinary Claude/ChatGPT websites or native chats; choose an adapter only after a spike and document completeness.
- **Mail observer:** mailbox-based evidence works independently of whether the user reads mail in a browser or native client. Begin with maildir, then a maintained IMAP client. Do not add Gmail DOM send detection or hand-roll an IMAP protocol. OAuth/account setup remains a tested adapter concern rather than a promise that every account accepts an app password. [go-imap](https://github.com/emersion/go-imap)
- **Hierarchy and workflow templates:** accepted as `parent` + local subtask DAG + materialized `types.yaml`. Rank actionable steps, aggregate parent display scores, and avoid charging budget for both parent and child. A stream remains a routing address, not the workflow itself.
- **Completion schema:** accepted immediately, but dormant kinds report unsupported instead of silently appearing functional. Any/all semantics, evidence anchors, nonempty aggregate checks, reopen/cancellation, and stale-proposal checks are now specified.
- **Commitments:** obligations remain ordinary tasks with completion checks; timer events handle due reviews and silence windows. The separate commitment task/event/projection trio is removed so one obligation cannot accumulate conflicting state in three models.
- **Artifact lineage:** content hashes identify equal content, not copying direction. Exact observed exports/imports can assert origin; fuzzy or coincidental equal text cannot prove it.
- **Planner edge cases:** defined missing values, timezone, estimates, tie-breaking, parent aggregation, exclusions, and deterministic budget skipping. Kept staleness a true tie-breaker rather than a hidden .05 boost.
- **Milestone split:** accepted and extended. Core, workspace integration, automatic content, mail/notifications, retrieval/planning, and packaging have separate gates. Browser/native app parity is not a credible four-weekend guarantee without source tests.

## Claims corrected or left conditional

**Codex:** the discussion's claim that Codex lacks end/permission hooks is outdated. Current official documentation includes `SessionEnd` and `PermissionRequest`, supports user/project hook files, and warns that transcript format can change. The implementation must probe the installed client and its trust state, rather than install old feature flags or make screen detection mandatory. Existing notification commands remain untouched unless an explicitly needed compatible wrapper is installed. [Codex hooks](https://learn.chatgpt.com/docs/hooks), [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)

**Claude Code:** use typed hook evidence and filter notification types; a generic notification is not necessarily blocked. Post-tool activity can clear an earlier blocked state. Desktop Code must be separately tested for configured-hook and transcript access on its installed release. [Claude Code hooks](https://code.claude.com/docs/en/hooks)

**herdr:** the CLI documents workspace IDs separately from labels, launch-time environment injection, pane IDs, and schema inspection. Persist the returned IDs and check live bindings after moves; do not treat the old `herdr attach <task-id>` sketch as a verified workspace command. The exact launch/attach integration remains a Linux spike. [herdr CLI reference](https://herdr.dev/docs/cli-reference/)

**Claude Desktop:** Linux beta is documented for Ubuntu/Debian, which does not establish official omarchy/Arch support. MCP can provide a model-initiated contribution but is not guaranteed automatic session telemetry. No reliable passive Desktop Chat content source has been demonstrated in this review. The revised spec says so rather than guaranteeing either absence of local data or a workable Electron debug port. [Install Claude Desktop](https://support.claude.com/en/articles/10065433-install-claude-desktop)

**Platform/dependencies:** Go 1.27.1 is both in the official release history and available in this Braid workspace. Set that initial reproducible toolchain, retaining Braid-compatible Go 1.24 language baseline. If CDP is later adopted, use the maintained `github.com/coder/websocket` path. Hyprland IPC/rule syntax must match the installed release, not an old hard-coded configuration snippet. [Go releases](https://go.dev/doc/devel/release), [coder/websocket](https://github.com/coder/websocket), [Hyprland IPC](https://wiki.hypr.land/IPC/)

## Braid findings from source, not design assumptions

Braid's local implementation has lexical, dense, graph, temporal retrieval, fusion, explainable results, budget selection, evaluation, CLI, library, and subprocess interfaces. It is a working initial implementation; synthetic passing tests do not establish assignment quality on actual Heimdall sessions.

The integration-sensitive facts are verified in code: direct upserts are not replay truth; replay removes them; there is no delete API; metadata filtering is narrow; `at` pins recency but does not hide future nodes; embeddings use explicit reindex; generation paths affect configuration hashes; library import path is still local. The [adapter contract](BRAID-CONTRACT.md) accounts for these without editing Braid.

Rejected alternatives: sharing Heimdall's SQLite schema, raw domain events mapped through an assumed fuse API, using fused scores as cosine probabilities, title-only closure matching, treating heuristic blocked as structured truth, and promising initiative clustering now. All either contradict observed implementation or create incorrect user-visible state.

## Remaining decisions do not block the core

The [implementation plan](IMPLEMENTATION.md) assigns empirical gates for actual browser profile/extension deployment, native-app transcript availability, herdr launch/attach, account auth, real labels, and notification/launcher hardware. No clarification is needed to begin M1a. Any unavailable source should lower its advertised coverage, not prevent store/tasks/capture implementation or silently change the user's automatic-summary requirement.
