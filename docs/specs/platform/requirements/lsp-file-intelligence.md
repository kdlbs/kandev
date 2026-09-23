---
status: active
system: platform
created: 2026-07-09
updated: 2026-09-23
owners:
  - tbd
---
# LSP File Intelligence Requirements

## Overview

Users inspect and edit code inside Kandev task file tabs, but code navigation and analysis otherwise require opening an external editor. Lightweight language-server intelligence lets users understand a project without leaving the task.

## Requirements

### REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001: LSP File Intelligence

**Intent:** Users inspect and edit code inside Kandev task file tabs, but code navigation and analysis otherwise require opening an external editor. Lightweight language-server intelligence lets users understand a project without leaving the task.

#### Acceptance criteria

- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.1:** Desktop Monaco file editors can connect to Language Server Protocol servers for:
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.2:** TypeScript and JavaScript via `typescript-language-server`
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.3:** Python via `pyright-langserver`
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.4:** Go via `gopls`
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.5:** Rust via `rust-analyzer`
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.6:** Kotlin via the official `kotlin-lsp`; Kotlin is marked experimental while its upstream server is alpha
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.7:** Wired editor capabilities are diagnostics and the server-advertised completion, hover, go-to-definition, references, signature-help, and semantic-token providers.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.8:** Global editor settings select languages that auto-start, languages Kandev may auto-install, and per-language configuration returned through `workspace/configuration`. Saving changed configuration updates the existing server through `workspace/didChangeConfiguration` without waiting for an idle disconnect or process restart.

### REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002: Browser-independent language-server continuity

**Intent:** Closing a browser must not discard a task's live language analysis. A returning editor shall regain useful LSP information without the user restarting the server.

#### Acceptance criteria

- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.1:** When a desktop browser tab closes or loses its connection while its task host remains active, an enabled language server shall continue running and analyzing that task workspace.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.2:** When a desktop editor for the same task and language opens again, it shall attach to the retained server, synchronize its current file contents, and restore available diagnostics, providers, and reported project progress without a manual Retry or a second server start.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.3:** When a connected browser briefly loses the LSP transport, the editor shall show a connection state, attempt to reattach, and reserve the server-exited error for an actual server exit. It shall not show stale diagnostics as current when its document snapshot differs from the server's snapshot.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.4:** An explicit Stop shall release that editor's server. Other concurrently open browser windows for the same task and language shall keep their independent server and editor state. Stopping the task host shall stop all of its retained servers and descendants.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.5:** Retained servers shall count against the configured LSP resource limit. When the limit is reached, reopening an editor with a matching retained server shall remain possible; starting an additional server shall show the existing capacity state.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.6:** If the task host or Kandev restarts, the editor shall start a new server when its existing auto-start or manual-enable policy requests one and shall show fresh initialization until that server is ready. It shall not present a lost server's progress or diagnostics as live.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.7:** On phone, opening the file viewer shall not start or attach to a language server in the background. A coarse-pointer tablet editor shall regain the same status and lifecycle action through its existing toolbar drawer.

## Out of scope

- Keeping a server running after its task host stops.
- Sharing one live protocol session between concurrently open browser windows.
- Adding LSP controls to the phone file viewer or extending LSP to unsupported executors.

## System design

The migrated technical source is split into [part 1](../system-design/lsp-file-intelligence-01.md), [part 2](../system-design/lsp-file-intelligence-02.md).
