# Neovim task continuity

T03 provides a small repository-owned Lua plugin using Heimdall's existing CLI.
Select a task or step, read current resume context, edit and submit checkpoint
drafts, open bound artifacts, check a Herdr binding, and launch completion review.
The editor uses the configured CLI and adds no credential class. The original schema-8 workflow gained [P01 artifact pins](ARTIFACT-SETUP.md) at
schema 11; the current daemon uses schema 21.

The installed acceptance target is Linux with Neovim **0.12.5**. The plugin uses
Neovim 0.11+ APIs; older supported API versions and other platforms have not been
tested. Artifact opening currently supports local POSIX resources only.

## Configure the plugin

Build `bin/heimdall`, initialize your chosen data directory and start its daemon
using the [terminal setup](CONTINUITY-SETUP.md). Configure one explicit executable
and data directory. Selection is local to this editor process; cwd, inherited
Herdr variables and filenames never select or bind a task automatically.

For LazyVim/lazy.nvim, adapt the [example plugin spec](../../examples/neovim-heimdall.lua)
in your chosen configuration's `lua/plugins/heimdall.lua`. Replace all example
paths. The repository plugin is loaded when a Heimdall command is used; it does
not require a download or additional editor plugin. No keymaps are installed.

For plain Neovim, this is the equivalent setup:

```lua
vim.opt.runtimepath:prepend('/path/to/heimdall/integrations/neovim')
require('heimdall').setup({
  executable = '/path/to/heimdall/bin/heimdall',
  data_dir = '/path/to/heimdall-data',
  -- Optional; default: stdpath('state') .. '/heimdall/drafts'
  draft_dir = '/private/path/to/checkpoint-drafts',
  timeout_ms = 10000,
})
```

Keep drafts outside observed resource trees, or exclude their directory. The CLI
creates private draft files exclusively; the plugin disables swap and persistent
undo for their buffers. Normal explicit editor writes and your configured backup
behavior still apply. Installation is an example/manual configuration step; the
implementation tests do not modify your global editor configuration.

## Commands

| Command | Behavior |
|---|---|
| `:HeimdallSelect [task-or-task#step]` | Choose from the daemon's task/step list, or validate an explicit target; then open its resume view. |
| `:HeimdallResume` | Fetch fresh structured context for the selected target. |
| `:HeimdallCheckpointDraft` | Create a new draft for the selected target and open it in a split. |
| `:HeimdallCheckpointOpen path` | Open a retained draft explicitly, including after editor restart. Its target must match the selection. |
| `:HeimdallCheckpointSubmit` | Submit the current saved draft with its original request ID and preconditions. |
| `:HeimdallArtifact` | Choose an active resource; for a tree, enter a relative file path. |
| `:HeimdallSessionCheck surface-id` | Check an explicitly selected logical surface through T02; display current/stale/disconnected status and issues. |
| `:HeimdallReview` / `:HeimdallTUI` | Open the selected task or step in a new terminal tab running the TUI. |

Resume is a read-only scratch view showing accepted direction, saved progress,
blockers, resources, pinned artifact versions/checks, drift and recorded evidence/review counts. Its timestamp is
the context check time; there is no background refresh. Task controls/bidi text
are escaped, and an unusually long display field is marked if truncated. A late
view response cannot replace another selected task. Closing/replacing the scratch
view wipes it; ordinary file buffers and unsaved edits are preserved.

Use `:HeimdallCheckpointDraft`, edit `checkpoint.summary` and
`checkpoint.next_action` (plus optional blockers/current step), then `:write` and
`:HeimdallCheckpointSubmit`. The plugin refuses unsaved or renamed draft buffers,
on-disk changes not reflected in the buffer, another selected target, or altered
request identity/preconditions, including exact artifact references on request-v2 drafts. It never advances a stale head automatically. Use the CLI to prepare explicitly revised artifact pins.

A successful submission retains its file. After an uncertain transport failure,
retry the unchanged draft for the same receipt. After a conflict, inspect fresh
context and create a new draft, transferring notes after review. After editor
restart, select the original target and use `:HeimdallCheckpointOpen` to reopen
the retained file. Reopening pins that file's reviewed identity; the CLI/daemon
still validate it against current state and committed receipts.

Artifact opening uses literal filename APIs and a fresh context response. It
requires an active file/tree resource and a regular file within its canonical
root. Tree exclusions (including `.git`), outside paths, missing files, symlink
paths and directories are refused. Modelines are disabled before loading the
file. Normal trusted editor filetype plugins/autocommands can still run. This
opens the current local file. Recording and checking immutable versions uses the [P01 CLI](ARTIFACT-SETUP.md); opening a file does not record a version.

Herdr remains optional. Session checking does not infer a surface, bind a pane,
publish metadata or block ordinary resume/checkpoint operations. Use the
[Herdr CLI](HERDR-SETUP.md) for explicit binding and reconciliation.

## Completion review and authority

Review opens the configured executable as an argv array in a new Neovim terminal
tab: `heimdall tui SELECTED_TARGET --data-dir DATA`. The selected task or step is
preserved. There is no browser, clipboard handoff, sign-in code or shell command.
`:HeimdallProgressReview` and `:HeimdallTUI` open the same terminal interface.

The TUI reads its own local CLI endpoint and offers explicit completion and
planning review controls. It is a local human CLI integration, not a new scoped
machine-client credential. Closing the terminal does not alter the selected task
or other editor buffers. Use the usual Neovim terminal-mode escape to return to
editor navigation. See [TUI setup](TUI-SETUP.md) for controls and retained drafts.

Requests run asynchronously with a configurable 10-second default timeout and
a combined 1 MiB stdout/stderr limit. Invalid responses and command failures are
reported without echoing raw output. There is no automatic submission retry.

## Reproduce acceptance

With a current Heimdall binary, Linux, Neovim and Node 24:

```sh
node scripts/neovim-smoke.cjs
# Optional: test the example spec with an already installed lazy.nvim checkout.
HEIMDALL_LAZY_PATH=/path/to/lazy.nvim node scripts/neovim-smoke.cjs
```

`HEIMDALL_BIN`, `HEIMDALL_TEST_TMP` and `HEIMDALL_NVIM` override the binary,
scratch parent and editor executable. The gate creates isolated config/state/cache,
synthetic tasks sharing a repository and an owned protocol-fixture socket; it
stops that socket to test disconnected Herdr display. The real installed Herdr
gate remains separate. It also checks task switching, stale responses, draft
conflicts and restart retries, artifact path handling, TUI handoff, output bounds
and unchanged task completion. The terminal job launcher is intercepted for this editor
gate; actual terminal acceptance is covered by `scripts/tui-smoke.cjs`.

The optional loader gate disables installs, updates and project-local specs. It
tests the lazy.nvim example, not every LazyVim distribution plugin or user override.
The installed editor gate is separate from portable CI; no editor is downloaded
by the test workflow. See [verification](../VERIFICATION.md) for the visual WCU check
and exact tested revisions. Workflow timing measurements, file-content preservation, automatic refresh and workspace recovery remain open.

## P02 planning review

- `:HeimdallResume` separates planning proposals from accepted direction.
- `:HeimdallProgress` opens a proposal picker for the selected context. An optional
  `PROPOSAL_ID` inspects a proposal owned by the selected task/step, including
  historical accepted/rejected records. The view includes its frozen text,
  contract/digest, exact artifact versions, freshness and review provenance.
- `:HeimdallProgressReview` opens the selected target in the TUI. Choose its
  proposal and review it explicitly there.

Inspection is read-only, and late responses are discarded after another view or
selection. Opening the terminal does not itself accept or reject a proposal.
Proposal authoring remains a CLI workflow; native review controls are in the TUI.
See [progress setup](PROGRESS-SETUP.md).
