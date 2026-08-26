---
created: 2026-08-26
status: complete
requirements:
  - REQ-UI-KANDEV-MCP-TOOL-RESULTS-001
system_design:
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
legacy_specs: []
---

# Implementation Plan: Kandev MCP Tool Result Fallback

## Overview

This repair preserves JSON text results when an MCP response contains null
structured content. One work order adds the regression first, corrects the
shared parser, and keeps the existing result precedence.

## Scope

### In scope

- Reproduce the persisted Codex ACP result envelope in focused tests.
- Treat null structured content as absent.
- Preserve non-null structured content and historic wrapper support.
- Keep desktop and mobile result values equal through the shared parser.

### Out of scope

- Backend MCP or ACP changes.
- New renderer types or presentation changes.
- New mobile composition or Playwright scenarios.

## Technical approach

Add an exact null-envelope case to `extractMcpResult` coverage. Update the
renderer fixture so it uses the persisted `CallToolResult` shape.

Change `extractMcpResult` to accept only non-null structured values. If a
structured value is null, continue to the existing `content` parser.

## Tests

- `AC-UI-KANDEV-MCP-TOOL-RESULTS-001.1`: Keep the existing structured-content
  precedence test in `kandev/parse.test.ts`.
- `AC-UI-KANDEV-MCP-TOOL-RESULTS-001.2`: Add a test with
  `structuredContent: null` and JSON text content.
- `AC-UI-KANDEV-MCP-TOOL-RESULTS-001.3`: Use the shared renderer fixture to
  show the returned count from the exact persisted envelope.

## E2E tests

The existing `mcp-status.spec.ts` and `mobile-mcp-status.spec.ts` cover the
shared transcript entry point on desktop and mobile. No new Playwright test is
required because this repair changes only data normalization.

The mock agent flattens MCP results before ACP delivery. Therefore, a focused
parser fixture is the faithful test for the Codex null envelope.

## Work orders

- [x] [Task 01: Preserve text fallback](task-01-preserve-text-fallback.md) (done)

## Verification results

Task 01 completed. The focused parser and renderer suite passed 51 tests, and
frontend typecheck passed.

## Risks

- A broad falsy-value check can change valid structured-result precedence.
- An incomplete fixture can miss the real Codex `CallToolResult` shape again.
