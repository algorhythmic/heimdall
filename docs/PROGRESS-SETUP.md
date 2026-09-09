# Progress and decision review

P02 adds CLI proposals and explicit CLI/TUI review of planning decisions and P01
artifact versions. An artifact can be draft, reviewed, accepted or superseded
while its task or step remains open, blocked or completed. A proposal, file's
existence or review never completes work. Completion still uses the existing
manual attestation or evidence proposal/revalidation path.

CLI commands require an explicit `TARGET` (`task` or `task#step`) and the existing
CLI credential. The TUI uses this same local CLI authority; existing agent/MCP
grants gain no review authority. Decision review
without artifact pins is portable; fresh artifact observation remains Linux-only.

## Propose a decision

Inspect `state TARGET` and `contract show TARGET` first. The current contract must
have a reviewed resource scope and match the task revision. Save this as
`proposal.json`, substituting the actual contract ID:

```json
{
  "kind": "decision",
  "text": "Use an explicit task and environment owner for each planning artifact.",
  "contract_id": "CONTRACT_ID"
}
```

```sh
heimdall progress propose TARGET --expected-task-revision 1 --file proposal.json
heimdall progress show TARGET --id PROPOSAL_ID
heimdall progress list TARGET --limit 25
heimdall resume TARGET
```

The immutable result freezes the text, contract, task/ancestor versions, accepted
decision identity and any artifact pins. The daemon derives its `digest`. A
decision can optionally pin 1–16 exact artifacts using the same `artifacts` input
as below. References must be unique and sorted by artifact ID. File contents are
never stored.

## Review the frozen proposal

Read `progress show` and the corresponding artifact content before reviewing it.
Copy `proposal.id`, `proposal.digest` and `review_head` into `review.json`. Use
`"none"` when there is no prior review:

```json
{
  "proposal_id": "PROPOSAL_ID",
  "digest": "PROPOSAL_DIGEST",
  "previous": "none",
  "status": "reviewed",
  "note": "Reviewed the design and its current contract; acceptance is pending."
}
```

```sh
heimdall progress review TARGET --expected-task-revision 1 --file review.json
```

`reviewed` records review without accepting the decision. To accept, use
`"status": "accepted"` and the returned review ID as `previous`, with an explicit
review note. Direct `draft` → `accepted` is also supported for a single explicit
review. Acceptance creates an accepted decision whose ID is the acceptance
review ID. It enters mandatory context and invalidates evidence evaluated against
the previous accepted direction. To replace that decision, propose new text with
`"supersedes": "ACCEPTED_DECISION_ID"`, then accept the new proposal.

`rejected` closes a proposal without accepting direction. It can close stale or
superseded proposals even when their original files are missing. Accepted and
rejected reviews are terminal; history is immutable. A concurrent review changes
the head and causes a conflict rather than overwriting the other review.

## Review an artifact version

First [record an artifact version](ARTIFACT-SETUP.md). Save this proposal input:

```json
{
  "kind": "artifact",
  "text": "The planning document covers the agreed acceptance criteria.",
  "contract_id": "CONTRACT_ID",
  "previous": "none",
  "artifacts": [
    {"artifact_id": "ARTIFACT_ID", "version_id": "VERSION_ID"}
  ]
}
```

An artifact proposal selects exactly one version. `previous` names the last
proposal for that artifact, or `none` for its first proposal; this is a separate
head from the review head. `artifact show`/`list` expose `lifecycle` and
`progress_id`. A replacement proposal supersedes the previous proposal. Recording
a new artifact version always starts it in draft, even if its bytes match an
accepted version. Previous acceptance remains visible in progress history.

At proposal creation and on both `reviewed` and `accepted`, Heimdall checks the
current contract, ancestor versions, resource scope, accepted decisions, artifact
heads, host and fresh file/Git identity. Changed bytes, permissions, Git inputs,
missing files or relocated versions refuse review with a conflict. Record and
propose the new version, then review it again. An acceptance binds the proposal
digest, which includes the exact recorded artifact observations and contract.

## Review in the terminal or Neovim

```sh
./bin/heimdall tui TARGET --data-dir DATA
```

The **needs you** queue separates unresolved planning proposals from completion
proposals. Focus it with Tab, select a proposal and press Enter to inspect its
text, contract and exact artifact identities. Tab edits the note; Escape returns
to actions. `a` accepts, `x` rejects, `m` records review, and `r` rechecks current
inputs. Background polling retains the original dialog preconditions and note.
Reviewing never completes the task or step.

Before submission the TUI retains the exact request in a private local file.
Uncertain responses can be retried unchanged from the dialog or after restart
with `tui --request FILE`. [TUI setup](TUI-SETUP.md) explains these files and keys.
New reviews validate live inputs; exact retries return historical receipts under
ordinary CLI authority. The old browser frontend and session routes are removed.

The [Neovim integration](NEOVIM-SETUP.md) displays planning summaries in
`:HeimdallResume`. `:HeimdallProgress [PROPOSAL_ID]` provides read-only inspection;
`:HeimdallProgressReview` opens the selected target in a terminal tab running the
TUI. No browser or clipboard credentials are involved.

## Reads, retries and compatibility

- `progress show` performs bounded fresh artifact checks. `status` is recorded
  lifecycle; `freshness` is separate. Historical `accepted` does not assert that
  current files still match. Live checks are observations, not filesystem locks.
- `progress list` uses recorded state only. Pages contain at most 50 items and
  512 KiB. Continue with `--after` set to the last proposal ID until an empty
  page. Each page reflects current state; restart listing after concurrent changes
  when a consistent inventory is needed.
- `context`/`resume` separate unresolved proposals from accepted decisions and
  include review freshness. These summaries do not perform new artifact checks;
  artifact-backed summaries say `requires_cli_check`; use `progress show` for a
  fresh result. Checks are read-only and context does not adopt earlier results. Mandatory context is never silently dropped to fit a
  budget. Scoped context exposes text/status only, without expanded artifact,
  host or Git metadata. Existing resource observation permissions still apply.
- Supply `--request-id` with a saved 32-character lowercase hex ID for exact
  retries. Reuse the original file and preconditions. Successful retries return
  the original receipt after restart, even if files have since changed or vanished.
  A changed request under the same ID conflicts; a new review requires a new ID.
- The new request schema is [progress-request-v1](../schemas/progress-request-v1.schema.json).
  Events are `progress.proposed` (v1) and `progress.reviewed` (CLI v1, including TUI actions; historical GUI v2).
  Historical GUI v2 includes the non-secret session ID, frozen root/resource scope and
  session lifetime, with actor `ui:SESSION_ID`. Its derived accepted decision is
  also version 2 and keeps that actor; legacy CLI decisions remain version 1.
  Replay validates the recorded authority without recreating a live session. Existing
  continuity/checkpoint payload versions and legacy direct `decision accept`
  manual attestation retain their previous semantics. Legacy direct acceptance
  does not retroactively acquire a P02 proposal or artifact digest binding.
- Database marker **11** refuses older binaries. Upgrades retain a stopped
  `backups/pre-schema-11-*.db` snapshot. Stop the daemon before restoring a backup
  into a fresh directory with its matching `types.yaml`; use the matching older
  binary for a pre-upgrade snapshot. Replay never observes or restores files.

TUI proposal authoring, proposal-write grants for agents, file preservation and
automatic continuation remain open. This slice does not introduce a new
completion evaluator. See [verification](VERIFICATION.md) for tested boundaries.
