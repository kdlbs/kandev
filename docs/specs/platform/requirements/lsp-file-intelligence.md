---
status: active
system: platform
created: 2026-07-09
updated: 2026-09-02
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

- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.1:** Kandev supports TypeScript/JavaScript through `typescript-language-server`, Python through `pyright-langserver`, Go through `gopls`, Rust through `rust-analyzer`, and experimental Kotlin through the official `kotlin-lsp`.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.2:** Monaco wires diagnostics and only the completion, hover, go-to-definition, references, signature-help, and semantic-token providers advertised by the initialized server.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.3:** Exactly one logical lifecycle exists per task and language. Sessions, editors, browser surfaces, file selection, panel selection, reconnects, and editor unmounts never become process owners or create duplicate servers.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.4:** Each task language exposes Inherit default, Keep warm, and Disabled policy plus Start, Stop, and Restart. Explicit Stop prevents automatic reacquisition; Restart reaps the old generation before creating exactly one replacement.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.5:** Supported project languages are detected with bounded filename/extension scanning before a matching file opens, without starting, installing, or importing a language server.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.6:** A task-level aggregate remains independent of the active file and exposes per-language state, policy, progress, lifecycle evidence, actionable errors, and controls. Users may hide languages from aggregate surfaces without changing their task policy or runtime.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.7:** The task control remains discoverable when the application status bar is disabled or no supported file is active. Phone and tablet use the existing task Status composition with 44 px controls, one contained scroll owner, safe-area handling, and no nested drawer.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.8:** Lifecycle status distinguishes admission, installation, process start, initialize, server-reported work, ready/idle, stopping, unsupported, unavailable, and error. Kandev never invents indexing percentages, completion, or ETAs.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.9:** Global editor settings remain defaults for auto-start, auto-install permission, status visibility, and per-language `workspace/configuration`; saving configuration updates the one live task server through `workspace/didChangeConfiguration` when possible.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.10:** Local PC/Worktree and Local Docker task environments are supported. Unsupported, missing, or unknown runtimes fail before capacity admission or task-host launch/resume.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.11:** Capacity counts actual starting, live, or stopping task/language servers, never transient browser/editor leases. Desired overflow queues without launching resources and starts only after a real slot is released.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.12:** Backend restart and browser reconnect recover a surviving task-host generation without another initialize/import. Ambiguous runtime evidence prevents duplicate launch until absence is proven.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.13:** Task stop, archive, delete, and environment teardown cancel pending lifecycle work, reap the full LSP process tree, clear runtime progress, and clean encrypted runtime credentials while preserving policy only for resumable task transitions.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.14:** Browser task access is authorized before runtime lookup. Browser document URIs are resolved only through pinned task-root handles, and project-controlled language-server binaries are never executed.
- **AC-PLATFORM-LSP-FILE-INTELLIGENCE-001.15:** A future task-scoped MCP action must call the same authorized controller with trusted origin metadata; this requirement does not add an MCP tool.

## System design

The migrated technical source is split into [part 1](../system-design/lsp-file-intelligence-01.md), [part 2](../system-design/lsp-file-intelligence-02.md).
