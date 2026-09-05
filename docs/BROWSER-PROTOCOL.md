# Browser transport v1 — implemented scope

This slice supplies metadata observation and explicit browser commands. Conversation bodies, native-app content, Hyprland pairing and workspace restoration remain separate acceptance gates.

Chrome launches the native host, which validates framed messages and forwards only `/browser/message` using a dedicated browser credential. That credential cannot use the task-command API. The browser never receives either daemon credential. Native framing follows [Chrome's documented protocol](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging).

Every message has `v:1`, `type`, `id`, `profile`, `epoch`, and `connection` (identities are 32 lowercase hex characters). Profile persists in `storage.local`; epoch and sequence persist in browser session storage; connection changes on native-port reconnect. Go's strict `internal/browser/protocol.go` decoding/validation is the executable wire schema, exercised by protocol and cross-process fixtures. No content-script ingress exists in this slice.

| Message | Additional fields | Reply |
|---|---|---|
| `hello` | label, extension_version | welcome or pairing_required |
| `inventory` | seq, observed_at, tabs, focused_window, complete | ack after committed observation |
| `poll` | none | commands or pairing_required |
| `command_result` | result: {operation_id,status,tab_id?,window_id?,url?,detail?} | ack after committed result |

Inventory is at most 240 KiB before framing; transport refuses frames above 256 KiB. Oversize metadata snapshots are explicitly incomplete, so missing tabs do not imply closure. No transcript chunk assembly is claimed in this metadata slice. Existing tab/window IDs are scoped to a browser epoch. A new epoch retires old instance identity; stale messages cannot overwrite current state.

Unpaired profiles register only their label/identity, without tab metadata. The popup shows the profile ID; `heimdall browser pair PROFILE` is explicit local authorization. `browser unpair PROFILE` stops further observations/commands. Native host setup also restricts the exact extension ID. A local same-user malicious program is outside the native-browser credential boundary.

Commands originate from authenticated CLI requests. Open creates an extension-owned nonce page in a new browser window, then navigates. Other actions require a matching epoch, managed tab, owner operation ID and expected current URL. Only http/https URLs without embedded credentials are accepted. No shell/file/JavaScript execution or arbitrary close-by-title operation exists. The extension writes a pending operation journal before action; uncertain retries return `uncertain`, never repeat the mutation. Expired commands are not executed. Closing a tab is an explicit request; an API error/refusal is retained as such.

The outbox is IndexedDB-backed and capped at 32 MiB of serialized UTF-8 records/24 hours. Durable acknowledgement removes committed items; pause, received unpairing, expired records and retired epochs also discard queued records. Queue loss is visible. Within an already-paired browser session, disconnected observations continue to queue and reconnect reconciles a fresh snapshot; a browser restart requires a fresh handshake before collecting. Tab events are coalesced into snapshots, so this does not claim uninterrupted attention history. Reconnect uses exponential backoff with jitter capped at 30 seconds plus an alarm. Reverse commands use a two-second poll over the established native port.

Transport request IDs deduplicate exact requests at the daemon. A reconnect wraps queued records in the new connection and a new request ID; inventory sequence rejects already-committed snapshots. Operation IDs and the extension's session journal prevent repeated browser side effects; a previously finalized operation's duplicate result is discarded. A late result for an expired operation records the reported outcome with a late-result detail, without re-executing it. Event order is daemon-assigned; a session-persistent outbox order keeps results ahead of snapshots queued afterward. `succeeded` means the browser API accepted an operation, not that a remote page loaded successfully or an application saved its contents.

Unpairing cancels pending daemon operations; a command already handed to the browser can be in flight. Pausing takes effect at the next worker cycle and preserves the journal for results of actions already attempted. Neither action purges past events. Daemon `last_observed_at` and inventory completeness describe the last observation; they are not a continuous online heartbeat.

The development host is a copy of the same Heimdall executable named `heimdall-browser-host`, with a sidecar config. This avoids shell quoting/launcher dependencies. Setup uses exact extension origins and generates Unicode-safe Windows registration files. The extension requests only `nativeMessaging`, `tabs`, `storage`, and `alarms`; site permissions and `scripting` are deferred until content adapters exist.

The host-to-daemon leg reuses authenticated loopback transport from the core release with a separate browser-only role; private Unix IPC is deferred. Setup generates native-host artifacts and registration instructions. Building/testing does not register the extension in a user's normal profile. Native host names and keys are development identities, not a published store release.
