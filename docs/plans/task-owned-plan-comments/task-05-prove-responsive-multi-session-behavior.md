---
id: "05-prove-responsive-multi-session-behavior"
title: "Prove responsive multi-session behavior"
status: completed
wave: 5
depends_on:
  - "03-project-comments-across-task-sessions"
  - "04-migrate-legacy-browser-drafts"
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
  - REQ-TASKS-PLAN-COMMENTS-004
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-001.1
  - AC-TASKS-PLAN-COMMENTS-001.2
  - AC-TASKS-PLAN-COMMENTS-001.3
  - AC-TASKS-PLAN-COMMENTS-001.7
  - AC-TASKS-PLAN-COMMENTS-002.1
  - AC-TASKS-PLAN-COMMENTS-002.2
  - AC-TASKS-PLAN-COMMENTS-002.5
  - AC-TASKS-PLAN-COMMENTS-002.6
  - AC-TASKS-PLAN-COMMENTS-003.1
  - AC-TASKS-PLAN-COMMENTS-003.2
  - AC-TASKS-PLAN-COMMENTS-003.3
  - AC-TASKS-PLAN-COMMENTS-003.6
  - AC-TASKS-PLAN-COMMENTS-004.1
  - AC-TASKS-PLAN-COMMENTS-004.3
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 05: Prove responsive multi-session behavior

## Summary

Add production-build browser evidence for the complete two-session contract.
Prove the visible shared state, selected-versus-primary routing, accepted
consumption, migration, and equivalent phone behavior through user surfaces.

## In scope

- A focused desktop multi-session Plan-comment Playwright spec.
- A `mobile-chrome` Plan Drawer and session-picker spec at Pixel 5 dimensions.
- Backend transcript/queue assertions that distinguish primary and secondary
  destinations rather than relying only on visible active-chat text.
- Reload, migration, touch-target, and horizontal-overflow assertions.

## Out of scope

- Timing sleeps, mocked component state, or a browser-only assertion of routing.
- Repeating repository conflict matrices already owned by Tasks 01 and 02.

## Acceptance

- Desktop proves one shared pending annotation/context across tabs and reload,
  Send from secondary only reaches secondary, and Run from secondary only
  reaches primary.
- A successfully delivered comment disappears from both Plan and composer
  projections, while a rejected path preserves it and the composer draft.
- Mobile proves the same destination rules through native phone navigation,
  with 44 px actions, contained Drawer scrolling, and no horizontal overflow.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/session/task-plan-comments.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-task-plan-comments.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/session/task-plan-comments.spec.ts`
- `apps/web/e2e/tests/session/mobile-task-plan-comments.spec.ts`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/helpers/api-client.ts`

## Dependencies

Tasks 03 and 04.

## Risks

- The selected session and primary session must have distinct IDs and both be
  input-capable; otherwise the test can pass without proving routing.
- Queue assertions must account for server-owned auto-run without converting
  the scenario into a timing race.
- Mobile tests must use the session picker and Drawer rather than desktop-only
  tab or Popover selectors.

## Parallelism

`sequential`

## Inputs

- All task plan comment requirements and the system-design verification map.
- Existing `multi-session-ux.spec.ts`, `comment-run-not-queued.spec.ts`, mobile
  session picker tests, and API message/queue helpers.

## Results

- Added focused desktop and Pixel 5 Playwright scenarios using real backend
  transcripts to distinguish selected-session Send from primary-session Run.
- Desktop proves shared comments across tabs and reload plus task-wide
  consumption after accepted delivery.
- Mobile proves two-session legacy migration, retained diff context, shared
  composer state, distinct routing, 44 px actions, and no horizontal overflow.
- Both scenarios pass against the final production build.
