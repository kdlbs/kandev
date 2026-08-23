---
id: "02-session-deletion-e2e"
title: "Session deletion E2E"
status: done
wave: 2
depends_on: ["01-inline-delete-feedback"]
plan: "plan.md"
spec: "../../specs/ui/session-tab-delete-feedback.md"
---

# Task 02: Session deletion E2E

## Intent

Prove the integrated desktop no-toast deletion result and preserve mobile capability parity through
the existing Sessions picker.

## Acceptance

- The existing desktop X-confirm-delete scenario verifies the selected tab and backend session are
  removed without a deletion progress or success toast.
- Desktop context-menu scenarios use the anchored confirmation popover, not an alert dialog, while
  preserving cancel and confirm outcomes.
- A `mobile-chrome` scenario cancels and then confirms the inline Sessions picker row action,
  verifies no alert dialog opens, and proves the other session remains reachable.
- Both scenarios use stable test IDs or scoped accessible controls and assert user-visible outcomes,
  not API-only behavior.

## Files likely touched

- `apps/web/e2e/tests/session/session-tab-management.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-deletion.spec.ts`
- `apps/web/e2e/pages/session-page.ts` only if a reusable scoped mobile-session action is needed

## Dependencies

Task 01 must be complete so the E2E suite runs against the final production behavior.

## Parallelism

Sequential; it depends on task 01 and shares the session-deletion flow.

## Inputs

- Spec: the desktop success and mobile parity scenarios.
- Plan: `E2E Tests` and `Mobile design contract`.
- Existing patterns: `createTaskWithTwoSessions` in
  `session-tab-management.spec.ts`, `seedTaskSession` in
  `mobile-multi-repository-session-picker.spec.ts`, and mobile touch interaction via `.tap()`.

## Verification

Bootstrap once if this worktree does not already have dependencies:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run the managed production-build runner:

```bash
cd apps/web && pnpm e2e:run tests/session/session-tab-management.spec.ts -- --grep "tab close button shows delete confirmation"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-deletion.spec.ts
```

Confirm Playwright discovers one intended test in each focused command before recording the result.

## Output contract

Report the scenarios added or updated, exact discovered/passed test counts, files changed, generated
failure/evidence artifacts if any, cleanup/teardown evidence, blockers, risks, and synchronized
task/plan status.

## Results

- Extended the desktop close-confirmation scenario to assert that neither the deletion progress nor
  success toast appears.
- Added `mobile-session-deletion.spec.ts`, using the native Sessions picker and scoped inline row
  actions to cancel once, delete one of two sessions, and keep the remaining row reachable without
  an alert dialog.
- `rtk pnpm e2e:run tests/session/session-tab-management.spec.ts -- --grep "tab close button shows delete confirmation"` — 1 desktop test passed with a production build.
- `rtk pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-session-deletion.spec.ts` — 1 mobile test passed.
- `rtk pnpm e2e:run --no-build tests/session/session-tab-management.spec.ts tests/session/session-tab-close-guard.spec.ts` — 9 desktop tests passed.

The managed runner used isolated temporary backend/database/task data and cleaned its test-results
and blob-report directories. The initial exploratory mobile run exposed that the Radix dropdown
trigger requires click semantics under the mobile browser; the final test keeps picker/row/dialog
touch interactions on `.tap()` and uses `.click()` only for that trigger. No external systems were
changed.
