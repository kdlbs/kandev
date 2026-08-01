---
id: "01-correct-reopen-input-indicator"
title: "Correct the reopen-menu input indicator"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/background-work-liveness.md"
---

# Task 01: Correct the Reopen-Menu Input Indicator

## Acceptance

- A `WAITING_FOR_INPUT` session with no pending clarification, permission, or
  background activity renders no state icon in the task add-panel menu.
- An input-capable session with a pending clarification or permission still
  renders the corresponding question or shield-question icon, and background
  plus terminal lifecycle icons remain unchanged.
- A focused browser regression proves the exact idle-waiting backend
  precondition and the absence of both misleading question glyphs in the
  session row.

## TDD Sequence

1. Update the pure-helper assertion and add the focused Playwright regression.
2. Run both focused checks against unchanged production code and record the
   expected failures.
3. Make the minimal `shouldShowReopenStateIcon` change.
4. Rerun both focused checks and record the passing results.

## Verification

From a worktree with dependencies installed:

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/task/session-reopen-menu.test.tsx
cd apps/web && pnpm e2e:run tests/session/multi-session-ux.spec.ts -- --grep "idle waiting session has no question icon"
```

## Files Likely Touched

- `apps/web/components/task/session-reopen-menu.tsx`
- `apps/web/components/task/session-reopen-menu.test.tsx`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`

## Dependencies

None.

## Parallelism

`sequential`. The unit regression, browser regression, and production helper
must share one RED-GREEN cycle.

## Inputs

- `docs/specs/platform/background-work-liveness.md`, especially the status
  semantics and new add-panel scenarios.
- `docs/specs/platform/notifications.md`, which establishes that generic
  `WAITING_FOR_INPUT` is not an explicit request for an answer.
- `apps/web/hooks/use-task-pending-input.ts` for the authoritative
  message-derived pending clarification and permission flags.
- `apps/web/components/task/session-reopen-menu.tsx` and its existing helper
  tests as the implementation pattern.
- `apps/web/e2e/pages/session-page.ts` and
  `apps/web/e2e/tests/session/multi-session-ux.spec.ts` for existing add-panel
  locators and mock-agent setup.

## Output Contract

Report the RED and GREEN results, files changed, any blocker or residual risk,
and update this task to `done` plus the matching checkbox and plan status in
`plan.md`. Do not modify shared session icon mappings or adjacent status
surfaces.
