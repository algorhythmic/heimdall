# Browser readback and recovery

The first C13 slice adds independent browser postconditions and actual Linux
Chromium native-host acceptance. Current extension is **0.6.0**, schema **21**; the original verification slice used 0.4.0/schema 17. The
extension ID remains `lffmpcoiimmjmacdbgnnjnegplmhiaic`. This guide covers browser API
verification. [Nonce pairing](BROWSER-PAIRING.md) and supported
[application recovery](APPLICATION-RECOVERY.md) are also delivered; W08 startup
and interruption gates remain open.

The daemon issues a five-second nonce after the action's latest event. A paired
extension with `verification_protocol: 1` obtains two new browser inventories,
checks for intervening tab/focus events, and returns the nonce directly. Offline
outbox inventory cannot satisfy this request. Both issuance and receipt have
monotonic deadlines within the daemon; a new daemon or connection needs new
readback. A persisted observation is historical evidence, not a surviving lease. The two
reads do not make browser/user activity atomic; later drift requires a new check.

The readback includes a bounded census of non-private tab IDs as well as allowed
HTTP(S) metadata. A tab outside URL coverage still appears in the census, so its
absence from URL metadata cannot prove closure. Ownership references must match
an already issued open attempt in that exact profile and browser epoch. Titles,
duplicate URLs and browser self-restored tabs cannot establish ownership.

Heimdall derives these outcomes from the readback:

| Requested action | Matched postcondition |
|---|---|
| Open | Exact owned tab exists at the requested URL |
| Navigate | Exact owned tab has the requested committed URL |
| Focus | Exact owned tab is active in the focused browser window |
| Move | Exact owned tab belongs to the requested browser window |
| Close | Exact owned tab ID is absent from a complete, stable census |

Redirect policy is explicitly `exact`. Open/navigation requests can include
`"load_condition": "complete"` inside `browser`; otherwise the recorded condition
is `not_requested`. Completed-load verification also requires the tab not be
discarded. Pending navigation remains unknown, and the adapter refuses to act on
an old committed URL while another navigation is pending. Chrome defines both
committed/pending URLs and the unloaded/loading/complete statuses in its
[tabs API](https://developer.chrome.com/docs/extensions/reference/api/tabs).

An API report never supplies a matched outcome. `api_reported` with `not_matched`
means the requested effect was not observed. An uncertain attempt with a negative
readback remains held, because a delayed effect may still occur. Missing results,
unstable/partial observations, unpairing and epoch loss remain unknown. Earlier
uncertainty and cancellation requests stay in history even after a late result.
Closing a view establishes nothing about whether its application data was saved.

Automatic reconciliation requests at most eight readbacks per unresolved result.
An unchanged retained result does not create another transition or restart that
budget. A blocked browser API, such as a close waiting on before-unload, may stop
worker progress; the daemon's deadline still marks possible input uncertain.
Cancellation cannot retract an API call already submitted to the browser.

To request another observation of the same attempt, retain a complete request
using the revision from `action show`:

```json
{
  "version": 1,
  "id": "11111111111111111111111111111111",
  "target": "alpha",
  "action_id": "22222222222222222222222222222222",
  "expected_revision": 5,
  "reason": "Recheck after resolving the application prompt"
}
```

```bash
./bin/heimdall action reconcile alpha --file reconcile.json --data-dir DATA
./bin/heimdall action show alpha --id ACTION_ID --data-dir DATA
./bin/heimdall action history alpha --id ACTION_ID --data-dir DATA
```

Reconciliation records an observation request; it never repeats browser input or
changes the original attempt. The task/surface and expected revision are checked,
and an unresolved conflicting attempt prevents retaking the surface. Exact
request retries return the original receipt. No browser, scoped-client or MCP
credential gains access to `/action/*`.

A new open request cannot duplicate an already owned logical surface. Within the
same browser epoch, fresh absence of the prior exact tab permits a new requested
open. A changed epoch or profile requires the later explicit adoption/recovery
workflow. A URL match is insufficient to adopt a self-restored tab, and there is
no automatic browser restart/relaunch or profile discovery here.

## Native-host acceptance

The Linux harness registers only a disposable profile's native host manifest,
using Chrome's documented [user-data-directory native-host
lookup](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging).
It copies the prepared manifest into `NativeMessagingHosts/` under its isolated
user-data directory and uses the real `connectNative` implementation. It does not
change the normal browser profile or a system registration directory.

```bash
node scripts/browser-verification-smoke.cjs
```

The harness requires Playwright and its Chromium runtime under
`scripts/browser-test`; use `PLAYWRIGHT_BROWSERS_PATH` if the runtime is installed
elsewhere. It exercises duplicate URLs, explicit redirects, completed navigation,
focus/movement/closure, and a local page that kills the daemon after the browser
side effect but before the result is delivered. Recovery must verify one existing
tab without another launch. A synthetic before-unload page also checks that an
unresolved close cannot be reported as verified closure. The harness retains its
synthetic database and events under `.tools/` and closes its test processes.

Schema 17 preserves older events, receipts, grants and unverified browser history.
Upgrade creates a consistent `pre-schema-17` backup. Older executables refuse the
new database; rollback uses that backup in a fresh directory. See
[continuity setup](CONTINUITY-SETUP.md) and [verification evidence](../VERIFICATION.md).
Actual Windows/Edge registration, normal-profile deployment, compositor pairing,
reboot/session adoption and full workspace recovery remain separate gates.

The subsequent C13 [browser pairing slice](BROWSER-PAIRING.md) delivers Linux compositor association. Earlier integration-gate notes above describe the first slice.
