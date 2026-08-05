---
status: building
created: 2026-07-09
owner: tbd
---

# LSP File Intelligence

## Why

Users inspect and edit code inside Kandev task file tabs, but code navigation and analysis otherwise require opening an external editor. Lightweight language-server intelligence lets users understand a project without leaving the task.

## What

- Desktop Monaco file editors can connect to Language Server Protocol servers for:
  - TypeScript and JavaScript via `typescript-language-server`
  - Python via `pyright-langserver`
  - Go via `gopls`
  - Rust via `rust-analyzer`
  - Kotlin via the official `kotlin-lsp`; Kotlin is marked experimental while its upstream server is alpha
- Wired editor capabilities are diagnostics, completions, hover, go to definition, references, signature help, and semantic tokens.
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
  - `status_bar` moves the active Monaco editor's control and live summary into the application status bar when that feature is enabled on a fine-pointer layout;
  - if the application status bar is disabled or the layout uses a coarse pointer, Kandev falls back to the editor toolbar without overwriting the saved preference;
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
- Binary discovery and npm/Go auto-install resolve commands and installation results with that same task environment, including task-provided `PATH`, `GOBIN`, `GOPATH`, `HOME`, and Windows `USERPROFILE` values.
- The managed npm/release cache derives its absolute root from the merged task environment's `HOME`, not from agentctl's parent-process home before executor overrides are applied.
- Managed npm-server lookup resolves the concrete platform launcher, including PATHEXT-backed `.cmd` shims on Windows, both immediately after installation and on later starts.
- TypeScript/JavaScript LSP providers use a dedicated registration guard. Monaco's built-in providers are wrapped before runtime suppression begins, including when Monaco loads after the LSP handshake, and are suppressed only for models owned by the corresponding active LSP connection. Unrelated sessions and models retain Monaco's built-ins.
- Monaco file navigation handles session-scoped LSP targets and regular task-workspace `file://` targets through Kandev tabs. Targets outside the active workspace are reported as unhandled so Monaco's other opener behavior can continue.
- Completion requests translate Monaco invocation, trigger-character, and incomplete-result context into the corresponding LSP enum values and forward the trigger character when present. A server item without `textEdit` receives Monaco's current-word insertion range; explicit LSP `TextEdit` and `InsertReplaceEdit` ranges remain authoritative.
- Successful Monaco file saves synchronize every matching open language-server document to the newest editor snapshot, then notify servers that requested `textDocument/didSave`. When that live snapshot still matches the persisted snapshot, the persisted text is included only for servers that advertise `includeText`. If editing advanced while persistence was in flight, the newer buffer stays dirty and synchronized and the optional stale save text is omitted so the language server cannot be rewound. Failed saves emit no save notification.
- V1 task-host support is limited to Local PC and local Docker executors. Remote Docker, SSH, and Sprites report an unsupported-executor state.
- Each active browser WebSocket owns one language-server process. The browser shares a connection for the same session and language inside one window and closes it after its idle timeout; separate browser windows may own separate processes.
- The backend caps active LSP WebSocket connections at 8 by default. `KANDEV_LSP_MAX_CONNECTIONS` overrides the cap.
- Language-server processes and npm/Go auto-install commands are owned by the existing agentctl process manager. Instance teardown cancels and drains install work, then reaps full process trees on Unix and Windows before releasing resources.
- Kandev-managed npm and release binaries live under the task host's `~/.kandev/lsp-servers`; `gopls` is installed through the task host's Go toolchain. No managed server cache lives inside a checked-out project.
- LSP JSON-RPC bodies are limited to 16 MiB across stdio and WebSocket transport; stdio headers are bounded separately. Oversized frames close the affected connection instead of allocating unbounded memory.
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

The main backend resolves or restores the task execution, checks executor support and global capacity, authenticates to that execution's agentctl instance, and proxies WebSocket frames.

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
| `4005` | Active LSP connection cap reached.                                         |
| `4006` | Language server exit or unexpected LSP proxy stream failure.               |
| `4007` | Server binary missing and the task host has no supported installer.        |
| `4008` | Language-server process failed to start.                                   |

The browser translates categorical close statuses from the close code instead of rendering transport prose. A preceding `install_failed` status payload may retain its actionable task-host diagnostic when the stream then closes with `4003`; without that payload, `4003` uses the localized installation-failure fallback. A JSON-RPC initialization rejection preserves the server-provided `error.message`.

### Browser LSP progress contract

The browser advertises standard LSP `window.workDoneProgress` support and includes a client-generated `workDoneToken` in `initialize`. This lets servers such as JetBrains Kotlin LSP report project-import phases before the initialize response.

The browser accepts both server-created progress tokens through `window/workDoneProgress/create` and `$/progress` notifications for the initialize token. Tokens can be strings or numbers and are scoped to one browser-owned session, language, and connection generation.

Supported work-done payloads are:

| Kind     | Observable behavior                                                                              |
| -------- | ------------------------------------------------------------------------------------------------ |
| `begin`  | Adds or replaces the token with its title, message, optional percentage, and local start time.   |
| `report` | Updates only the matching active token; an omitted percentage preserves its last reported value. |
| `end`    | Removes only the matching active token and records the server's optional completion message.     |

Percentages are clamped to 0–100 for presentation. Unknown tokens, malformed payloads, and late notifications from a replaced connection are ignored.

No backend or task-host payload transforms are required: both WebSocket proxy hops transport the JSON-RPC body unchanged.

## Readiness and progress state

- Connection readiness remains the existing lifecycle (`connecting`, `installing`, `starting`, `ready`, `stopping`, `unavailable`, or `error`).
- The task-host `ready` handshake means the executable has launched and the JSON-RPC bridge can begin; it does not mean the language server has completed LSP initialization.
- Initialization is locally observable from the initialize request until its response. The UI shows locale-aware elapsed time even when the server sends no progress payload.
- The 60-second long-running presentation is derived only from elapsed wall time. It does not change connection state, cancel the request, restart the server, or assert that the server is indexing.
- Work-done progress is runtime-only activity attached to a live connection. Multiple active tokens are a flat list because LSP defines no parent/child relationship.
- The oldest active work item is the primary summary; additional active items are shown as a count. Percentages from unrelated work items are never averaged.
- The most recently ended item can remain visible as “server-reported work finished” for the lifetime of that connection. It is not described as project-wide success.
- Stop, idle disconnect, crash, socket close, connection replacement, and retry clear all active and completed progress. A replacement generation starts with no inherited work state.
- Progress activity is scoped to the current editor's session and language. It is not a global task-wide language-server dashboard.

## State and persistence

- User settings persist in the existing user-settings store.
- Manual enablement persists only in browser local storage under the session and language.
- Processes, open documents, diagnostics, and semantic-token caches are runtime-only.
- Save notifications are runtime-only and follow confirmed workspace persistence; they are not emitted for closed documents, failed writes, or servers that did not request them.
- A missing server starts only when a supported file is opened and auto-start or a toolbar action requests it.
- Closing the browser connection stops its process; stopping the task reaps every owned language-server process even if a browser connection remains open.

## Failure modes

- **Unsupported executor:** the file toolbar reports that the task host is unsupported and no process starts.
- **Missing Kotlin server:** the UI tells the user to install `kotlin-lsp` on the task host; it does not offer or retry auto-install.
- **Missing auto-installable server:** the UI reports a localized missing-server status or shows install progress when auto-install is enabled. Editors settings keep each language's installation command, prerequisites, and destination visible beside its auto-install control.
- **Installer failure:** the UI preserves the detailed task-host installer error after the WebSocket's generic close frame so toolchain, network, and package output remains actionable.
- **Task-only toolchain:** binary lookup, installer execution, and installed-binary discovery use the task runtime environment instead of the agentctl host environment.
- **Windows npm launcher:** installation and later cache lookup return the executable npm shim selected through PATHEXT rather than an unlaunchable extensionless path.
- **Windows Go workspace:** when `GOBIN`, `GOPATH`, and `HOME` are unset, post-install discovery checks the task environment's `USERPROFILE\go\bin` for the executable written by Go's default Windows workspace.
- **Unsupported Rust installer:** Windows rejects Rust auto-install with `4007`, the UI directs the user to install the server manually, and agentctl continues to discover a manually installed `rust-analyzer` from the task environment.
- **Cold Monaco initialization:** TypeScript built-ins are wrapped before model-scoped LSP suppression is registered; the LSP provider registration guard does not depend on suppression state.
- **Capacity exceeded:** the UI reports that too many language servers are active; the backend rejects the request before starting or resuming its supported task host.
- **Process start failure:** agentctl logs the task-host execution error, closes with categorical `4008` and no transport prose, and the UI shows a localized start-failure status with Retry.
- **Server crash:** the connection closes with categorical `4006`, Monaco providers and markers are cleaned up, and the status shows the localized server-exited message with a Retry action. Only intentional stop or idle teardown returns to Off.
- **Initialize rejection:** when the server rejects the JSON-RPC `initialize` request, the UI preserves its `error.message` instead of stringifying the error object as `[object Object]`.
- **No progress support or reports:** initialization still shows an indeterminate state and elapsed time; after initialize succeeds, the status surface says the server has not reported background analysis progress.
- **Initialize response is slow or never arrives:** the UI confirms that the process launched, changes to a long-running initialization warning after 60 seconds, and keeps Stop available. Kandev does not automatically kill a cold project import or claim that the server is indexing.
- **Indeterminate progress:** the UI shows the server title/message and elapsed time without a percentage or ETA.
- **Malformed or stale progress:** the client ignores the payload and preserves the current connection and valid work items.
- **Cross-file intelligence remains incomplete after ready:** the UI does not claim that the server is still indexing unless a work item is active; the status surface explains that project import, dependencies, or module resolution may require investigation.
- **Task stop:** agentctl closes process admission and reaps the language-server process tree before releasing task resources.
- **Instance teardown during auto-install:** agentctl cancels the install, removes an unpublished partial release download, drains the shared cache mutation, and reaps npm/Go descendants before releasing task resources.
- **Unknown language:** no LSP control is shown.

## Scenarios

- **GIVEN** Kotlin auto-start is enabled and `kotlin-lsp` is on a Local PC task host's `PATH`, **WHEN** a `.kt` or `.kts` file opens, **THEN** the toolbar reaches ready and Monaco registers Kotlin providers.
- **GIVEN** a language server was acquired by global auto-start, **WHEN** the user explicitly stops it and a matching editor remounts or its configuration changes, **THEN** the server stays Off for that session and language until the user explicitly starts it again.
- **GIVEN** `kotlin-lsp` is missing, **WHEN** Kotlin LSP starts, **THEN** the connection closes with `4007` and the UI shows Kotlin-specific manual setup guidance without attempting installation.
- **GIVEN** an auto-installable server is missing and the task-host close reason contains backend prose, **WHEN** the browser handles `4001` in a non-English locale, **THEN** it renders the localized missing-server catalog message rather than the transport reason.
- **GIVEN** a language supports configurable auto-install, **WHEN** the user opens Editors settings, **THEN** its installation command, prerequisites, or managed destination are visible in the language card without requiring pointer hover.
- **GIVEN** a local Docker task, **WHEN** an LSP starts, **THEN** the binary is resolved and executed inside the container rather than on the main backend host.
- **GIVEN** Go and `GOBIN` are available only through the task runtime environment, **WHEN** Kandev discovers or installs `gopls`, **THEN** lookup, `go install`, and result discovery all use those task values.
- **GIVEN** a Windows Local PC task uses Go's defaults with only `USERPROFILE` set, **WHEN** `go install` publishes `gopls.exe`, **THEN** Kandev discovers it under `USERPROFILE\go\bin` and completes auto-install successfully.
- **GIVEN** the executor overrides `HOME`, **WHEN** Kandev discovers or installs an npm/release-managed language server, **THEN** cache lookup and publication use that task home rather than agentctl's parent-process home.
- **GIVEN** an npm-managed language server is installed on Windows, **WHEN** installation completes or a later connection reuses the cache, **THEN** Kandev returns and launches the concrete PATHEXT-resolved shim.
- **GIVEN** Kandev runs on Windows and Rust auto-install is enabled, **WHEN** a Linux Local Docker task needs `rust-analyzer`, **THEN** the preference reaches its Linux agentctl and installation can proceed; a Windows Local PC agentctl reports `4007` with manual-install guidance while continuing to run a manually installed binary.
- **GIVEN** agentctl reports a detailed npm, Go, or release installation failure and then closes with the generic install-failed code, **WHEN** the browser handles both frames, **THEN** the detailed installer error remains visible.
- **GIVEN** agentctl closes with `4003` and generic task-host prose but no preceding detailed installation payload, **WHEN** the browser handles that close in a non-English locale, **THEN** it renders the localized installation-failure fallback instead of the transport reason.
- **GIVEN** TypeScript LSP initializes while Monaco is still loading, **WHEN** Monaco's lazy TypeScript providers register, **THEN** the built-ins are wrapped and suppressed while only the explicitly guarded LSP providers remain active.
- **GIVEN** one session has an active TypeScript LSP and another session or model does not, **WHEN** Monaco requests TypeScript intelligence for either model, **THEN** built-ins are suppressed only for the model owned by the active connection and remain available to the unrelated model.
- **GIVEN** a server validly returns an empty semantic-token array, **WHEN** Monaco requests full document semantic tokens, **THEN** Kandev returns a completed empty token payload without scheduling periodic retries; a later server-requested semantic-token refresh remains supported.
- **GIVEN** Monaco requests completion manually, after a trigger character, or for an incomplete result, **WHEN** the provider sends `textDocument/completion`, **THEN** the server receives LSP trigger kinds `1`, `2`, or `3` respectively and the trigger character for kind `2`.
- **GIVEN** the server returns a completion list with `isIncomplete: true`, **WHEN** Monaco receives it and the user keeps typing, **THEN** Kandev preserves that incomplete marker so Monaco requests refreshed results with LSP trigger kind `3`.
- **GIVEN** the server advertises completion trigger characters and returns standard LSP completion kinds, **WHEN** Monaco registers and renders that provider, **THEN** it invokes automatic completion only for the advertised characters and uses the matching Monaco category for every standard kind.
- **GIVEN** the server omits signature-help capability, **WHEN** Monaco providers register, **THEN** Kandev does not register or call signature help; when the capability is present, Kandev preserves its advertised trigger and retrigger characters.
- **GIVEN** a completion item omits `textEdit`, **WHEN** Monaco renders or accepts it, **THEN** Kandev uses the current word at the requested position as its insertion range; an explicit server `TextEdit` or `InsertReplaceEdit` range overrides that fallback and preserves Monaco's corresponding single or dual range.
- **GIVEN** a live server was initialized with one per-language configuration, **WHEN** the user saves different LSP JSON in Editors settings, **THEN** the existing connection answers future `workspace/configuration` requests with the new value and sends `workspace/didChangeConfiguration` without spawning another server.
- **GIVEN** an open Monaco document has a debounced content change and its server requests save synchronization, **WHEN** Kandev successfully persists the current editor snapshot, **THEN** the server receives the final `textDocument/didChange` before `textDocument/didSave` for its canonical task-host URI and receives that persisted snapshot on the save notification only when it requests `includeText`; a rejected write sends no save notification.
- **GIVEN** the user types again while a file save is in flight, **WHEN** the older snapshot finishes persisting, **THEN** the newer editor snapshot remains dirty and is the language server's current document, while `textDocument/didSave` omits the stale optional text instead of rewinding the document.
- **GIVEN** an SSH, Sprites, or remote-Docker task, **WHEN** a user starts LSP, **THEN** the UI reports an unsupported executor and no language-server or task execution is started or resumed for that request.
- **GIVEN** the configured connection cap is reached, **WHEN** another editor starts LSP for a stopped supported task, **THEN** the new connection closes with `4005` before Kandev starts or resumes that task host.
- **GIVEN** a discovered language-server executable cannot be launched, **WHEN** agentctl starts it, **THEN** the task-host error stays in logs while the browser receives `4008` with no reason and shows the localized start-failure status.
- **GIVEN** a language server rejects `initialize` with a JSON-RPC error object, **WHEN** the browser handles that response, **THEN** the error state shows the server's `error.message` rather than `[object Object]`.
- **GIVEN** two task/session connections have active providers, placeholder models, or diagnostics, **WHEN** one connection stops or crashes, **THEN** cleanup removes only that connection's state and leaves the other connection fully functional.
- **GIVEN** an initialized language server exits unexpectedly, **WHEN** agentctl closes the WebSocket with `4006` and no transport prose, **THEN** its editor shows the localized server-exited error with Retry rather than presenting the server as intentionally off.
- **GIVEN** two sessions expose the same task-host file URI (for example two Docker tasks rooted at `/workspace`), **WHEN** both files are open, **THEN** Monaco keeps session-scoped models and content while both language servers receive the clean task-host URI.
- **GIVEN** a connection is replaced for the same session and language, **WHEN** callbacks from the old connection arrive late, **THEN** they cannot close, initialize, or clean up the replacement generation.
- **GIVEN** session workspace metadata hydrates after the LSP connection, **WHEN** the client opens or navigates to a document, **THEN** it uses the canonical workspace URI and repository subpaths from the task-host ready handshake, including after that LSP connection stops.
- **GIVEN** a definition or reference target is nested beneath unloaded folders, **WHEN** Monaco navigates to that file, **THEN** the Files tree loads and expands every ancestor and marks the target as active.
- **GIVEN** an attached-repository file is already open under the task-root-relative identity supplied by the Files tree, **WHEN** Monaco navigates to the same file through an LSP definition or reference, **THEN** Kandev activates and scrolls that existing editor instead of opening a second repository-scoped tab.
- **GIVEN** Monaco's built-in intelligence returns a regular `file://` target inside the active task workspace, **WHEN** its editor opener runs, **THEN** Kandev opens the matching task file; a target outside that workspace is reported as unhandled instead of being swallowed.
- **GIVEN** the task host has launched a language-server process, **WHEN** the LSP `initialize` response is still pending, **THEN** the current editor's status surface distinguishes the launched process from protocol readiness and shows increasing elapsed time with no ETA.
- **GIVEN** a non-English or pseudo locale is active, **WHEN** initialization or server work shows elapsed time, **THEN** its hour, minute, and second units and their composition come from that locale's catalog.
- **GIVEN** Kotlin LSP has not answered `initialize` for 60 seconds, **WHEN** the user opens its status, **THEN** the UI says initialization is taking longer than usual, identifies Gradle project import as a possible cause, keeps Stop available, and does not restart or time out the server automatically.
- **GIVEN** Kotlin LSP reports initialize work with a title, message, and percentage, **WHEN** `begin` and `report` notifications arrive, **THEN** the current editor shows the latest server text, the clamped percentage, and elapsed time while its connection continues initializing or remains ready.
- **GIVEN** a server reports an indeterminate work item, **WHEN** it omits percentage, **THEN** the UI shows activity and elapsed time without fabricating percentage or time remaining.
- **GIVEN** two work-done tokens are active, **WHEN** either token reports or ends, **THEN** only that token changes and the UI continues to show the oldest active item plus the remaining active count.
- **GIVEN** the final active token ends, **WHEN** the connection remains open, **THEN** the UI records that server-reported work finished without claiming all project references are complete.
- **GIVEN** a connection has active or completed work progress, **WHEN** it stops, crashes, retries, or is replaced, **THEN** the replacement connection starts without stale progress from the old generation.
- **GIVEN** initialize has completed and no work item is active, **WHEN** cross-file references are still missing, **THEN** the UI says the server has not reported ongoing analysis rather than labeling the condition as indexing.
- **GIVEN** a fine-pointer Monaco editor, **WHEN** the user opens the LSP status control, **THEN** an anchored popover presents connection readiness, project progress, and the available lifecycle action.
- **GIVEN** an LSP server reports a project-progress title or message containing a long URL, path, or identifier without ordinary break points, **WHEN** the user opens either desktop progress popover or the coarse-pointer tablet drawer, **THEN** the full text wraps within that surface without clipping, truncation, or horizontal overflow.
- **GIVEN** the saved LSP status location is `status_bar`, the application status bar is enabled, and a supported Monaco file is active on a fine-pointer layout, **WHEN** the editor renders, **THEN** the toolbar control is absent and one reorderable status-bar item shows that active file's language and live LSP summary.
- **GIVEN** the saved LSP status location is `status_bar`, **WHEN** the application status bar is disabled or the current Monaco layout uses a coarse pointer, **THEN** the toolbar control remains available and the saved `status_bar` preference is unchanged.
- **GIVEN** the active panel changes from a supported Monaco file to a non-file panel or unsupported file, **WHEN** the status bar is the preferred location, **THEN** the LSP status-bar item hides rather than showing another session or language.
- **GIVEN** a supported filename is routed to a loading, binary/static, diff, or CodeMirror surface, **WHEN** the status bar is the preferred location, **THEN** no LSP status-bar item or inert Start/Retry action is exposed until an actual Monaco text editor mounts.
- **GIVEN** a coarse-pointer tablet Monaco editor, **WHEN** the user taps the LSP status control, **THEN** an inset bottom drawer presents the same progress and lifecycle action with a touch-sized trigger and no document-level horizontal overflow.
- **GIVEN** an LSP server has spawned descendants, **WHEN** the task stops, **THEN** agentctl reaps the full process tree.
- **GIVEN** auto-install is downloading or running npm/Go, **WHEN** the agentctl instance is torn down, **THEN** the install is canceled and drained without publishing a partial binary or leaving descendants, and its `1001` WebSocket close carries no task-host prose so the browser uses localized connection-close copy.
- **GIVEN** a repository contains `.kandev/lsp-servers/kotlin-lsp`, **WHEN** Kotlin LSP starts, **THEN** Kandev ignores that project-controlled executable.
- **GIVEN** a mobile viewport, **WHEN** a supported file opens, **THEN** the mobile viewer does not start an LSP process invisibly.

## Out of scope

- Remote Docker, SSH, and Sprites executor support.
- Durable per-task/session enablement and deny lists.
- Sharing one server process across browser windows.
- Rename, code actions, document symbols, formatting, and workspace-edit application.
- CodeMirror/mobile LSP parity.
- A global dashboard across every session/language connection; the application status-bar item represents only the active Monaco file.
- Estimated time remaining, predicted completion, or any guarantee that a percentage maps linearly to project readiness.
- Inferring actual indexing state, percentage, or completion from `window/logMessage`, `window/showMessage`, process output, elapsed time, or language-specific text heuristics. Elapsed time is used only to disclose that initialization is long-running.
- Request-scoped partial-result streaming, `partialResultToken`, `$/cancelRequest`, and progress cancellation.
- Bootstrapping project dependencies such as Gradle import, `npm install`, `go mod download`, or Python virtual environments.
- Replacing external editors or embedded VS Code.

## References

- Kotlin LSP documentation: <https://kotlinlang.org/docs/kotlin-lsp.html>
- Kotlin LSP repository: <https://github.com/Kotlin/kotlin-lsp>
- Kotlin LSP slow-initialize report: <https://github.com/Kotlin/kotlin-lsp/issues/148>
- Kotlin LSP never-completing initialize report: <https://github.com/Kotlin/kotlin-lsp/issues/189>
