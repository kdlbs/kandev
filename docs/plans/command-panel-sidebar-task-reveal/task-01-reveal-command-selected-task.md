---
id: "01-reveal-command-selected-task"
title: "Reveal command-selected sidebar task"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/command-panel-sidebar-task-reveal.md"
---

# Task 01: Reveal command-selected sidebar task

## Acceptance

- The desktop regression test fails before production changes because Cmd+K navigation leaves the
  chosen rendered task row outside the overflowing sidebar viewport, then passes after the fix.
- Cmd+K task selection navigates normally and reveals an available row with nearest-block scrolling
  without moving focus, scrolling the document, or changing sidebar view/collapse preferences.
- Guarded navigation can outlive the initial reveal retry budget without losing the queued reveal;
  a newer selection supersedes an older pending reveal.
- Phone Cmd+K task navigation remains direct and does not target the hidden desktop sidebar.

## Verification

1. `cd apps && pnpm install --frozen-lockfile`
2. RED, before production changes: `cd apps/web && pnpm e2e:run --host --no-build tests/task/sidebar-scroll-preservation.spec.ts --grep "after a delayed settings navigation blocker"`
3. Unit: `cd apps/web && pnpm exec vitest run lib/sidebar/task-navigation.test.ts`
4. Typecheck: `cd apps/web && pnpm exec tsc --noEmit`
5. Build: `cd apps && pnpm --filter @kandev/web build:vite`
6. Desktop GREEN: `cd apps/web && pnpm e2e:run --host --no-build tests/task/sidebar-scroll-preservation.spec.ts`
7. Mobile GREEN: `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-command-panel-task-navigation.spec.ts`

Confirm Playwright discovers the expected focused tests before treating either browser command as
evidence. The managed runner performs the required production build and teardown.

## Files likely touched

- `apps/web/lib/sidebar/task-navigation.ts`
- `apps/web/lib/sidebar/task-navigation.test.ts`
- `apps/web/hooks/use-command-panel-task-navigation.ts`
- `apps/web/components/command-panel.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`
- `apps/web/e2e/tests/task/mobile-command-panel-task-navigation.spec.ts`

## Dependencies

None.

## Parallelism

Sequential; the browser regression, navigation helper, command-panel integration, and responsive
guard form one TDD vertical slice.

## Inputs

- Behavioral contract: `docs/specs/ui/requirements/command-panel-sidebar-task-reveal.md`.
- Root-cause trace and frontend design: `plan.md`.
- Retryable navigation precedent: `apps/web/lib/review/navigation.ts` and its unit test.
- Existing overflowing-sidebar fixture: `apps/web/e2e/tests/task/sidebar-scroll-preservation.spec.ts`.
- Mobile composition precedent: `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`.

## Output contract

Report the RED failure, implementation summary, actual files changed, exact test commands and
counts, generated artifact paths, cleanup evidence, blockers, and remaining risks. Mark this task
`done`, check it in `plan.md`, and replace the plan/task verification placeholders only after all
targeted checks pass.

## Results

Implemented the bounded visible-sidebar reveal helper, latest-request cancellation, post-navigation
command-panel queue, task-row marker, and desktop/mobile coverage. The RED supersession unit test
and delayed-blocker desktop regression failed before their respective fixes and passed afterward.

- Unit: 1 file, 9 tests passed; task-navigation hook and command-panel consumers: 3 files, 24 tests passed.
- Typecheck and Vite build: passed.
- Desktop full sidebar-scroll E2E: 8 passed, including delayed guarded navigation and above-viewport reveal.
- Mobile focused E2E: 1 passed under `mobile-chrome`.
- Focused Prettier and ESLint (`--max-warnings 0`): passed.
- `git diff --check`: passed.
- Generated E2E output is disposable and excluded from the change; no security or trust-boundary
  changes were introduced.
