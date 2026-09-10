# Setup and usage

These guides cover implemented interfaces. [Current status](../STATUS.md) and
[acceptance reports](../README.md#acceptance-evidence) define tested versions and
limits. Schema numbers in migration history describe that migration, not the
current schema. Commands run from the repository root unless noted otherwise.

| Area | Guides |
| --- | --- |
| Daily task work | [Continuity and resume](CONTINUITY-SETUP.md), [TUI](TUI-SETUP.md), [progress and decisions](PROGRESS-SETUP.md) |
| Evidence and files | [Evaluators](EVIDENCE-SETUP.md), [artifact identity](ARTIFACT-SETUP.md), [manual preservation](PRESERVATION-SETUP.md), [dependencies](DEPENDENCY-SETUP.md) |
| Agent access | [MCP](MCP-SETUP.md), [scoped credentials](SCOPED-ACCESS.md), [S2a action records](../S2A-IMPLEMENTATION.md) |
| Editor and terminal | [Neovim](NEOVIM-SETUP.md), [Herdr](HERDR-SETUP.md), [workspace declarations](WORKSPACE-SETUP.md) |
| Desktop recovery | [Hyprland observation](HYPRLAND-SETUP.md), [snapshots](SNAPSHOT-SETUP.md), [preview](WORKSPACE-PREVIEW.md), [operations](WORKSPACE-OPERATIONS.md), [application recipes](APPLICATION-RECOVERY.md), [verification](../RECOVERY-VERIFICATION.md) |
| Browser | [Setup](BROWSER-SETUP.md), [window pairing](BROWSER-PAIRING.md), [readback](BROWSER-VERIFICATION.md), [shared actions](ACTIONS-SETUP.md), [wire protocol](BROWSER-PROTOCOL.md) |

The browser GUI is retired; use the TUI. New hooks/conversation sensors belong to
S1, automatic login recovery to W08/S2b, retrieval to S3 and planning/notifying to
S4. These guides do not establish those future capabilities.
