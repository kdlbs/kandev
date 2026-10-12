---
status: active
system: platform
created: 2026-07-09
updated: 2026-09-24
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

- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.1:** When a desktop browser tab closes or loses its connection, an enabled language server shall continue running and analyzing that task workspace while its lease is retained. The lease shall keep the task host active across a completed agent turn and idle-reclaim interval, unless the user stops the task.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.2:** When a desktop editor for the same task and language opens again while its lease is retained, it shall attach to the same server, restore providers and reported project progress without a manual Retry or second server start, synchronize its current file contents, and show diagnostics only after the server publishes them for the reopened document.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.3:** When a connected browser briefly loses the LSP transport, the editor shall show a connection state, attempt to reattach, and reserve the server-exited error for an actual server exit. Diagnostics from a previous attachment shall remain hidden until the server analyzes the reopened document.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.4:** An explicit Stop shall release that editor's server. Other concurrently open browser windows for the same task and language shall keep their independent server and editor state. Stopping the task host shall stop all of its retained servers and descendants.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.5:** Retained servers shall count against the configured LSP resource limit. Reattaching to a matching retained lease shall remain possible at the limit. Admission of a new server shall release the least recently detached lease when necessary; if every lease is attached, it shall show the existing capacity state. An evicted lease shall start fresh on a later eligible editor open.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.6:** If the task host or Kandev restarts, the editor shall start a new server when its existing auto-start or manual-enable policy requests one and shall show fresh initialization until that server is ready. It shall not present a lost server's progress or diagnostics as live.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.7:** On phone, opening the file viewer shall not start or attach to a language server in the background. A coarse-pointer tablet editor shall regain the same status and lifecycle action through its existing toolbar drawer.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.8:** When the last editor in a connected browser remains closed for the existing two-minute idle timeout, that browser shall release its lease and free its capacity. Closing the browser or losing the network before the timeout shall retain the lease until its detached deadline. A saved per-language configuration change shall reach a detached server without requiring a restart.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.9:** A returning tab shall attach to its retained lease when the user has not explicitly stopped it, even if auto-start was subsequently disabled. A new tab without a lease hint shall follow current auto-start or browser-local manual-enable policy. A duplicated tab shall not take over the original tab's attached lease.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.10:** A lease shall expire one hour after its most recent browser detachment if no browser reattaches. Expiry shall stop its language server, release its capacity, and allow normal task-host idle reclaim. An attached lease shall not expire. A later eligible editor open shall start a fresh server and project analysis.

## Out of scope

- Keeping a server running after its task host stops.
- Sharing one live protocol session between concurrently open browser windows.
- Adding LSP controls to the phone file viewer or extending LSP to unsupported executors.

## System design

The migrated technical source is split into [part 1](../system-design/lsp-file-intelligence-01.md), [part 2](../system-design/lsp-file-intelligence-02.md).
