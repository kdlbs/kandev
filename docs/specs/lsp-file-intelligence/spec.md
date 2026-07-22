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
- Global editor settings select languages that auto-start, languages Kandev may auto-install, and per-language configuration returned through `workspace/configuration`.
- A user can manually start or stop the current file's server from the file toolbar. Manual state is remembered in browser local storage for that session and language.
- Kotlin supports auto-start but not auto-install. `kotlin-lsp` must already be available on the task host's `PATH`.
- Language servers run through the task's `agentctl`, with the task workspace as their working directory. This keeps project files, dependencies, and server execution in the same environment.
- V1 task-host support is limited to Local PC and local Docker executors. Remote Docker, SSH, and Sprites report an unsupported-executor state.
- Each active browser WebSocket owns one language-server process. The browser shares a connection for the same session and language inside one window and closes it after its idle timeout; separate browser windows may own separate processes.
- The backend caps active LSP WebSocket connections at 8 by default. `KANDEV_LSP_MAX_CONNECTIONS` overrides the cap.
- Language-server processes and npm/Go auto-install commands are owned by the existing agentctl process manager. Instance teardown cancels and drains install work, then reaps full process trees on Unix and Windows before releasing resources.
- Kandev-managed npm and release binaries live under the task host's `~/.kandev/lsp-servers`; `gopls` is installed through the task host's Go toolchain. No managed server cache lives inside a checked-out project.
- LSP JSON-RPC bodies are limited to 16 MiB across stdio and WebSocket transport; stdio headers are bounded separately. Oversized frames close the affected connection instead of allocating unbounded memory.
- Mobile file viewing does not start language servers in the background.

## User settings

Existing user-setting fields are the durable global policy:

| JSON field                   | Type       | Meaning                                                                                                                       |
| ---------------------------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `lsp_auto_start_languages`   | `string[]` | Languages that connect when a matching file opens.                                                                            |
| `lsp_auto_install_languages` | `string[]` | Languages Kandev may install when their server binary is missing. Kotlin is rejected because it requires manual installation. |
| `lsp_server_configs`         | `object`   | Per-language JSON returned to the server through `workspace/configuration`.                                                   |

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

| Code   | Meaning                                                            |
| ------ | ------------------------------------------------------------------ |
| `4001` | Server binary missing and auto-install is unavailable or disabled. |
| `4002` | Session, execution, or agentctl stream unavailable.                |
| `4003` | Auto-install failed.                                               |
| `4004` | Executor unsupported in V1.                                        |
| `4005` | Active LSP connection cap reached.                                 |

## State and persistence

- User settings persist in the existing user-settings store.
- Manual enablement persists only in browser local storage under the session and language.
- Processes, open documents, diagnostics, and semantic-token caches are runtime-only.
- A missing server starts only when a supported file is opened and auto-start or a toolbar action requests it.
- Closing the browser connection stops its process; stopping the task reaps every owned language-server process even if a browser connection remains open.

## Failure modes

- **Unsupported executor:** the file toolbar reports that the task host is unsupported and no process starts.
- **Missing Kotlin server:** the UI tells the user to install `kotlin-lsp` on the task host; it does not offer or retry auto-install.
- **Missing auto-installable server:** the UI reports the missing binary or shows install progress when auto-install is enabled.
- **Capacity exceeded:** the UI reports that too many language servers are active.
- **Server crash:** the connection closes, Monaco providers and markers are cleaned up, and the user can retry.
- **Task stop:** agentctl closes process admission and reaps the language-server process tree before releasing task resources.
- **Instance teardown during auto-install:** agentctl cancels the install, removes an unpublished partial release download, drains the shared cache mutation, and reaps npm/Go descendants before releasing task resources.
- **Unknown language:** no LSP control is shown.

## Scenarios

- **GIVEN** Kotlin auto-start is enabled and `kotlin-lsp` is on a Local PC task host's `PATH`, **WHEN** a `.kt` or `.kts` file opens, **THEN** the toolbar reaches ready and Monaco registers Kotlin providers.
- **GIVEN** `kotlin-lsp` is missing, **WHEN** Kotlin LSP starts, **THEN** the connection closes with `4001` and the UI shows manual setup guidance without attempting installation.
- **GIVEN** a local Docker task, **WHEN** an LSP starts, **THEN** the binary is resolved and executed inside the container rather than on the main backend host.
- **GIVEN** an SSH, Sprites, or remote-Docker task, **WHEN** a user starts LSP, **THEN** the UI reports an unsupported executor and no process starts.
- **GIVEN** the configured connection cap is reached, **WHEN** another editor starts LSP, **THEN** the new connection closes with `4005`.
- **GIVEN** two task/session connections have active providers, placeholder models, or diagnostics, **WHEN** one connection stops or crashes, **THEN** cleanup removes only that connection's state and leaves the other connection fully functional.
- **GIVEN** two sessions expose the same task-host file URI (for example two Docker tasks rooted at `/workspace`), **WHEN** both files are open, **THEN** Monaco keeps session-scoped models and content while both language servers receive the clean task-host URI.
- **GIVEN** a connection is replaced for the same session and language, **WHEN** callbacks from the old connection arrive late, **THEN** they cannot close, initialize, or clean up the replacement generation.
- **GIVEN** session workspace metadata hydrates after the LSP connection, **WHEN** the client opens or navigates to a document, **THEN** it uses the canonical workspace URI and repository subpaths from the task-host ready handshake, including after that LSP connection stops.
- **GIVEN** a definition or reference target is nested beneath unloaded folders, **WHEN** Monaco navigates to that file, **THEN** the Files tree loads and expands every ancestor and marks the target as active.
- **GIVEN** an LSP server has spawned descendants, **WHEN** the task stops, **THEN** agentctl reaps the full process tree.
- **GIVEN** auto-install is downloading or running npm/Go, **WHEN** the agentctl instance is torn down, **THEN** the install is canceled and drained without publishing a partial binary or leaving descendants.
- **GIVEN** a repository contains `.kandev/lsp-servers/kotlin-lsp`, **WHEN** Kotlin LSP starts, **THEN** Kandev ignores that project-controlled executable.
- **GIVEN** a mobile viewport, **WHEN** a supported file opens, **THEN** the mobile viewer does not start an LSP process invisibly.

## Out of scope

- Remote Docker, SSH, and Sprites executor support.
- Durable per-task/session enablement and deny lists.
- Sharing one server process across browser windows.
- Rename, code actions, document symbols, formatting, and workspace-edit application.
- CodeMirror/mobile LSP parity.
- Bootstrapping project dependencies such as Gradle import, `npm install`, `go mod download`, or Python virtual environments.
- Replacing external editors or embedded VS Code.

## References

- Kotlin LSP documentation: <https://kotlinlang.org/docs/kotlin-lsp.html>
- Kotlin LSP repository: <https://github.com/Kotlin/kotlin-lsp>
