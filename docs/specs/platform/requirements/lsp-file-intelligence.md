---
status: active
system: platform
created: 2026-07-09
updated: 2026-08-11
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

## System design

The migrated technical source is split into [part 1](../system-design/lsp-file-intelligence-01.md), [part 2](../system-design/lsp-file-intelligence-02.md).
