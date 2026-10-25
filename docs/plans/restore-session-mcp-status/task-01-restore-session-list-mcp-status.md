---
id: "01-restore-session-list-mcp-status"
title: "Restore MCP status from session lists"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
acceptance_criteria:
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.10
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.11
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.12
system_design:
  - ../../specs/platform/system-design/mcp-session-observability.md
---

# Task 01: Restore MCP Status From Session Lists

## Summary

Restore persisted MCP attachment history during task session-list and forced
task-detail route reconciliation. Reject invalid metadata and prevent delayed
snapshots from replacing newer live evidence.

## In scope

- Validate the version-1 frontend projection in
  `metadata.mcp_attachment_state`.
- Compare incoming and stored current-attempt timestamps with the strict
  RFC3339 parser.
- Reuse the validation and freshness comparator during forced task-detail
  route hydration.
- Update only the MCP status entry for the owning session.
- Add focused regression tests for restoration, rejection, freshness, route
  ordering, JSON null optionals, and sibling isolation.

## Out of scope

- Backend, persistence, or wire-contract changes.
- WebSocket handler changes.
- Component, layout, localization, or mobile interaction changes.

## Acceptance

- A valid persisted history appears in the owning session's MCP status entry.
- Equal or older snapshots never replace newer live evidence.
- Invalid metadata does not change the owning entry or a sibling entry.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/session/set-task-sessions-mcp.test.ts lib/state/hydration/hydrator.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web exec eslint lib/state/slices/session/session-slice.ts lib/state/slices/session-runtime/mcp-attachment-reconciliation.ts lib/state/hydration/hydrator.ts lib/state/slices/session/set-task-sessions-mcp.test.ts lib/state/hydration/hydrator.test.ts
```

## Files likely touched

- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session-runtime/mcp-attachment-reconciliation.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/slices/session/set-task-sessions-mcp.test.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`

## Dependencies

None.

## Risks

- The metadata type is `unknown` at runtime. A partial guard can admit invalid
  nested values.
- Backend timestamps can use RFC3339 nanosecond precision. Millisecond parsing
  can misorder live and persisted evidence.

## Parallelism

`sequential`

## Inputs

- `REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001` and its new acceptance criteria.
- The session MCP observability system design and ADR.
- Existing strict timestamp parsing and session-list reconciliation patterns.

## Results

Implemented shared validated hydration of `metadata.mcp_attachment_state` during
session-list reconciliation and freshness-guarded forced route hydration.
Valid version-1 histories restore the owning session, additive fields and JSON
null optionals remain supported, malformed histories are ignored, and older or
equal snapshots cannot replace newer live evidence.

Verification passed:

- `cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/session/set-task-sessions-mcp.test.ts lib/state/hydration/hydrator.test.ts` (46 tests)
- `cd apps/web && pnpm run typecheck`
- Targeted ESLint for the session slice, reconciliation helper, hydrator, and both regression tests.
- Related session reconciliation tests (3 files, 57 tests)
