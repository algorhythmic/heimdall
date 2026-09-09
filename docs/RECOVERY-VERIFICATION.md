# Fresh recovery verification (W07)

`workspace verify` reports what is independently observable now. An action's
`matched` result is not a full recovery claim. W07 checks each selected surface
and aggregates its existence, exact ownership, current task membership, named
workspace membership, usable display placement, supported window/application
state and outstanding action uncertainty.

```sh
heimdall workspace verify alpha
heimdall workspace verify alpha --operation OPERATION_ID
heimdall workspace verify alpha --snapshot SNAPSHOT_ID --output /tmp/recovery.json
```

Without an operation, the selection is every surface in the current manifest,
against the selected saved point or current complete snapshot head. With an
operation, verification uses its original surface selection and saved point
(the pre-close point for a close). To inspect the outgoing side of a swap,
provide that task explicitly with the same operation ID. The aggregate cannot
be full while the overall operation remains partial, cancelled or unresolved.
An empty selection never counts as recovery. Optional surfaces are still checked
when selected; `required` is shown, not used to hide failures.

The command observes applications: it performs no launch, movement, resize, closure,
adoption, session input, task completion or capacity release. A saved JSON report
is a private, no-overwrite file, not a request token. The database remains schema
20: no new event family, reducer, stored success flag or migration is introduced. Reports
are not automatically persisted; export one when a retained diagnostic is needed.
Browser verification requests a bounded fresh census, including for settled
actions. It reuses the existing challenge/readback observation journal and live
monotonic freshness leases; it creates no new action or outbox operation. The
request-only demand expires within five seconds and cannot survive restart.
Replay cannot invoke verification. The local CLI authority is required; browser
and scoped agent credentials gain no new route access.

## Outcomes and evidence

| Outcome | Meaning |
| --- | --- |
| `verified`, `full: true` | Every selected supported postcondition matches, with current complete coverage and valid saved/operation scope. |
| `degraded`, `full: false` | Observable view recovery has a stated limitation, such as generic terminal process continuity or an explicit monitor fallback. |
| `partial`, `full: false` | At least one observed postcondition does not match, such as a wrong workspace, offscreen window or failed close. |
| `unknown`, `full: false` | Required evidence is missing, stale, unsupported or may still change because an action is unresolved. |

Per-check statuses are `matched`, `not_matched`, `unknown`, `unsupported`,
`degraded` or `not_required`. A failure takes precedence over uncertainty in the
aggregate; both remain visible per surface. Reports include immutable scope IDs,
the compositor observation ID and capture interval, applicable browser challenge
and sequence, session-check time, current binding/action references, input digest,
report digest and an expiry. They omit window titles, executable classes, PIDs,
foreign surfaces and page text. The report digest identifies content; it is not
an authorization signature.

Native coverage must be fresh within two seconds on the explicitly selected
source/epoch. Application-created windows additionally require the original
PID/start time and unique attempt class to agree with the observed binding.
Browser membership requires a complete stable challenged census from the current
profile/epoch/connection after the latest relevant action, received within five
seconds and before challenge expiry. Report expiry is bounded by both sources.
Browser observation waits up to four seconds and bounds its current-profile
action reference census to 128; pause, disconnect, unavailable capability or
excess history cannot turn into success.
Bindings/recipes/task/operation inputs are rechecked after observation; concurrent
changes refuse publication instead of producing a mixed-scope report.

These are bounded point-in-time observations, not an atomic desktop transaction
or a promise of continued availability. Re-run verification after an interruption
or after acting on the application. Repaired event gaps remain diagnostic history;
unavailable current coverage never counts as success.

## Placement and fallback

The default `saved` policy requires the saved uniquely named workspace and monitor,
unchanged monitor scale/transform/geometry, matching supported window flags, and
usable global logical coordinates. Floating geometry must match the saved point.
For tiled views, the check requires usable bounds and membership, not identical
split ratios. Numeric workspace/monitor IDs never substitute for saved names
across epochs. Hidden/offscreen views and wrong workspace/display membership fail.

When the saved monitor is missing or its geometry has changed, the default
reports review required. An explicit fallback can define an alternative expected
placement for an ordinary saved floating window:

```sh
heimdall workspace verify alpha \
  --placement-policy named-monitor-clamp --fallback-monitor DP-1
```

The policy preserves the window's offset from the old display origin, maps that
offset onto the named display, bounds its size and clamps it inside the new
logical rectangle. Workspace membership must still match. It does **not** move
or resize the application: until independently observed geometry matches that
expectation, placement is not matched. A matching fallback is explicitly labeled
degraded and includes `fallback_applied` and the expected geometry. Tiled,
fullscreen, hidden and pinned fallback is unsupported; there is no implicit
"first monitor" policy.

The placement contract covers full logical display rectangles and observable
window flags. Reserved panel/work areas, decorations, occlusion, exact tiled
split trees, application responsiveness and pixel-identical restoration are not
asserted by `full`. No automatic layout-restoration dispatcher is added in W07.

## Application boundaries

- Native views can verify their supported existence/membership/window contract.
- Generic terminals remain degraded: reopening a view does not resume prior
  processes. A PID/start-time mismatch makes ownership unknown.
- Herdr readback checks the exact bound session and original pane process.
  Wrong/missing session evidence blocks recovery. A surviving session alone does
  not prove this window renders its attachment, so open recovery stays unknown.
  A closed attach view can separately verify that the original session survived.
- Editors remain unsupported for full buffer/cursor/unsaved-state verification.
  A clean Neovim saved-file launch is not evidence that those states were restored.
- Browser checks require one exact currently owned tab, the reviewed committed
  URL, completed non-discarded load, and membership in its explicitly paired
  native window. Moved, self-restored/unowned or filtered tabs are not adopted.
  Close requires the complete exact-ID census; native-window absence alone is
  insufficient. If other tabs leave the outer window open, the result is degraded
  and those tabs remain untouched.

The TUI workspace dialog (`p`, then `r` to refresh) includes the fresh aggregate
and per-surface limitations. CLI operation verification gives the more detailed
close/swap/action-specific view. See [application recipes](APPLICATION-RECOVERY.md)
and [verification evidence](VERIFICATION.md). W08 startup/readiness and controlled
compositor/reboot/VM interruption acceptance remain separate work.
