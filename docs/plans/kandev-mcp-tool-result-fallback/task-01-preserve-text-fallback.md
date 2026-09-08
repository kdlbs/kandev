---
id: "01-preserve-text-fallback"
title: "Preserve text fallback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-KANDEV-MCP-TOOL-RESULTS-001
acceptance_criteria:
  - AC-UI-KANDEV-MCP-TOOL-RESULTS-001.1
  - AC-UI-KANDEV-MCP-TOOL-RESULTS-001.2
  - AC-UI-KANDEV-MCP-TOOL-RESULTS-001.3
system_design:
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
---

# Task 01: Preserve Text Fallback

## Summary

Add a regression for the persisted Codex MCP envelope. Then correct the shared
parser so a null structured value does not hide JSON text content.

## In scope

- Add the failing null-envelope parser test before the production change.
- Update the renderer fixture to match the persisted `CallToolResult` shape.
- Make the minimum null-handling correction in `extractMcpResult`.
- Preserve existing structured-content and historic-wrapper behavior.

## Out of scope

- Backend MCP, ACP adapter, or database changes.
- Renderer layout or copy changes.
- New Playwright tests.

## Acceptance

- The regression fails because the parser returns null before the correction.
- The corrected parser returns the JSON text payload when `structuredContent`
  is null.
- All focused parser and native Kandev renderer tests pass after the change.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/kandev/parse.test.ts components/task/chat/messages/kandev-tool-message.test.tsx)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/parse.ts`
- `apps/web/components/task/chat/messages/kandev/parse.test.ts`
- `apps/web/components/task/chat/messages/kandev-tool-message.test.tsx`
- `docs/plans/kandev-mcp-tool-result-fallback/plan.md`
- `docs/plans/kandev-mcp-tool-result-fallback/task-01-preserve-text-fallback.md`

## Dependencies

None.

## Risks

- Preserve empty but valid structured objects. Treat only null and undefined
  as absent.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-KANDEV-MCP-TOOL-RESULTS-001` and its system design.
- The persisted task-session message and ACP envelope from the diagnosis.
- Existing parser and renderer tests.

## Results

RED: The real Codex envelope test failed because `extractMcpResult` returned
null when `structuredContent` was null.

GREEN: The parser now falls through to JSON text content only for null or
undefined structured values. The focused suite passed 51 tests, and frontend
typecheck passed.
