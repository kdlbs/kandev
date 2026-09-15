---
id: "05-prove-thread-view-behavior"
title: "Prove saved-view behavior and resource bounds"
status: done
wave: 5
depends_on:
  - "04-add-mobile-thread-view-drawer"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-001
  - REQ-UI-THREADS-SAVED-VIEWS-002
  - REQ-UI-THREADS-SAVED-VIEWS-003
  - REQ-UI-THREADS-SAVED-VIEWS-004
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-001.1
  - AC-UI-THREADS-SAVED-VIEWS-001.6
  - AC-UI-THREADS-SAVED-VIEWS-001.7
  - AC-UI-THREADS-SAVED-VIEWS-002.11
  - AC-UI-THREADS-SAVED-VIEWS-003.5
  - AC-UI-THREADS-SAVED-VIEWS-003.6
  - AC-UI-THREADS-SAVED-VIEWS-003.10
  - AC-UI-THREADS-SAVED-VIEWS-003.11
  - AC-UI-THREADS-SAVED-VIEWS-004.2
  - AC-UI-THREADS-SAVED-VIEWS-004.3
  - AC-UI-THREADS-SAVED-VIEWS-004.5
  - AC-UI-THREADS-SAVED-VIEWS-004.6
  - AC-UI-THREADS-SAVED-VIEWS-004.7
  - AC-UI-THREADS-SAVED-VIEWS-004.9
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
  - ../../specs/platform/system-design/viewport-bounded-session-delivery.md
---

# Task 05: Prove Saved-View Behavior and Resource Bounds

## Summary

Add browser evidence for saved view lifecycle, filtering, sorting, column caps,
deep links, responsive parity, persistence, and unchanged stream limits.

## In scope

- Add deterministic desktop fixtures for several workflows, states, agents,
  repositories, review states, and activity times.
- Verify exact-task scope, live filters, sort, column limit, and hidden count.
- Verify Save, reload, view switching, and live settings synchronization.
- Verify deep-link admission and no saved-view mutation.
- Verify an empty result and recoverable settings-write failure.
- Verify the phone drawer, task picker, parity, touch targets, safe area, focus,
  and zero document overflow.
- Keep outgoing WebSocket assertions for selected-session-only detail streams.

## Out of scope

- Load testing the maximum saved user-settings payload.

## Acceptance

- Desktop and mobile user journeys satisfy every mapped criterion.
- Three capped columns never mount a fourth task shell.
- Hidden tasks create no task-session or transcript subscription.
- Reload and a second client reproduce the accepted backend state.
- Existing sidebar saved views remain unchanged throughout the scenarios.

## Verification

Run the focused browser scenarios, then the affected frontend and backend
suites:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/threads-view.spec.ts -- --grep "saved view|column limit")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-view.spec.ts -- --grep "saved view|column limit")
(cd apps/backend && go test ./internal/user/...)
(cd apps && pnpm --filter @kandev/web test -- --run lib/threads components/threads lib/state/slices/ui)
(cd apps/web && pnpm run i18n:check && pnpm run typecheck)
```

## Files likely touched

- `apps/web/e2e/tests/task/threads-view.spec.ts`
- `apps/web/e2e/tests/task/mobile-threads-view.spec.ts`
- `apps/web/e2e/fixtures/`
- `apps/web/components/threads/threads-board.test.tsx`
- `apps/web/lib/state/slices/ui/thread-view-actions.test.ts`

## Dependencies

- Tasks 01 through 04 supply the complete desktop and mobile feature.

## Risks

- Use deterministic fixture activity times so sort assertions do not flake.
- Assert settled subscription ownership after scroll and drawer animations.

## Parallelism

`sequential`

## Results

The latest review pass added deterministic live-cap, attention-recency,
hidden-deep-link, settings-recovery, input-limit, and label-resolution tests.
The focused frontend suite passes 359 tests in 41 files. The focused backend
suites pass 1,815 tests in 7 packages.

Desktop browser coverage saves a capped view, restores it after reload, reports
hidden columns, and switches back to the built-in view. Mobile browser coverage
exercises the drawer, editor, task picker, focus restoration, 44-pixel
controls, safe-area clearance, and horizontal-overflow protection.

Typecheck, ESLint, Prettier, localization checks, the localization ratchet,
specification lint, and diff checks pass. The full web test command remains
environment-limited by an integration test that expects a service on
`localhost:3000`. The full backend command has existing config and launcher
failures because the shared `/root/.kandev/config.yaml` takes precedence over
temporary test homes.
