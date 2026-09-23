---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002
created: 2026-07-09
updated: 2026-09-23
owners:
  - tbd
---
# LSP File Intelligence System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001` | [Migrated source detail](#migrated-source-detail) |
| `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002` | [Browser-independent leases](#browser-independent-leases) |

## Migrated source detail

## Why

Users inspect and edit code inside Kandev task file tabs, but code navigation and analysis otherwise require opening an external editor. Lightweight language-server intelligence lets users understand a project without leaving the task.

## What

- Desktop Monaco file editors can connect to Language Server Protocol servers for:
  - TypeScript and JavaScript via `typescript-language-server`
  - Python via `pyright-langserver`
  - Go via `gopls`
  - Rust via `rust-analyzer`
  - Kotlin via the official `kotlin-lsp`; Kotlin is marked experimental while its upstream server is alpha
- Wired editor capabilities are diagnostics and the server-advertised completion, hover, go-to-definition, references, signature-help, and semantic-token providers.
- Global editor settings select languages that auto-start, languages Kandev may auto-install, and per-language configuration returned through `workspace/configuration`. Saving changed configuration updates the existing server through `workspace/didChangeConfiguration` without waiting for an idle disconnect or process restart.
- A user can manually start or stop the current file's server from the effective status control. Manual enablement is remembered in browser local storage for that session and language. An explicit Stop suppresses global auto-start for that session and language in the current browser runtime until the user explicitly starts it again; it does not rewrite the global preference or affect another browser window.
- The current editor's LSP status surface distinguishes process launch, protocol readiness, and server-reported project work:
  - after the task host reports `ready`, it says that the server process started and that Kandev is waiting for the LSP `initialize` response;
  - while initialization is pending, it shows locally measured elapsed time without treating that time as server-reported indexing progress;
  - after 60 seconds without an initialize response, it says initialization is taking longer than usual, keeps the connection alive, and retains the Stop action;
  - Kotlin's long-running state explains that Kotlin LSP may be importing a Gradle project and that cross-file features remain unavailable until initialization completes, without promising an ETA;
  - when the server reports standard LSP work-done progress, it shows the server title, optional message, optional percentage, elapsed time, and the number of concurrent work items;
  - when the server reports no work progress, it says so instead of inventing an indexing state, percentage, or time remaining.
- The LSP status location is a portable editor preference:
  - `toolbar` is the default and keeps the control beside the current Monaco editor's actions;
  - `status_bar` moves the active Monaco editor's control and live summary into the application status bar when **Show status bar** is on for a fine-pointer layout;
  - if **Show status bar** is off or the layout uses a coarse pointer, Kandev falls back to the editor toolbar without overwriting the saved preference;
  - the status-bar item follows only an active, mounted Monaco text editor's session and language and is not a global dashboard of every live server; loading, binary/static, diff, CodeMirror, unsupported, and non-file panels do not expose it.
- The effective status control remains disclosure-first:
  - fine-pointer toolbar and status-bar controls open the same anchored progress popover;
  - coarse-pointer Monaco layouts use the toolbar and open an inset bottom drawer with the same status, progress, and Start, Stop, or Retry action;
  - phone file viewing remains LSP-free and does not render the control.
- A connected server remains connected while project work is active. Progress never replaces the ready connection state or disables document synchronization and editor providers.
- Progress copy warns that cross-file results may be incomplete while server-reported analysis is active. Completion means only that the reported work item ended; it does not guarantee that every reference, dependency, or project module is resolved.
- Server-reported project titles and messages remain fully readable and wrap within the LSP status surface, including URL-, path-, and identifier-like text without ordinary break points. The desktop popover and coarse-pointer tablet drawer do not clip, truncate, or horizontally overflow this text.
- Kotlin supports auto-start but not auto-install. `kotlin-lsp` must already be available on the task host's `PATH`.
- Rust auto-install is available only on supported macOS and Linux task hosts. Windows can still run a manually installed `rust-analyzer` from the task host's `PATH`; when no installer is available on the actual task host, the task-host stream reports that condition separately and the UI directs the user to manual installation even before the global preference is enabled.
- The boot runtime advertises the task-host-independent language set that may be saved as a global auto-install preference. The main backend must not filter that preference using its own OS; agentctl is the final authority because it runs on the selected task host.
- Language servers run through the task's `agentctl`, with the task workspace as their working directory. This keeps project files, dependencies, and server execution in the same environment.
- Binary discovery and npm/Go auto-install resolve commands and installation results with that same task environment, including task-provided `PATH`, `GOBIN`, `GOPATH`, `HOME`, and Windows `USERPROFILE` values. Command and Go result lookup ignore relative directory entries so repository-controlled paths cannot become executable search roots.
- The managed npm/release cache derives its absolute root from the merged task environment's `HOME`, not from agentctl's parent-process home before executor overrides are applied.
- Managed npm-server lookup resolves the concrete platform launcher, including PATHEXT-backed `.cmd` shims on Windows, both immediately after installation and on later starts.
- TypeScript/JavaScript LSP providers use a dedicated registration guard. Monaco's built-in providers are wrapped before runtime suppression begins, including when Monaco loads after the LSP handshake, and are suppressed only for models owned by the corresponding active LSP connection. Unrelated sessions and models retain Monaco's built-ins.
- Monaco file navigation handles session-scoped LSP targets and regular task-workspace `file://` targets through Kandev tabs. Targets outside the active workspace are reported as unhandled so Monaco's other opener behavior can continue.
- Completion requests translate Monaco invocation, trigger-character, and incomplete-result context into the corresponding LSP enum values and forward the trigger character when present. A server item without `textEdit` receives Monaco's current-word insertion range; explicit LSP `TextEdit` and `InsertReplaceEdit` ranges remain authoritative.
- Successful Monaco file saves synchronize every matching open language-server document to the newest editor snapshot, then notify servers that requested `textDocument/didSave`. When that live snapshot still matches the persisted snapshot, the persisted text is included only for servers that advertise `includeText`. If editing advanced while persistence was in flight, the newer buffer stays dirty and synchronized and the optional stale save text is omitted so the language server cannot be rewound. Failed saves emit no save notification.
- V1 task-host support is limited to Local PC and local Docker executors. Remote Docker, SSH, and Sprites report an unsupported-executor state.
- Each runtime LSP lease owns one language-server process. Editors in one browser window share an attachment for the same session and language; separate concurrently attached browser windows keep separate leases. An intentional editor-idle timeout releases the lease; tab closure and transport loss detach it.
- The backend caps active LSP leases, whether attached or detached, at 8 by default. The existing `KANDEV_LSP_MAX_CONNECTIONS` setting remains the compatible operator override for this resource cap. At capacity, admission of a new process releases the least recently detached lease; attached leases cannot be evicted.
- Language-server processes and npm/Go auto-install commands are owned by the existing agentctl process manager. Instance teardown cancels and drains install work, then reaps full process trees on Unix and Windows before releasing resources.
- During auto-install, the runtime lease retains the task-host WebSocket when the browser detaches. Explicit Stop or task-host teardown cancels installation; the lease consumes the first inbound frame after installation without racing the ready handshake.
- Agentctl must deliver the bounded `installing` status to the backend lease before acquiring auto-install work. If that write fails or times out, it closes the stream without starting the installer.
- Kandev-managed npm and release binaries live under the task host's `~/.kandev/lsp-servers`; `gopls` is installed through the task host's Go toolchain. No managed server cache lives inside a checked-out project.
- LSP JSON-RPC bodies are limited to 16 MiB across stdio and WebSocket transport; stdio headers are bounded separately. Oversized frames close the affected connection instead of allocating unbounded memory.
- Every task-host LSP WebSocket write has a five-second deadline, including installing, installed, failure, ready, close, and bridged JSON-RPC frames. The backend lease continuously drains that stream, including while no browser is attached; a stalled browser attachment is detached without blocking the server reader.
- Each browser-to-server stdio frame has a 30-second write cutoff, and stdout-forwarder termination closes stdin immediately. A language server that stops reading cannot leave the bridge handler or its owned process pinned indefinitely.
- Mobile file viewing does not start language servers in the background.

## User settings

Existing user-setting fields are the durable global policy:

| JSON field                   | Type                    | Meaning                                                                                                                              |
| ---------------------------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `lsp_auto_start_languages`   | `string[]`              | Languages that connect when a matching file opens.                                                                                   |
| `lsp_auto_install_languages` | `string[]`              | Languages Kandev may install when their server binary is missing. Kotlin and platform-unsupported installers are rejected.           |
| `lsp_server_configs`         | `object`                | Per-language JSON returned through `workspace/configuration` and pushed to a live server through `workspace/didChangeConfiguration`. |
| `lsp_status_location`        | `toolbar \| status_bar` | Preferred LSP status surface. Missing or invalid values normalize to `toolbar`; runtime capability fallbacks do not rewrite it.      |

There is no durable per-task or per-session LSP policy in V1. Manual toolbar state is browser-local and does not override another browser window.

## API surface

### Browser-facing stream

`GET /lsp/:sessionId?language=<language>`

The main backend resolves or restores the task execution, checks executor support and global capacity, and authenticates to that execution's agentctl instance. With browser continuity enabled, the broker owns the upstream and attaches the browser to a lease; with it disabled, the route retains its current WebSocket proxy behavior.

### Task-host stream

`GET /api/v1/lsp/stream?language=<language>&autoInstall=<bool>`

This authenticated agentctl route resolves or installs the binary, starts it in the task workspace, converts WebSocket JSON messages to LSP stdio framing, and converts LSP stdio responses back to WebSocket messages.

Before JSON-RPC traffic begins, the task-host stream can emit:

```json
{ "status": "installing", "language": "python" }
{ "status": "installed", "language": "python" }
{ "status": "ready", "workspacePath": "/abs/task/worktree" }
{ "status": "install_failed", "language": "python", "error": "..." }
```

Application close codes are:

| Code   | Meaning                                                                    |
| ------ | -------------------------------------------------------------------------- |
| `4001` | Auto-installable server binary missing and auto-install was not requested. |
| `4002` | Session, execution, or agentctl stream unavailable.                        |
| `4003` | Auto-install failed.                                                       |
| `4004` | Executor unsupported in V1.                                                |
| `4005` | Active LSP lease cap reached.                                              |
| `4006` | Language server exited unexpectedly.                                       |
| `4007` | Server binary missing and the task host has no supported installer.        |
| `4008` | Language-server process failed to start.                                   |
| `4009` | LSP transport or broker connection failed while the server may still run.  |

The browser translates categorical close statuses from the close code instead of rendering transport prose. A preceding `install_failed` status payload may retain its actionable task-host diagnostic when the stream then closes with `4003`; without that payload, `4003` uses the localized installation-failure fallback. A JSON-RPC initialization rejection preserves the server-provided `error.message`.

### Browser LSP progress contract

The browser advertises standard LSP `window.workDoneProgress` support and includes a client-generated `workDoneToken` in `initialize`. This lets servers such as JetBrains Kotlin LSP report project-import phases before the initialize response.

The broker accepts both server-created progress tokens through `window/workDoneProgress/create` and `$/progress` notifications for the initialize token. Tokens can be strings or numbers and are scoped to one LSP lease and server generation; the attached browser receives the current snapshot.

Supported work-done payloads are:

| Kind     | Observable behavior                                                                              |
| -------- | ------------------------------------------------------------------------------------------------ |
| `begin`  | Adds or replaces the token with its title, message, optional percentage, and local start time.   |
| `report` | Updates only the matching active token; an omitted percentage preserves its last reported value. |
| `end`    | Removes only the matching active token and records the server's optional completion message.     |

Percentages are clamped to 0–100 for presentation. Unknown tokens, malformed payloads, and late notifications from a replaced connection are ignored.

The broker preserves LSP method semantics while mapping request IDs, tracking progress and diagnostics, and separating browser attachments from the task-host process lifetime.

## Readiness and progress state

- Connection readiness remains the existing lifecycle (`connecting`, `installing`, `starting`, `ready`, `stopping`, `unavailable`, or `error`).
- The task-host `ready` handshake means the executable has launched and the JSON-RPC bridge can begin; it does not mean the language server has completed LSP initialization.
- Initialization is locally observable from the initialize request until its response. The UI shows locale-aware elapsed time even when the server sends no progress payload.
- The 60-second long-running presentation is derived only from elapsed wall time. It does not change connection state, cancel the request, restart the server, or assert that the server is indexing.
- Work-done progress is runtime-only activity attached to a live connection. Multiple active tokens are a flat list because LSP defines no parent/child relationship.
- The oldest active work item is the primary summary; additional active items are shown as a count. Percentages from unrelated work items are never averaged.
- The most recently ended item can remain visible as “server-reported work finished” for the lifetime of that connection. It is not described as project-wide success.
- Explicit Stop, server crash, or task-host teardown clears active and completed progress. Browser detach leaves progress in the lease; reattachment receives its current snapshot. A new server generation starts without stale progress.
- Progress activity is scoped to the current editor's session and language. It is not a global task-wide language-server dashboard.

## Browser-independent leases

The main backend's LSP lease manager owns the browser-to-agentctl upstream stream. It keys a lease by the task execution, language, and an opaque lease ID; it keeps the upstream open and reads it even when no browser is attached. Agentctl still owns and reaps the actual process. The lease is runtime-only and ends on intentional editor-idle release, explicit Stop, capacity eviction, server exit, execution replacement or stop, or backend shutdown. A backend restart does not claim to preserve a process that no longer has a live upstream stream. This boundary follows [ADR-2026-09-23](../../../decisions/2026-09-23-task-owned-lsp-leases.md).

All three orchestrator paths that call `reclaimIdleSession` (the periodic idle reaper and the agent and streaming event handlers) treat an active lease as live task-runtime work, including when no browser is attached and no agent turn is running. The lease thereby keeps the whole Local PC or Docker task host, including agentctl, container, and worktree runtime resources, active. Lease admission and reclaim use the same session lifecycle fence or a generation-checked equivalent, and reclaim rechecks the lease immediately before cleanup. A new lease cannot be admitted onto an execution already being reclaimed. Explicit task stop still wins and closes all leases for that execution. Editor-idle release and capacity eviction remove the pin when the lease ends.

The existing browser-facing `/lsp/:sessionId` route authorizes the caller before attachment or lease lookup. The browser includes its lease ID when it has one. The backend reattaches to that detached lease if its execution and language still match; otherwise it may claim the newest detached lease for the same execution and language, or create a new one when admission permits. An attached lease is never stolen from another live browser window, including when a duplicated tab copies its `sessionStorage` hint. In that case the new tab receives a separate lease if admitted. The opaque ID is a routing hint, not an access credential, and is never logged. Browser `sessionStorage` can retain it across a tab reload; a new tab can still claim a detached lease without it when its current auto-start or local manual-enable policy requests LSP. A previously attached tab may resume its hinted lease after auto-start is disabled, unless it explicitly stopped LSP. A lease is counted against `limits.lspMaxConnections` from process admission until teardown, including periods without a browser. Reattachment needs no new capacity slot. Admission under a full cap first releases the oldest detached lease, waits for its upstream/process cleanup, and then admits the replacement; if all leases are attached it returns `4005` without starting a process. An evicted hint becomes a fresh start under the current browser policy.

The backend broker is the stable LSP protocol peer. It retains the first `initialize` result, workspace identity, effective per-language configuration, active work progress, dynamic `client/registerCapability` registrations, and per-URI server-facing document-version counters within a 4 MiB state budget. It applies `client/unregisterCapability` and answers both methods while detached; while attached it also forwards effective registration changes so Monaco providers update immediately. The resumed handshake includes the effective registrations so providers match the server's current capabilities. It answers `workspace/configuration` from the latest persisted user settings, scoped to the lease owner, rather than from a detached browser snapshot. A settings-save event updates affected leases and sends `workspace/didChangeConfiguration` even while detached; reattachment also reconciles the latest settings revision before reporting ready. This preserves AC .8 without keeping a browser connection open.

The broker continues reading server output while detached, answers supported server-to-client requests, and gives unsupported requests a JSON-RPC method error so they cannot hang indefinitely. It assigns server-facing request IDs and maps them to the current browser attachment generation. Browser `$/cancelRequest` maps to the corresponding server-facing ID; on detach it cancels all that generation's pending browser requests toward the server and rejects late responses. Replies to a detached generation are discarded. A browser close or write timeout detaches that generation without closing the upstream. Explicit Stop or idle release uses an acknowledged broker control frame: the broker rejects outstanding browser requests, sends LSP `shutdown`, waits within a bounded deadline, sends `exit`, then closes the upstream and waits for agentctl process cleanup. The UI cannot report Stop complete before acknowledgment. An unexpected agentctl process exit sends an explicit upstream `4006`; an upstream read failure without confirmed process exit maps to transport `4009`. Invalid WebSocket reserved close codes such as `1005` and `1006` are normalized before forwarding.

On detach, the broker sends `textDocument/didClose` for each open document, clears all diagnostic state, and drops later diagnostic publications for closed documents. It retains no document text. On reattachment, the ready handshake identifies the retained lease and supplies server capabilities, effective dynamic registrations, workspace metadata, and current progress. The browser rebuilds Monaco providers without sending a second LSP `initialize`. It reopens current documents with their current text and sends attachment-ready after handlers are registered. The broker assigns each `didOpen` and `didChange` a strictly increasing server-facing version for that URI over the lease lifetime, independent of browser-local versions, and acknowledges synchronization before the UI reports `ready`. On a resumed lease it forwards `publishDiagnostics` only for a currently open URI after synchronization and only when the publication's version equals the current server-facing version. Unversioned publications cannot be bound to the reopened text and remain hidden for that URI until a fresh server generation; initial connections may still display unversioned diagnostics. Old markers remain clear until a qualifying new publication. Two simultaneously attached windows retain separate leases, request maps, documents, and progress, preserving their current independent unsaved-buffer behavior.

`features.lspBrowserContinuity` (`KANDEV_FEATURES_LSP_BROWSER_CONTINUITY`) is a restart-required runtime release toggle, off in prod, dev, and e2e shipped profiles. The backend gates lease manager construction and the browser-facing WebSocket path; when disabled, the existing browser-owned proxy and idle-reclaim behavior remain unchanged. The frontend uses the matching boot feature value to keep its existing LSP client behavior when disabled. An enabled test installation or E2E fixture turns the flag on explicitly.

The browser manager uses bounded reconnect attempts after unexpected transport loss. During those attempts it shows a localized reconnecting state with no stale markers. A lost task host or backend starts a fresh server under the existing auto-start or manual-enable preference and clears the previous generation's progress. A confirmed server process exit uses `4006` and the server-exited state; a broker or proxy transport failure uses `4009` or the browser's abnormal-close signal and offers Retry after automatic attempts fail. The toolbar, fine-pointer status bar, and coarse-pointer tablet drawer share this state. The phone viewer neither creates nor attaches to a lease.

The broker logs lease creation, attachment, detachment, reattachment, Stop, idle release, capacity eviction, process exit, and teardown reason with bounded metadata. Dev expvar exposes `lsp_lease_active` and `lsp_lease_detached` gauges, `lsp_lease_evicted_total`, and `lsp_lease_released_total` with a closed `reason` set (`stop`, `editor_idle`, `capacity`, `server_exit`, `runtime_stop`, `backend_stop`). No metric uses task, session, browser, lease, or language IDs as labels. Capacity and active/detached counts are observable without IDs. It releases capacity exactly once on every terminal path and never lets a slow browser block task-host stdout draining.

## State and persistence

- User settings persist in the existing user-settings store.
- Manual enablement persists only in browser local storage under the session and language.
- Processes, open documents, capabilities, and progress are runtime-only. The backend lease retains bounded metadata but discards diagnostics on detach; no new durable task or user record is created.
- Save notifications are runtime-only and follow confirmed workspace persistence; they are not emitted for closed documents, failed writes, or servers that did not request them.
- A missing server starts only when a supported file is opened and auto-start or a toolbar action requests it.
- Closing a browser attachment leaves its server lease running. Explicit Stop and intentional editor-idle timeout release it; capacity admission may evict a detached lease. Stopping the task or its runtime reaps every owned language-server process even if a browser connection remains open.

## Failure modes

- **Unsupported executor:** the file toolbar reports that the task host is unsupported and no process starts.
- **Missing Kotlin server:** the UI tells the user to install `kotlin-lsp` on the task host; it does not offer or retry auto-install.
- **Missing auto-installable server:** the UI reports a localized missing-server status or shows install progress when auto-install is enabled. Editors settings keep each language's installation command, prerequisites, and destination visible beside its auto-install control.
- **Installer failure:** the UI preserves the detailed task-host installer error after the WebSocket's generic close frame so toolchain, network, and package output remains actionable.
- **Task-only toolchain:** binary lookup, installer execution, and installed-binary discovery use the task runtime environment instead of the agentctl host environment.
- **Windows npm launcher:** installation and later cache lookup return the executable npm shim selected through PATHEXT rather than an unlaunchable extensionless path.
- **Windows Go workspace:** when `GOBIN`, `GOPATH`, and `HOME` are unset, post-install discovery checks the task environment's `USERPROFILE\go\bin` for the executable written by Go's default Windows workspace.
- **Relative Go binary directory:** relative `GOBIN`, individual `GOPATH` entries, `HOME`, and `USERPROFILE` values are ignored; only absolute task-host directories may supply an installed `gopls` result.
- **Unsupported Rust installer:** Windows rejects Rust auto-install with `4007`, the UI directs the user to install the server manually, and agentctl continues to discover a manually installed `rust-analyzer` from the task environment.
- **Cold Monaco initialization:** TypeScript built-ins are wrapped before model-scoped LSP suppression is registered; the LSP provider registration guard does not depend on suppression state.
- **Capacity exceeded:** the UI reports that too many language servers are active; the backend rejects the request before starting or resuming its supported task host.
- **Process start failure:** agentctl logs the task-host execution error, closes with categorical `4008` and no transport prose, and the UI shows a localized start-failure status with Retry.
- **Server crash:** the lease ends with categorical `4006`, Monaco providers and markers are cleaned up, and the status shows the localized server-exited message with a Retry action. Only intentional Stop returns to Off.
- **Initialize rejection:** when the server rejects the JSON-RPC `initialize` request, the UI preserves its `error.message` instead of stringifying the error object as `[object Object]`.
- **No progress support or reports:** initialization still shows an indeterminate state and elapsed time; after initialize succeeds, the status surface says the server has not reported background analysis progress.
- **Initialize response is slow or never arrives:** the UI confirms that the process launched, changes to a long-running initialization warning after 60 seconds, and keeps Stop available. Kandev does not automatically kill a cold project import or claim that the server is indexing.
- **Indeterminate progress:** the UI shows the server title/message and elapsed time without a percentage or ETA.
- **Malformed or stale progress:** the client ignores the payload and preserves the current connection and valid work items.
- **Cross-file intelligence remains incomplete after ready:** the UI does not claim that the server is still indexing unless a work item is active; the status surface explains that project import, dependencies, or module resolution may require investigation.
- **Task stop:** agentctl closes process admission and reaps the language-server process tree before releasing task resources.
- **Instance teardown during auto-install:** agentctl cancels the install, removes an unpublished partial release download, drains the shared cache mutation, and reaps npm/Go descendants before releasing task resources.
- **Browser disconnect during auto-install:** the lease continues a requested installation after browser detach. Explicit Stop or runtime teardown cancels and drains it.
- **Unread install status:** if the backend lease cannot receive the initial `installing` frame, agentctl closes the stream before starting npm, Go, or release install work.
- **Stalled browser peer:** bounded writes detach that browser without blocking task-host output or terminating the language-server process.
- **Stalled language-server stdin:** stdout-forwarder termination releases an active stdin write immediately; otherwise the write cutoff closes stdin and enters owned-process cleanup after 30 seconds.
- **Unknown language:** no LSP control is shown.
