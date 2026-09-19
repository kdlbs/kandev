---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001
created: 2026-07-09
updated: 2026-08-11
owners:
  - tbd
---
# LSP File Intelligence System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
- **GIVEN** TypeScript LSP initializes while Monaco is still loading, **WHEN** Monaco's lazy TypeScript providers register, **THEN** built-ins are wrapped only for features Kandev can replace and explicitly guarded LSP providers remain active.
- **GIVEN** one session has an active TypeScript LSP and another session or model does not, **WHEN** Monaco requests TypeScript intelligence for either model, **THEN** built-ins are suppressed only for an advertised external provider on the model owned by that connection and remain available to unrelated models and unadvertised or unwired features.
- **GIVEN** a server omits completion, hover, definition, references, or signature-help capability, **WHEN** Kandev registers Monaco providers, **THEN** it sends no requests for that optional method and leaves the corresponding TypeScript/JavaScript built-in available; advertised providers register and suppress only their matching built-in feature.
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
- **GIVEN** two task sessions have the same repository file open, **WHEN** the user selects a content-search hit scoped to one active session, **THEN** both the pending cursor and immediate mounted-editor reveal target only that session's model.
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
- **GIVEN** the saved LSP status location is `status_bar`, **Show status bar** is on, and a supported Monaco file is active on a fine-pointer layout, **WHEN** the editor renders, **THEN** the toolbar control is absent and one reorderable status-bar item shows that active file's language and live LSP summary.
- **GIVEN** the saved LSP status location is `status_bar`, **WHEN** **Show status bar** is off or the current Monaco layout uses a coarse pointer, **THEN** the toolbar control remains available and the saved `status_bar` preference is unchanged.
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
