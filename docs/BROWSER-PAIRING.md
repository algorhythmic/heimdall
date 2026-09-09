# Browser window pairing — extension 0.5.0 / schema 18

C13 now joins an explicitly authorized browser action to the selected Hyprland
window. This establishes the outer window's task/surface association. It does not
adopt unrelated tabs or authorize workspace restoration. Browser restore recipes
and fresh membership verification remain W05–W07 work.

## Run an explicit pairing

Use the existing [browser setup](BROWSER-SETUP.md) and [Hyprland source
selection](HYPRLAND-SETUP.md). The selected source must still have the same epoch.
A browser surface must be present in the task's current reviewed manifest. The
extension advertises action, verification and pairing protocol 1. Version 0.4
continues ordinary browser actions but cannot receive pairing requests.

Read the current pins with `./bin/heimdall action context alpha` and
`./bin/heimdall viewport status`. Save an action request using version **2**:

```json
{
  "version": 2,
  "id": "11111111111111111111111111111111",
  "target": "alpha",
  "expected_task_revision": 1,
  "manifest_id": "22222222222222222222222222222222",
  "surface_id": "33333333333333333333333333333333",
  "context_digest": "4444444444444444444444444444444444444444444444444444444444444444",
  "browser": {
    "profile": "55555555555555555555555555555555",
    "epoch": "66666666666666666666666666666666",
    "action": "open",
    "url": "https://example.com/",
    "load_condition": "complete",
    "pairing": {
      "version": 1,
      "source_id": "77777777777777777777777777777777",
      "source_epoch": "8888888888888888888888888888888888888888888888888888888888888888",
      "previous_viewport": "none"
    }
  }
}
```

Replace the synthetic IDs/digests with the current values; generate a new request
ID once, then retain the file for exact retries. Submit with
`./bin/heimdall action queue alpha --file request.json`. Inspect with
`action show alpha --id ACTION_ID`, `action history alpha --id ACTION_ID`, and
`viewport list alpha`.

For an existing **task-owned** tab, use `action: "associate"`, `tab_id`,
`window_id`, `owner_id` (its original shared open action), and `expected_url`.
Omit `url` and `load_condition`. Set `previous_viewport` to the current binding ID
when rebinding. Arbitrary existing user tabs are not adopted by this command.

## Observations and delivery

1. The shared action intent and first delivery are committed before input.
   The extension journals creation before opening a temporary `pair.html#ACTION_ID`
   page. Open creates a new window; associate creates a temporary active tab in
   the explicitly selected owned tab's window.
2. A challenged stable browser read identifies the exact temporary tab/window.
   A fresh compositor inventory must contain exactly one nonce title. The first
   probe is retained, forcing the next browser challenge to follow that event.
3. A second browser read must retain the connection, event generation and marker.
   A second compositor inventory must name the same stable native identity and
   unique title. Browser and native leases are checked again before the atomic
   association, viewport binding and action transition commit.
4. A separate continuation delivery is committed before it reaches the extension.
   The extension journals that continuation independently, checks the exact marker
   URL/window/activation and original owned tab again, then navigates the new
   window or removes only the temporary association tab.
5. A new challenged read independently verifies the requested final browser
   postcondition. API settlement and verification remain separate; no task is
   completed automatically.

Exact native title forms are pinned to Linux Chromium and the tested Chrome for
Testing distribution: `Heimdall pairing ACTION_ID - Chromium` and
`Heimdall pairing ACTION_ID - Google Chrome for Testing`. Ordinary titles, class,
PID, similar URLs, and restored session IDs confer no ownership. Other products
and translated title formats fail closed until separately supported.

Pairing attempts serialize globally, and an unresolved pairing blocks other
browser actions for that profile. The original 30-second deadline applies to
both phases. At most three native probes are retained per attempt. The browser
nonce/readback and association delivery guards expire after five seconds.

A reconnect before continuation delivery can collect a new association within
the original deadline and probe budget. A recorded continuation delivery is
never redelivered under a new poll or new continuation ID. Lost acknowledgments
recover only retained results. Storage failure after input remains uncertain;
replay is inert. Cancellation stops new input. Closing the untouched marker
before any continuation delivery allows complete challenged absence to release
the attempt. A marker changed into user content is left open. Uncertain attempts
retain their holds for explicit recovery; a new action ID is not a retry.

Viewport and snapshot reads omit temporary pairing windows and strip browser
page title, class and PID from scoped owned-window metadata. Ownership of the
outer window is not permission to save every tab's content. Browser recovery
previews continue to require a reviewed recipe and fresh membership checks.

## Compatibility and verification

Schema 18 preserves previous events, receipts and credentials and publishes a
`pre-schema-18` backup before upgrading. Old executables refuse the new marker.
The extension keeps the existing development ID. No global native-host
registration or normal browser profile is modified by the acceptance harness.

`node scripts/browser-pairing-smoke.cjs` uses actual Linux Chromium native
messaging and synthetic read-only compositor IPC. Set
`HEIMDALL_PAIRING_HEADFUL=1` for the selected local Hyprland compositor with a
disposable browser profile. The current tested versions are Chromium
151.0.7922.34 and Hyprland 0.56.2. Fake-observer tests cover ambiguity, browser
ABA events, source replacement, cancellation, ordered reads, restart before and
after delivery, exact continuation provenance and inert replay. Extension tests
inject failures before/after journal writes and refuse changed marker URLs,
windows, activation and original-tab ownership.

These checks do not establish Windows/Edge native association, browser-reboot
adoption, recovery of application data, or VM power-loss durability.
