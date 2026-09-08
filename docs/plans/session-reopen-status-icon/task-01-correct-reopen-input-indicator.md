---
id: "01-correct-reopen-input-indicator"
title: "Correct the reopen-menu input indicator"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/background-work-liveness.md"
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
- A reload regression preserves a real pending clarification on a secondary
  session row when that session's messages are not loaded.

## TDD Sequence

1. Update the pure-helper assertion and add the focused Playwright regression.
2. Run both focused checks against unchanged production code and record the
   expected failures.
3. Make the minimal `shouldShowReopenStateIcon` change.
4. Add the per-session pending-action projection and message-aware fallback.
5. Rerun the focused checks and the reload regression and record the passing
   results.

## Verification

From a worktree with dependencies installed:

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/task/session-reopen-menu.test.tsx
cd apps/web && pnpm e2e:run tests/session/multi-session-ux.spec.ts -- --grep "idle waiting session has no question icon"
```

## Files Likely Touched

- `apps/web/components/task/session-reopen-menu.tsx`
- `apps/web/components/task/session-reopen-menu.test.tsx`
- `apps/web/hooks/use-task-pending-input.ts`
- `apps/web/hooks/use-task-pending-input.test.tsx`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`

## Dependencies

None.

## Parallelism

`sequential`. The unit regression, browser regression, and production helper
must share one RED-GREEN cycle.

## Inputs

- `docs/specs/platform/requirements/background-work-liveness.md`, especially the status
  semantics and new add-panel scenarios.
- `docs/specs/platform/requirements/notifications.md`, which establishes that generic
  `WAITING_FOR_INPUT` is not an explicit request for an answer.
- `apps/web/hooks/use-task-pending-input.ts` for the authoritative
  message-derived pending clarification and permission flags.
- `apps/web/components/task/session-reopen-menu.tsx` and its existing helper
  tests as the implementation pattern.
- `apps/web/e2e/pages/session-page.ts` and
  `apps/web/e2e/tests/session/multi-session-ux.spec.ts` for existing add-panel
  locators and mock-agent setup.

## Output Contract

RED results:

- Unit: failed as expected because plain `WAITING_FOR_INPUT` returned `true`.
- E2E: failed as expected because the reopen row contained one
  `tabler-icon-message-question` glyph.

GREEN results:

- `pnpm --filter @kandev/web exec vitest run
  components/task/session-reopen-menu.test.tsx` — 9 tests passed.
- `pnpm e2e:run tests/session/multi-session-ux.spec.ts -- --grep "idle waiting
  session has no question icon"` — 1 Chromium test passed against the managed
  production build.

Remediation results:

- Frontend unit tests: 20 tests passed across the pending-input hook and
  reopen-menu helper suites.
- Backend package tests: `go test ./internal/backendapp ./internal/task/dto
  ./internal/task/handlers` passed.
- Reload E2E: the secondary pending clarification row retained the projected
  message-question icon after its transcript was evicted; the idle waiting
  regression remained icon-free.

Files changed:

- `apps/web/components/task/session-reopen-menu.tsx`
- `apps/web/components/task/session-reopen-menu.test.tsx`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`
- `apps/web/hooks/use-task-pending-input.ts`
- `apps/web/hooks/use-task-pending-input.test.tsx`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- This task and its plan status.

No blockers or residual risks remain within scope. Shared session icon mappings
and adjacent status surfaces were not modified.
