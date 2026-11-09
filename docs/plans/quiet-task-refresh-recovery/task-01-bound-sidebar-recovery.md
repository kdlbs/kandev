---
id: "01-bound-sidebar-recovery"
title: "Bound sidebar refresh recovery"
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
  - AC-PLATFORM-INTERACTIVE-READS-007.5
  - AC-PLATFORM-INTERACTIVE-READS-007.6
  - AC-PLATFORM-INTERACTIVE-READS-007.7
  - AC-PLATFORM-INTERACTIVE-READS-007.8
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 01: Bound sidebar refresh recovery

## Summary

Replace the provisional-page failure loop with one bounded recovery cycle per shared query.
Keep rows quiet during automatic recovery and retain one stable error after exhaustion.

## In scope

- `SidebarTaskPageCache` consumer lifetime, bounded runner, and exhausted-cycle admission.
- Shared temporary HTTP classification and compatibility with task-navigation classification.
- Hook trigger intent, provisional/queued refresh coordination, error stability, and quiet accessible progress.
- Desktop and phone sidebar regression scenarios, including affected existing failure fixtures.

## Out of scope

Task-detail warning changes, new query semantics, cache expansion, backend admission changes, and writer diagnosis.

## Acceptance

1. RED: reproduce a provisional page followed by repeated structured 503 responses without new events.
   The existing code makes eight requests in two seconds. The correction obeys the shared delay and cycle bounds.
   Cover five attempts, ten-second attempt deadlines, thirty-second visible time, Retry-After, hidden tabs, and ignored aborts.
2. Desktop and phone consumers share pending and terminal recovery state.
   Live invalidations and queued refreshes cannot bypass delays or restart exhaustion.
   Retry and a later foreground episode start one new cycle, retaining an existing warning until success.
   Preserve deletion, disclosure, authorization, page-bound, StrictMode, reconciliation, and final-release behavior.
3. Focused desktop and phone E2E prove thirteen-second quiet recovery, exhausted Retry, and preserved conversation, draft, rows, and scroll.
   Initial recovery shows loading rather than empty success. Permanent errors remain immediate.
   Phone actions retain 44px targets, contained scrolling, dismissal, and focus return.

## ASCII UI preview

UI-01 / UI-02; [full preview](plan.md#ascii-ui-preview).

```text
Automatic recovery              Exhausted recovery
TASKS [View] [Filters]          TASKS [View] [Filters]
Task A                         Tasks could not be refreshed.
Task B                         [Retry]
                               Task A
```

The phone picker uses the same order with a touch-sized Retry after exhaustion.
Its existing drawer/menu body owns scrolling. No warning or extra gap appears before exhaustion.
This maps to the AC-007 criteria in frontmatter. Existing localized copy and design tokens remain authoritative.

## Verification

Run from the repository root. Install workspace dependencies first if this worktree lacks them.
The managed E2E runner rebuilds the application. Run desktop and phone commands sequentially.

```bash
(cd apps/web && pnpm exec vitest run lib/api/temporary-read-error.test.ts lib/sidebar/sidebar-task-page-recovery.test.ts lib/sidebar/sidebar-task-page-cache.test.ts hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-sidebar-task-page.recovery.test.tsx hooks/domains/kanban/use-sidebar-task-page.disclosure.test.tsx hooks/domains/kanban/use-sidebar-task-page.removal.test.tsx components/task/sidebar-task-query-status.test.tsx lib/state/task-navigation-reads.test.ts lib/state/task-navigation-reads-retry.test.ts lib/state/task-navigation-reads-budget.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/api/temporary-read-error.ts lib/sidebar/sidebar-task-page-recovery.ts lib/sidebar/sidebar-task-page-cache.ts hooks/domains/kanban/use-sidebar-task-page.ts hooks/domains/kanban/sidebar-task-page-load.ts hooks/domains/kanban/sidebar-task-page-refresh.ts components/task/sidebar-task-query-status.tsx lib/state/task-navigation-reads.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-task-pagination.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-pagination.spec.ts tests/layout/mobile-sidebar-read-recovery.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/lib/api/temporary-read-error.ts` and `.test.ts` (new).
- `apps/web/lib/sidebar/sidebar-task-page-recovery.ts` and `.test.ts` (new).
- `apps/web/lib/sidebar/sidebar-task-page-cache.ts` and `.test.ts`.
- `apps/web/lib/state/task-navigation-reads.ts` and its classification tests.
- `apps/web/hooks/domains/kanban/use-sidebar-task-page.ts`, `sidebar-task-page-load.ts`, `sidebar-task-page-refresh.ts`, and their test files, including the recovery suite.
- `apps/web/components/task/sidebar-task-query-status.tsx` and `.test.tsx`.
- `apps/web/e2e/tests/task/sidebar-view-reuse-fixtures.ts`.
- `apps/web/e2e/tests/task/sidebar-task-pagination.spec.ts` and `mobile-sidebar-task-pagination.spec.ts`.

Reuse `TaskNavigationRecoveryClock` without renaming its existing API.
Extract hook helpers only if repository limits require it. Add changed helper tests to the focused command.

## Dependencies

None. The existing task-navigation schedule and clock supply the recovery policy.

## Risks

- Releasing the shared record at settlement loses terminal admission state.
- A retry's read journal must not replay older task values over accepted WebSocket updates.
- Multiple responsive mounts can duplicate retries or progress announcements.

## Parallelism

`sequential`

## Inputs

- [Quiet task refresh recovery](../../specs/platform/system-design/interactive-read-availability.md#quiet-task-refresh-recovery).
- Existing cache, hook, disclosure/removal tests, and sidebar reuse E2E fixtures.
- `apps/web/AGENTS.md`, mobile-parity, TDD, and E2E guidance.

## Results

Implemented on 2026-10-29. All acceptance checks passed.

- `lib/api/temporary-read-error.ts` owns the shared temporary/permanent classification and Retry-After parsing. Task-navigation classification delegates to it, and a per-attempt deadline is a distinct temporary error.
- `lib/sidebar/sidebar-task-page-recovery.ts` runs one bounded cycle: five attempts, delays of 2, 5, 10, and 10 seconds, a ten-second per-attempt deadline, and the existing visible-time clock. Cycle expiry and cancellation abort the current attempt even when a mocked transport ignores cancellation.
- `SidebarTaskPageCache` keeps one recovery record per query. Consumer leases span the displayed query, and the terminal record survives promise settlement. The exhausted latch rejects background admission without a request. Manual Retry and a new foreground episode start one cycle. Entity ownership moved from the settled request to the record and is released with the final lease.
- `useSidebarTaskPage` holds one lease for the displayed query, keeps the page request pending through the cycle, stops the provisional refresh loop while a warning is latched, keeps the warning and a disabled Retry during explicit recovery, and forwards foreground episodes. Load and refresh helpers moved to `sidebar-task-page-load.ts` and `sidebar-task-page-refresh.ts` to stay inside the file and function limits.
- The sidebar view-reuse fixture now expects quiet automatic recovery and awaits exhaustion before asserting the warning, Retry, and 44px phone target. Manual recovery requires an enabled Retry and a matching successful sidebar response; interaction failures are not suppressed.
- Review remediation: a foreground event raised during the active cycle is marked on the shared recovery record through the consumer lease, and the hook queues only a background trailing refresh. A latched terminal error drops queued refreshes instead of restarting recovery. Cache and hook regressions cover the claimed episode, mixed foreground and invalidation triggers, shared consumers, and a genuinely later foreground episode.

Focused verification passed:

- `vitest`: 11 files, 126 tests for classification, the recovery cycle, the page cache, the hook including admission rules, disclosure, removal, the query status, and task-navigation reads.
- `pnpm run typecheck`, ESLint on changed files, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run e2e:sleep-ratchet` passed.
- Desktop `sidebar-task-pagination.spec.ts`: 3 passed, including the view-reuse recovery scenario (2.6 minutes).
- Phone `mobile-sidebar-task-pagination.spec.ts` and `mobile-sidebar-read-recovery.spec.ts`: 5 passed, including the exhausted Retry and drawer recovery scenarios (3.5 minutes).
- The broader `lib`, `hooks`, `components`, and `src` Vitest runs for the affected modules passed.
