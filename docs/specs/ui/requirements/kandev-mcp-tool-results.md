---
status: active
system: ui
created: 2026-08-26
owners:
  - kandev
---

# Kandev MCP Tool Results Requirements

## Overview

Kandev shows native transcript rows for its built-in MCP tools. A completed
result can contain structured content, text content, or both forms.

The UI system owns this reusable presentation contract. The agent and platform
systems continue to own tool execution, ACP normalization, and persistence.

## Terminology

- **Structured result:** A non-null object in `structuredContent` or
  `structured_content`.
- **Text fallback:** JSON text in the standard MCP `content` array.

## Requirements

### REQ-UI-KANDEV-MCP-TOOL-RESULTS-001: Reliable result display

**Intent:** Users need each completed Kandev MCP row to show the result that
the tool returned.

#### Acceptance criteria

- **AC-UI-KANDEV-MCP-TOOL-RESULTS-001.1:** When a completed result contains
  non-null structured content, the transcript shall use that content.
- **AC-UI-KANDEV-MCP-TOOL-RESULTS-001.2:** When structured content is null or
  absent, the transcript shall parse an available JSON text fallback.
- **AC-UI-KANDEV-MCP-TOOL-RESULTS-001.3:** Desktop and mobile transcripts shall
  show the same result values from the shared parser.

## Out of scope

- Changes to MCP handlers, ACP normalization, or message persistence.
- Portable rendering for non-Kandev MCP content.
- Changes to transcript layout, navigation, scrolling, or touch behavior.
