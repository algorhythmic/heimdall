# P0 verification — revision 4

Work started from clean `2f11175`, schema 20. No applicable AGENTS.md was found
in the repository or its ancestors. The handoff was read in full and adopted at
[docs/design](design/HANDOFF-heimdall-v1-r4.md), with both original SVGs unchanged.
This P0 change contains no S2a/later implementation. Daily-use gates remain
open; the operator subsequently authorized committing/pushing this work and
proceeding to S2a without waiting for those machine-dependent gates.

## Acceptance and evidence

| Criterion | Result |
|---|---|
| W07 recovery report time includes fresh observations | Fixed the later-start/earlier-origin calculation in recovery and browser readback. Regression tests include a delayed supplied time; future/stale observations and monotonic lease expiry remain rejected. The close and delayed-time tests pass 100 repetitions. |
| r4 roadmap reconciliation | STATUS, BACKLOG and README use P0 → S2a → S1 → S2b → S3 → S4 → S5. Historical plans and baseline status/backlog are retained under design/history. C17–C20 are retired. Planned markers remain 21 for S2a, 22 for S1. |
| Evaluator environment | Existing platform essentials plus §9.2 names only; optional named `env` entries are validated, inherited, digested and revalidated. Tests exercise secret exclusion and invalidation after a named value changes. Output remains digest-only. |
| YAML materialization | Atomic resource.bound → contract.accepted → evaluator.accepted, actor cli and materialized_from provenance; target/step and ancestor scopes, reuse, predecessor chains, no-op saves, explicit override and rollback covered. A normalized event golden and replay equality are checked. Saving never executes a check. |
| Real test.exit without JSON definition files | `scripts/p0-smoke.cjs` edits only tasks.yaml, syncs, and explicitly evaluates `go test ./internal/checks/...` on this checkout. Evidence is matched with exit code 0; the task stays active. Replay and daemon restart preserve state. |
| Browser inventories | Online worker deltas carry changed tabs, explicit removals and the prior sequence. Daemon snapshots bound pairing, epoch, reconnect/runtime change and daily intervals; replay rebuilds Tabs from snapshot plus deltas. Gaps and old epochs require a snapshot. Readback/ownership consumers continue using Tabs. |
| Synthetic event volume | 100 public tabs and 300 successive title changes plus one close: about 452 KB serialized events, including command receipts, below 1 MB. This is a reproducible synthetic workload, not the ordinary-day gate. |
| Browser compatibility | Isolated real Chromium native-host acceptance passes: explicit pairing, challenged readback, exact URL redirects, ownership, focus/move/closure, real daemon kill and retained-result recovery without duplicate tabs. Worker acceptance passes pause/resume, offline IndexedDB buffering and reconnect. |
| Ordinary daily profile paired | Pending operator confirmation and deployment; see below. |
| Ordinary browsing day below 1 MB | Pending a measured day on the confirmed daily profile. |

Go tests, vet, build, historical schema/action golden replay, extension unit
tests and protocol TypeScript checks pass. The existing compiled explicit CLI
evidence fixture also passes with the documented Node 24 runtime; the default
Node 26 executable exceeds the unchanged 128 MiB evaluator executable limit. Socket-based tests require execution outside the restricted sandbox;
no approval-review rejection occurred. Browser fixtures use the already installed
`.tools/playwright` Chromium cache and isolated profiles, not the daily profile.

## Baseline audit and scope clarifications

The tree confirms schema 20; extension 0.6.0; Go 1.25/toolchain 1.27.1; the direct
module list; the action/workspace/recovery, continuity, evidence and MCP packages;
and missing hooks/conversations/planner/notifier/Braid adapter. Historical claims
about external platform acceptance remain historical unless tested here.
The handoff's extension 0.5.0 section label, Neovim contrib location, absent bare
`state` command, action `succeeded` value and blanket green W07 claim were stale.
Neovim is in integrations/nvim; state is built; the live action result is
api_reported. These corrections are recorded in the adopted handoff.

P0 uses browser.inventory_delta for container updates. Observed-surface IDs,
surface events and focus spans remain S1, resolving §13's overlap with §12.
The existing bounded offline inventory outbox is retained; daemon ingress reduces
its full censuses to deltas after the reconnect snapshot. The resource observer
is shared through internal/resourceobs, retaining continuity.Observe unchanged.

P0 keeps marker 20 as planned. New additive event/definition shapes require this
P0 binary; use a pre-P0 backup for rollback to the baseline binary. No machine
installation or existing user database was upgraded by this work.

## Daily-browser blockers and inspected settings

Read-only inspection found `chromium.desktop` as the default browser and one
Chromium profile, `/home/david/.config/chromium/Default`. Developer mode is off;
the Heimdall extension is not loaded. The current chromium-flags.conf loads three
Omarchy extensions, which must be preserved. The native-host directory contains
Omarchy hosts but no dev.heimdall.browser entry. No default Heimdall data directory
exists. Omarchy is packaged at `/usr/share/omarchy`; its files were not changed.
The self-test uses the Heimdall repository, as §9.2 specifies, so it needs no
separate Omarchy source checkout.

Before daily deployment, confirm the actual daily build/profile and the persistent
Heimdall data directory. Then prepare `browser setup` artifacts at a stable path,
register only dev.heimdall.browser.json in the confirmed browser's native-host
directory, load the extension while preserving the existing extension list, pair
the profile ID observed through the native host, and measure one normal day.
No browser profile, machine setting, service, native-host registration or pairing
was changed while that confirmation remained pending.

## Reproduce

Use the repository Go toolchain/cache if Go is not on PATH:

```sh
export PATH="$PWD/.tools/go/bin:$PATH"
export GOCACHE="$PWD/.tools/go-cache"
export GOMODCACHE="$PWD/.tools/go-mod"
go test ./...
go vet ./...
go build -o bin/heimdall ./cmd/heimdall
node --test extension/test/*.test.js
node scripts/p0-smoke.cjs
PLAYWRIGHT_BROWSERS_PATH="$PWD/.tools/playwright" node scripts/worker-smoke.cjs
PLAYWRIGHT_BROWSERS_PATH="$PWD/.tools/playwright" node scripts/browser-verification-smoke.cjs
```

The ordinary-day gate should compare serialized event bytes over a timestamped
24-hour interval, including receipts; record tab count, changes, reconnects and
coverage gaps alongside bytes. Idle polls have no event rows. No synthetic test
can establish the user's ordinary-day volume.

Final retained acceptance artifacts (ignored local fixture directories):

- `.tools/p0-test-pytbBc`: matched evidence `9252f0a6c37a13c83dd7ffab64928494`.
- `.tools/browser-verification-test-9ISOUU`: Chromium 151.0.7922.34 native-host
  acceptance, event export and database backup. Refused before-unload closure
  remains uncertain/unknown, never a false matched result.
- `.tools/evidence-test-4eAUDN`: existing explicit configuration, invalidation,
  revalidation, retry and ratification acceptance under Node 24.20.0.
