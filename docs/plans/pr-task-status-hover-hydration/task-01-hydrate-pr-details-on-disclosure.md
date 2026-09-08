---
id: "01-hydrate-pr-details-on-disclosure"
title: "Hydrate PR details on disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
acceptance_criteria:
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.1
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.13
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.14
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.15
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.16
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
---

# Task 01: Hydrate PR Details on Disclosure

## Summary

Give compact GitHub PR indicators the same tooltip trigger as full indicators.
Load stored full PR records for only the disclosed task and keep the result in
the existing store.

## In scope

- Add a task-scoped hydration hook with store-scoped request deduplication.
- Guard workspace and workspace-context changes, preserve matching WebSocket
  store records, and honor deletion tombstones.
- Treat `TaskPR.workspace_id` as authoritative for live updates. Ignore missing
  or foreign browser payloads and route typed backend events through the
  fail-closed workspace broadcaster when authentication is enforced.
- Render compact loading and unavailable tooltip content.
- Start the load on mouse pointer entry or visible keyboard focus.
- Add localized loading and unavailable copy in all required locales.

## Out of scope

- Author presentation.
- Upstream GitHub refreshes.
- Workspace-wide PR hydration.
- Backend, persistence, or task-summary changes.

## Acceptance

- A compact indicator opens immediately and becomes a full summary after one
  task-scoped request.
- Concurrent disclosure requests share one request, and loaded store data
  prevents another request.
- Empty, failed, stale-workspace, and WebSocket-before-HTTP paths preserve safe
  state and permit a later retry.
- A missing or mismatched workspace ID in a live TaskPR update leaves the active
  scoped cache unchanged, and an unattributed typed backend event cannot fan out
  across workspace owners when authentication is enforced.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx components/github/pr-task-icon.render.test.tsx components/task/task-item.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web run i18n:check)
(cd apps && pnpm --filter @kandev/web run i18n:ratchet)
```

## Files likely touched

- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.ts`
- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx`
- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon.render.test.tsx`
- `apps/web/components/task/task-contribution-icons.tsx`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/src/locales/*/github.json`

## Dependencies

None.

## Risks

- HTTP hydration and WebSocket updates can arrive in either order.
- Local component state alone cannot deduplicate several mounted task surfaces.
- Pointer leave must not cancel useful store hydration.

## Parallelism

`sequential`

## Inputs

- Requirement criteria `.1` and `.13` to `.16`.
- The disclosure data flow and failure sections in the system design.
- `RegisteredChangeRequestTaskIcon` as the on-open refresh pattern.

## Results

- Added task-scoped compact PR hydration with store-scoped request
  deduplication, settled-cache reuse, workspace race protection, WebSocket
  precedence, and retryable empty or failed states.
- Added localized loading and unavailable tooltip states in all required
  locales.
- Added pointer and keyboard disclosure coverage plus focused hydration tests.
- Added task-id and workspace-context generation race coverage, plus protection
  against resurrecting an association deleted while an HTTP request is pending.
- Focused unit tests, typecheck, i18n checks, ratchet, full web lint, desktop
  E2E, mobile E2E, and the E2E build passed.
- The 2026-08-26 review fixup added frontend missing/foreign workspace event
  coverage, backend typed-event routing and fail-closed coverage, and preserved
  taskPR scope metadata in the singleton task-switch test. The focused frontend
  suite passed 95 tests across 6 files and the backend gateway/GitHub suites
  passed 2,176 tests.
