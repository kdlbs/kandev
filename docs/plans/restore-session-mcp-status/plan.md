---
created: 2026-09-09
status: done
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
system_design:
  - ../../specs/platform/system-design/mcp-session-observability.md
legacy_specs: []
---

# Implementation Plan: Restore Session MCP Status

## Overview

Issue #3547 leaves persisted MCP status unavailable after some task switches.
The repair adds validated, freshness-aware restoration to session-list and
forced task-detail route reconciliation. One frontend work order owns the code
and regression tests.

## Confirmed root cause

`setTaskSessionsForTask` merges the returned session rows, but it does not read
`metadata.mcp_attachment_state`. Boot hydration and live WebSocket events can
populate `sessionMcpStatus.bySessionId`, but forced route hydration could also
replace newer live MCP evidence with an older response.

The new `set-task-sessions-mcp.test.ts` file will contain the permanent
regression test. Its restoration case fails on the current code because the
status entry remains undefined.

## Scope

### In scope

- Restore valid version-1 MCP attachment history from task session metadata.
- Reject malformed and unsupported histories.
- Keep newer live evidence when a delayed session-list snapshot arrives.
- Keep newer live evidence when a delayed forced route snapshot arrives.
- Keep every sibling session status unchanged.

### Out of scope

- Backend persistence or response changes.
- Changes to WebSocket event production.
- Changes to the MCP status layout or user-facing copy.
- Changes to MCP evidence semantics or schema versioning.

## Technical approach

### Runtime validation and freshness

Add shared validation and freshness helpers in the session-runtime slice. Reuse
`parseStrictRfc3339Timestamp` for all timestamps in the frontend projection.
Accept additive fields and JSON `null` for optional values, but reject invalid
required fields and unknown status values.

During `setTaskSessionsForTask`, inspect each merged session's
`metadata.mcp_attachment_state`. Compare the current attempt's `updated_at`,
or its `started_at` fallback, with the stored history. Apply only a strictly
newer valid snapshot, unless no valid stored history exists.

Update only `sessionMcpStatus.bySessionId[session.id]` in the same Immer
transaction. Metadata absence or rejection leaves the existing entry and all
sibling entries unchanged. Use the same comparator during forced task-detail
route hydration so its response cannot regress a newer live entry.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.10` | `set-task-sessions-mcp.test.ts` proves restoration with and without `updated_at`. |
| `AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.11` | Session-list and route-hydration tests prove that older and equal snapshots do not replace live evidence. |
| `AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.12` | Table tests cover malformed shapes, timestamps, names, statuses, and versions. A sibling-isolation test protects unrelated state. |

## E2E tests

No new Playwright test is required. This repair only normalizes data in an
existing store and does not change a rendered interaction. Existing desktop
and mobile MCP status tests prove that both surfaces consume this shared state.

Relevant existing files:

- `apps/web/e2e/tests/chat/mcp-status.spec.ts`
- `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts`

## Work orders

- [x] [Task 01: Restore MCP status from session lists](task-01-restore-session-list-mcp-status.md)

## Verification results

- `cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/session/set-task-sessions-mcp.test.ts lib/state/hydration/hydrator.test.ts` passed (46 tests).
- `cd apps/web && pnpm run typecheck` passed.
- Targeted ESLint passed for the session slice, reconciliation helper, hydrator, and both regression tests.
- Related session reconciliation tests passed (3 files, 57 tests).

## Risks

- Excessive validation can reject additive safe fields. The guard validates
  known fields and permits unknown optional fields.
- Timestamp ties can hide live details. Equal timestamps retain the current
  store entry.
