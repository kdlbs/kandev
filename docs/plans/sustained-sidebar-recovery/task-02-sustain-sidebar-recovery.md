---
id: "02-sustain-sidebar-recovery"
title: "Sustain sidebar recovery with retained rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-007
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-007.1
  - AC-PLATFORM-INTERACTIVE-READS-007.2
  - AC-PLATFORM-INTERACTIVE-READS-007.3
  - AC-PLATFORM-INTERACTIVE-READS-007.4
  - AC-PLATFORM-INTERACTIVE-READS-007.6
  - AC-PLATFORM-INTERACTIVE-READS-007.8
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 02: Sustain sidebar recovery with retained rows

## Summary

When sidebar rows are already displayed, recover quietly for up to 150 seconds of visible time
before warning. After a warning, let a later live update restart recovery once sixty seconds have passed.

## In scope

- Schedule selection in `lib/sidebar/sidebar-task-page-recovery.ts`: default 5 attempts / 30 s;
  sustained 9 attempts with delays 2, 5, 10, 10, 20, 30, 30, 30 s / 150 s, via the clock's `budgetMs`.
- `hasRetainedRows` request option, passed from `beginSidebarLoad` for a matching displayed response or cached first page.
- `RecoveryRecord.exhaustedAt` and the sixty-second background cooldown in `SidebarTaskPageCache.admit()`;
  disclosure-sync exhaustion keeps its explicit-Retry-only latch.
- Update the sidebar paragraph in `apps/web/AGENTS.md` to the new schedule and admission rule.

## Out of scope

Task-detail and task-navigation recovery, copy changes, new UI, backend changes, and cache budgets.

## Acceptance

1. RED: with a displayed page, a cycle of temporary failures lasting ninety seconds of visible time recovers
   without publishing `sidebar:queryRefreshFailed`; current code publishes it at about thirty seconds.
   Without a displayed page, the 5-attempt / 30-second schedule is unchanged.
2. After exhaustion, a background admission under sixty seconds sends no request and keeps the warning;
   one at or after sixty seconds starts exactly one cycle, and success clears the warning.
   Manual Retry and a new foreground episode still start immediately. Disclosure-sync exhaustion still ignores background events.
3. Joining consumers do not change an active cycle's schedule or budget. Hidden tabs pause the visible budget.
   Retry-After is honored. Existing deletion, disclosure, access-denial, and reconciliation tests pass.

## ASCII UI preview

UI-01 / UI-02; [full preview](plan.md#ascii-ui-preview).

```text
Rows retained, 0 to 150 s          After 150 s of failures
TASKS [All tasks] [Filter]         TASKS [All tasks] [Filter]
Task A                  24m        Tasks could not be refreshed.  [Retry]
Task B                  12s        Task A                  24m
```

Phone surfaces follow the same states in their existing containers. No layout or copy changes.

## Verification

Run from the repository root. Run E2E projects sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/sidebar/sidebar-task-page-recovery.test.ts lib/sidebar/sidebar-task-page-cache.test.ts lib/sidebar/sidebar-task-page-cache-cooldown.test.ts hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-sidebar-task-page.recovery.test.tsx hooks/domains/kanban/use-sidebar-task-page.disclosure.test.tsx hooks/domains/kanban/use-sidebar-task-page.removal.test.tsx lib/state/task-navigation-reads-budget.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-task-pagination.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-pagination.spec.ts tests/layout/mobile-sidebar-read-recovery.spec.ts)
git diff --check
```

The existing view-reuse E2E fixture waits for exhaustion. Update its wait to the 150-second retained-row budget
only if that scenario retains rows; otherwise leave it unchanged.

## Files likely touched

- `apps/web/lib/sidebar/sidebar-task-page-recovery.ts` and `.test.ts`.
- `apps/web/lib/sidebar/sidebar-task-page-cache.ts`, `sidebar-task-page-record.ts`, and `.test.ts`.
- `apps/web/hooks/domains/kanban/sidebar-task-page-load.ts` and the recovery hook test.
- `apps/web/e2e/tests/task/sidebar-view-reuse-fixtures.ts`, only if required.
- `apps/web/AGENTS.md`.

## Dependencies

None.

## Risks

- Selecting the schedule per consumer instead of per cycle would let a joining consumer extend a budget.
- The cooldown must compare against the cycle end, not the last rejected admission, or bursts would postpone it forever.

## Parallelism

`sequential`

## Inputs

- [Sidebar recovery ownership](../../specs/platform/system-design/interactive-read-availability.md#sidebar-recovery-ownership).
- [Quiet task refresh recovery Task 01](../quiet-task-refresh-recovery/task-01-bound-sidebar-recovery.md) results.
- `apps/web/AGENTS.md`, mobile-parity, TDD, and E2E guidance.

## Results

- The specified Vitest suites passed: 8 files, 93 tests, including cached-page remount recovery and cooldown boundaries for cold loads and retained-row reads.
- `pnpm run typecheck` and `pnpm run lint` passed.
- `pnpm --filter @kandev/web build:e2e` passed.
- The backend E2E app and host fixtures built with `make build-kandev` and `make build-mock-agent build-e2e-delivery-fixture build-e2e-send-now-fixture e2e-plugin-package`.
- The managed host Chromium E2E passed: 3 tests. The managed host mobile-chrome E2E passed: 5 tests.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- E2E used `--host --no-build` after building the app, production assets, and fixtures separately. The broader multi-platform `make build` was stopped during the macOS/amd64 helper build; that cross-platform target is outside this work order.
