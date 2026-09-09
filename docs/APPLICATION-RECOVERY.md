# Reviewed application recovery (W06)

W06 connects explicit application recipes to W05's shared operation/action
journal. Linux supports a dedicated Foot terminal, direct attachment to an
existing Herdr terminal, a paired browser, and structured Neovim saved-file
state. The tested native baseline is Foot 1.28.0, Herdr 0.8.2/protocol 20 and
Hyprland 0.56.2. Browser recovery requires extension 0.6.0's recovery capability.
Other native platforms retain the records but cannot execute these adapters.

An operation's `matched` result describes its supported action postconditions.
It is not W07's full recovery report: terminal attachment rendering, editor
buffers/cursor readback, layout and display placement can remain unverified.
Task completion is independent of application recovery.

## Review a recipe

Accept a desired manifest and select a compositor as described in
[workspace operations](WORKSPACE-OPERATIONS.md). Recipe review does not launch
anything. Submit an explicit task, revision, manifest, surface and previous
recipe head. `previous: "none"` creates the first recipe; subsequent review
uses its immutable ID. Omit `spec` to retire it. Changing a recipe invalidates
earlier workspace reviews. Commands never come from observed titles, process
arguments, captured shell history or task text.

```sh
sha256sum /usr/bin/foot /usr/bin/bash
heimdall application review alpha --file /path/to/reviewed-recipe.json
heimdall application show alpha --surface SURFACE_ID
```

Example generic-terminal request (replace all pins, paths and digests):

```json
{
  "version": 1,
  "id": "11111111111111111111111111111111",
  "target": "alpha",
  "expected_task_revision": 1,
  "manifest_id": "22222222222222222222222222222222",
  "surface_id": "33333333333333333333333333333333",
  "previous": "none",
  "spec": {
    "adapter": "foot",
    "executable": "/usr/bin/foot",
    "executable_digest": "REPLACE_WITH_SHA256",
    "command": "/usr/bin/bash",
    "command_digest": "REPLACE_WITH_SHA256",
    "argv": ["--noprofile", "--norc"],
    "cwd": "/absolute/canonical/worktree",
    "close_policy": "leave_open"
  }
}
```

Both executable hashes are checked at review and before dispatch. The cwd must
exist and be canonical; there is no fallback to the daemon's directory. Missing
or changed executables, directories and saved editor files refuse preparation.
Arguments are passed literally through an argv array, without a shell wrapper.
The reviewed command may itself be a shell; that is an explicit recipe choice.
Recipe files can contain sensitive arguments and should be stored privately.

Save or select a retained point, review a fresh diff, then explicitly open:

```sh
heimdall workspace diff alpha --output /tmp/alpha-review.json
heimdall workspace open alpha --file /tmp/alpha-review.json --surfaces all
heimdall workspace operations alpha
heimdall action show alpha --id ACTION_ID
```

The existing CLI/TUI open and close controls consume the recipe. Human-readable
reviews include its adapter, command/cwd or browser URL, and close policy.
Recipe creation/retirement is a CLI operation. `application review` accepts the
[request envelope](../schemas/application-request-v1.schema.json).

## Application boundaries

| Adapter | Reviewed inputs | Open and close behavior |
| --- | --- | --- |
| `foot` | Emulator/command digests, literal argv, canonical cwd | Reopens the view. Prior processes are **not resumed**. `leave_open` preserves it; `graceful_session_end` explicitly permits graceful window closure and consequent shell-session termination. |
| `foot-herdr` | Same executable pins, empty argv, exact `session_binding_id`, bound cwd | Generates `herdr terminal attach TERMINAL_ID` against the exact bound API socket. Uses no takeover. `detach` closes only a previously associated attach view and then checks the original pane/process survives. `leave_open` is also supported. |
| `browser` | `browser: {"profile": "PROFILE_ID", "url": "https://…"}`, close policy | Focuses the current owned tab or uses C13 nonce pairing to create an absent view. `owned_tab` closes that exact tab; `leave_open` preserves it. It never closes an entire browser window as a substitute. |
| `foot-nvim` | Emulator/Neovim digests, empty argv, cwd, `editor` metadata | Opens saved files using clean Neovim (`-u NONE -i NONE --noplugin`). Only `leave_open` is accepted; workspace close cannot destroy potentially unsaved buffers. |

For Herdr, first use `session bind-herdr` and inspect `session refresh` as in
[Herdr setup](HERDR-SETUP.md). Recipe review pins that immutable binding. The
adapter checks the complete locator and Herdr identity before input and during
readback. Pane movement, cwd/worktree changes, server restart or shell PID/start
changes invalidate the recipe's session. Expired sessions are reported without
starting a new server or reissuing a prior process command.

The pinned [Herdr direct-attach implementation](https://github.com/herdrdev/herdr/blob/v0.8.2/src/client/mod.rs)
connects to the selected server and exits on failure. This differs from the
[named-session command](https://github.com/herdrdev/herdr/blob/v0.8.2/src/session.rs),
which rewrites `session attach` to ordinary startup. Only direct-terminal
attachment is generated. No terminal input, prompt or agent resume command is
sent by this adapter.

For the browser, omit executable/command/cwd fields. The paired profile must
have a fresh complete inventory and advertise recovery protocol 1. Existing
URLs without current task ownership are left unowned and block duplicate
creation. The extension checks URL/pending-URL duplication both before marker
creation and before navigation. A changed browser epoch or unresolved prior
attempt requires explicit reconciliation; this release does not automatically
adopt browser self-restored tabs across epochs. If closure leaves a native
window containing other tabs, residency remains conservatively partial.

Neovim metadata is structured data, for example:

```json
"editor": {
  "files": ["/absolute/worktree/notes.md", "/absolute/worktree/main.go"],
  "active": 1,
  "line": 12,
  "column": 1
}
```

`active` is zero-based; line/column are one-based. Files must already exist.
This restores a reviewed saved-file argument list and requests its cursor
position. It does not source Vim session scripts, reconstruct unsaved buffers,
run terminal buffers, restore plugins or claim full LazyVim session fidelity.
Detailed attachment/editor readback belongs to W07.

## Interruption and identity

Each prepared launch is single-use and expires after two seconds. The writer
commits `dispatching` before process creation. A receipt records PID/start time;
fresh compositor inventory must then contain exactly one window with that PID
and the attempt's unique app ID. A second process check rejects PID reuse. The
verification and new logical-surface binding commit together. Old compositor
identities are never relabeled as new ones.

Only this operation's proven output bindings are excluded when rechecking its
original input digest. External rebinding or recipe edits invalidate the pins.
Cancellation prevents new input while allowing observation of an already
launched view. Uncertain launches keep their surface and resident reservation;
neither a new operation ID, a daemon restart nor replay repeats them. Missing
launch receipts remain uncertain even if a similar window exists.

Schema 20 adds recipe records, native application intent v4, workspace-browser
intent v5 and observed application viewport v3. Schema-19 events retain their
original input digests when no recipe was present. Upgrade creates a
`pre-schema-20-*.db` backup; use the stopped-daemon backup/rollback workflow from
[verification](VERIFICATION.md). Replay performs no filesystem or app calls.

Installed acceptance is opt-in:

```sh
HEIMDALL_NATIVE_APPLICATION=1 node scripts/application-smoke.cjs
HEIMDALL_APPLICATION_BROWSER=1 node scripts/browser-pairing-smoke.cjs
```

The first creates disposable GTK/Foot/Herdr/Neovim views and isolated task data.
The second uses an isolated Chromium profile and synthetic compositor sockets;
add `HEIMDALL_PAIRING_HEADFUL=1` to exercise the selected real compositor. Tests
leave acceptance records in `.tools/`. No login hook or user app configuration
is installed. Reboot, compositor interruption and controlled VM power-loss
acceptance remain W08 gates.
