---
id: "02-defer-task-warnings"
title: "Defer task refresh warnings"
status: done
wave: 2
depends_on:
  - "01-bound-sidebar-recovery"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-005
  - REQ-PLATFORM-INTERACTIVE-READS-007
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-005.1
  - AC-PLATFORM-INTERACTIVE-READS-005.2
  - AC-PLATFORM-INTERACTIVE-READS-005.3
  - AC-PLATFORM-INTERACTIVE-READS-005.4
  - AC-PLATFORM-INTERACTIVE-READS-005.5
  - AC-PLATFORM-INTERACTIVE-READS-005.6
  - AC-PLATFORM-INTERACTIVE-READS-005.7
  - AC-PLATFORM-INTERACTIVE-READS-005.8
  - AC-PLATFORM-INTERACTIVE-READS-005.9
  - AC-PLATFORM-INTERACTIVE-READS-005.10
  - AC-PLATFORM-INTERACTIVE-READS-007.1
  - AC-PLATFORM-INTERACTIVE-READS-007.4
  - AC-PLATFORM-INTERACTIVE-READS-007.5
  - AC-PLATFORM-INTERACTIVE-READS-007.7
  - AC-PLATFORM-INTERACTIVE-READS-007.8
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 02: Defer task refresh warnings

## Summary

Keep task details and authorized current projections quiet while the existing recovery cycle runs.
Show the task refresh notice after exhaustion and preserve it through explicit recovery until success.

## In scope

- Owner-level exhausted-warning state, shared across route and task-detail consumers.
- Quiet retained-content progress, neutral cold loading, terminal notices, and phone header-offset behavior.
- Desktop and phone recovery scenarios, public guidance, and package results.

## Out of scope

Backend changes, another retry owner, altered task/session eligibility, optimistic hydration replay, mutations, and navigation redesign.

## Acceptance

1. RED: component and route tests expect no visible refresh strip after the first temporary failure with eligible current content.
   The existing first-error notice fails that assertion. Add owner tests for exhaustion and its latch through manual or foreground recovery.
   Preserve all existing deadline, hidden-time, cancellation, sharing, permanent-error, and selection tests.
2. Cold navigation shows ordinary progress with overview access until exhaustion.
   Retained details and current projections preserve conversation, valid secondary session, draft, and geometry during a thirteen-second outage.
   After exhaustion, Retry remains reachable. During explicit recovery the notice stays stable and Retry stays disabled until settlement.
   Desktop and phone E2E prove both quiet recovery and terminal behavior without another automatic trigger.
3. Update task/sidebar recovery guidance in `docs/public/tasks-and-workflows.md` during implementation.
   All changed copy uses existing translations or all supported catalogs and generated locales.
   Synchronize package statuses and exact results after the targeted checks pass.

## ASCII UI preview

UI-01 / UI-02 / UI-03; [full preview](plan.md#ascii-ui-preview).

```text
Automatic recovery             Exhausted desktop
[Task header]                  [Task header]
[Existing conversation]        [Refresh notice] [Retry]
[Draft remains]                [Existing conversation]

Exhausted phone                No eligible content
[Task header]                  [Loading task...]
[Refresh notice]               [Back to task overview]
[            Retry           ]
[Existing conversation]
```

Automatic recovery adds no visible strip or reserved gap on either viewport.
One screen-reader status announces progress. The terminal phone strip retains safe-area/header clearance and a 44px Retry target.
The existing task content owns scrolling, and the composer keeps its draft.
Copy and spacing are illustrative. This maps to the full AC identifiers in frontmatter.

## Verification

Run from the repository root after Task 01. Run the managed desktop and phone commands sequentially.
If copy changes, generate Traditional Chinese and pseudo catalogs before the i18n checks.

```bash
(cd apps/web && pnpm exec vitest run lib/state/task-navigation-reads.test.ts lib/state/task-navigation-reads-retry.test.ts lib/state/task-navigation-reads-budget.test.ts lib/state/task-navigation-recovery-clock.test.ts src/task-detail-route-recovery.test.tsx src/task-detail-route-view.test.ts components/task/task-page-content.test.tsx components/task/task-page-mobile-feedback.test.ts components/task/task-load-progress-state.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/state/task-navigation-reads.ts src/task-detail-route.tsx components/task/task-page-content.tsx components/task/task-page-content-helpers.ts components/task/task-navigation-read-feedback.tsx components/task/task-layout.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-transient-read-recovery.spec.ts tests/task/task-loading-state.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-transient-read-recovery.spec.ts tests/task/mobile-task-loading-state.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run the PR-documentation coverage preflight against the final diff through `.github/scripts/pr-docs.cjs`.
Use `classifyChangedFiles`, `selectChangedWorkOrders`, and `validateCoverage` with the linked work-order frontmatter.
Do not claim these implementation checks ran during the design turn.

## Files likely touched

- `apps/web/lib/state/task-navigation-reads.ts` and its owner tests.
- `apps/web/components/task/task-navigation-read-feedback.tsx`.
- `apps/web/components/task/task-page-content.tsx` and `.test.tsx`.
- `apps/web/components/task/task-page-content-helpers.ts` and `task-page-mobile-feedback.test.ts`.
- `apps/web/src/task-detail-route.tsx`, `task-detail-route-recovery.test.tsx`, and `task-detail-route-view.test.ts`.
- `apps/web/components/task/task-layout.tsx` if its recovery props require adjustment.
- `apps/web/e2e/tests/task/task-transient-read-recovery.spec.ts` and `mobile-task-transient-read-recovery.spec.ts`.
- `docs/public/tasks-and-workflows.md`.
- Relevant locale catalogs only if existing progress and error keys cannot express the final states.
- This package's plan and work orders.

## Dependencies

Task 01 supplies the shared error classification and completed sidebar behavior.

## Risks

- Checking only `retrying` hides the terminal warning again during explicit recovery.
- A hidden strip with reserved header space still shifts the phone layout.
- An optimistic current-task projection must not survive a later 403 or 404.
- The public docs must distinguish quiet recovery from exhausted recovery and permanent failures.

## Parallelism

`sequential`

## Inputs

- [Task navigation and quiet recovery design](../../specs/platform/system-design/interactive-read-availability.md#task-navigation-recovery).
- Completed [task read resilience](../task-read-resilience/plan.md) and its exact existing scenarios.
- Existing task-detail route, selected-session, draft, mobile feedback, and loading-state tests.
- `apps/web/AGENTS.md`, mobile-parity, TDD, E2E, and docs-maintainer guidance.

## Results

Implemented on 2026-10-29. All acceptance checks passed.

- `TaskNavigationReadSnapshot` now carries an `exhausted` latch. It is set on temporary exhaustion and preserved while an explicit cycle retries. Successful reads and permanent failures clear it; scope changes discard the record.
- `TaskNavigationReadFeedback` shows the in-flow notice only after exhaustion. Automatic recovery with eligible content renders one screen-reader status and no visible strip or reserved gap.
- `TaskLoadProgressState` supplies ordinary progress with a reachable task-overview action and a disabled Retry for cold recovery. `TaskDetailRoute` uses it until exhaustion; `TaskPageContent` uses it while a eligible-content read is still automatic. The phone header offset is reserved only for the terminal strip.
- `docs/public/tasks-and-workflows.md` now separates quiet automatic recovery from exhausted recovery and permanent failures.

Focused verification passed:

- `vitest`: 9 files, 80 tests for task-navigation reads, the recovery clock, task-detail route recovery and view derivation, task-page content, mobile feedback, and the new progress state.
- `pnpm run typecheck`, ESLint on changed files, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run e2e:sleep-ratchet` passed.
- Desktop `task-transient-read-recovery.spec.ts`: 4 passed (2.7 minutes), including quiet thirteen-second recovery and the retained projection notice.
- Phone `mobile-task-transient-read-recovery.spec.ts`: 3 passed (1.5 minutes).
- Desktop `task-loading-state.spec.ts`: 4 passed (1.2 minutes). Loading, missing-route, and session-selection continuity stayed intact.
- Phone `mobile-task-loading-state.spec.ts`: 3 passed (40 seconds).
- `node --test scripts/validate-public-docs.test.mjs` (63 tests) and `node scripts/validate-public-docs.mjs` (54 pages) passed.
- Specification validation, all 36 linter tests, specification lint, the PR-documentation coverage preflight, and `git diff --check` passed.
