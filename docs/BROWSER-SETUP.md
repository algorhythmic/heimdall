# Browser setup — development build 0.2.0

Run the extension and daemon together. Chrome/Edge launches the small native helper as needed. Braid remains a separate retrieval component; neither Braid nor the task engine is bundled into the extension.

## Windows

From the Heimdall project, choose a private, persistent local data directory. Keep the daemon running in its terminal:

```powershell
.\bin\heimdall.exe init --data-dir "$env:LOCALAPPDATA\Heimdall\data"
.\bin\heimdall.exe start --data-dir "$env:LOCALAPPDATA\Heimdall\data"
```

In a second terminal, prepare the native helper in its final location. The output directory must be empty; use a new version directory for an upgrade.

```powershell
.\bin\heimdall.exe browser setup --extension-id lffmpcoiimmjmacdbgnnjnegplmhiaic --output "$env:LOCALAPPDATA\Heimdall\browser-host-0.2.0" --data-dir "$env:LOCALAPPDATA\Heimdall\data"
```

Inspect the generated `host-config.json`, `dev.heimdall.browser.json`, and `SETUP.txt`. Import `register-chrome.reg` for Chrome or `register-edge.reg` for Edge. This registers a current-user native host; it does not install a background service. Do not move the host directory afterward. Registration is not performed by the build or setup command.

Open the browser's extension management page, enable developer mode, select **Load unpacked**, and choose the project's `extension` folder. Alternatively extract `bin/heimdall-extension-0.2.0.zip` and select the extracted directory containing `manifest.json`. Keep that directory in place. The included public development key fixes the ID at `lffmpcoiimmjmacdbgnnjnegplmhiaic`; verify the displayed ID matches. This is not a signed Web Store release.

Open the Heimdall popup. It displays a profile ID and pairing command. Run that command with the same data directory:

```powershell
.\bin\heimdall.exe browser pair PROFILE --data-dir "$env:LOCALAPPDATA\Heimdall\data"
.\bin\heimdall.exe browser status --data-dir "$env:LOCALAPPDATA\Heimdall\data"
```

After pairing, ordinary HTTP(S) tab titles/URLs and active/focused state are recorded locally. URLs may include query strings. Page bodies, conversation text, private tabs, browser settings, and file pages are excluded. Popup pause stops collection/actions at the next worker cycle and discards unsent observations. Unpair with `browser unpair PROFILE`; past committed observations remain in the event log. No selective event-purge command is available yet. Use a suitable development data directory.

## Managed browser commands

Read the profile and its current epoch from `browser status`. Use them explicitly:

```powershell
.\bin\heimdall.exe browser open --profile PROFILE --epoch EPOCH --url https://example.com/ --data-dir DATA
```

The initial result is `pending`, with an operation ID. `browser status` shows its eventual result and managed tab ID. Follow-up actions require the exact currently observed URL and a tab created by Heimdall:

```powershell
.\bin\heimdall.exe browser navigate --profile PROFILE --epoch EPOCH --tab TAB --expected-url https://example.com/ --url https://example.org/ --data-dir DATA
.\bin\heimdall.exe browser focus --profile PROFILE --epoch EPOCH --tab TAB --expected-url https://example.org/ --data-dir DATA
.\bin\heimdall.exe browser move --profile PROFILE --epoch EPOCH --tab TAB --expected-url https://example.org/ --window WINDOW --data-dir DATA
.\bin\heimdall.exe browser close --profile PROFILE --epoch EPOCH --tab TAB --expected-url https://example.org/ --data-dir DATA
```

Wait for updated inventory between navigation and subsequent commands. Redirected pages may have a different URL. Commands expire after 30 seconds; actions already attempted are never blindly repeated after interruption. An `uncertain` result requires inspection. `succeeded` confirms the browser API call, not page load or document-save completion. Normal browser confirmation/refusal behavior still applies to closing pages.

## Troubleshooting and upgrades

- **Native host not found:** check the matching browser registry file, extension ID, fixed manifest path and helper executable path. Reload the extension after correcting setup.
- **Daemon unavailable:** run `doctor` with the configured data directory; then start the daemon there. The helper discovers its rotated endpoint credential after restart.
- **Pairing required:** pair the popup's profile ID locally. A different browser profile has a different ID.
- **Stale epoch/URL:** refresh `browser status`. Commands cannot transfer ownership across a browser restart or control unrelated user tabs.
- **Paused or gap:** use the popup. A gap indicates outbox retention/epoch loss; snapshots do not provide complete historical activity coverage.

For upgrades, stop the daemon and copy its data directory as a backup, replace its executable, prepare a new versioned host directory, update the browser's native registration and reload the extension. Build 0.2.0 upgrades the database marker from 1 to 2 while retaining existing events/tasks; the old core executable then refuses that directory. Rollback requires the old backup, not lowering the marker. Keep the development manifest key to retain its extension ID. To remove the integration, unpair profiles, remove the extension, and remove only the `dev.heimdall.browser` entry beneath the chosen browser's current-user `NativeMessagingHosts` registry key. Existing task/event data is separate.

## Linux and acceptance boundary

The Linux binary is cross-built, not executed in this verification run. Run its `browser setup` command to produce a platform-native helper and manifest; place the manifest under the appropriate Chrome/Chromium `NativeMessagingHosts` directory following `SETUP.txt`. Host discovery for alternate profiles, distro wrappers and managed browsers needs local validation. No login service, browser auto-launch, Hyprland pairing or restoration has been implemented.

Automated verification covers real Chromium APIs, compiled native framing, pairing, metadata, commands, offline buffering, pause/resume and daemon recovery. The worker integration harness replaces Chrome's OS native-host discovery with a test port; installation into a normal Chrome/Edge profile remains an explicit deployment acceptance step. See [VERIFICATION.md](VERIFICATION.md).
