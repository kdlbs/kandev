---
id: "01-recover-task-reads"
title: "Recover task reads across a health cycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-005
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
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 01: Recover task reads across a health cycle

## Summary

Extend the shared recovery owner across a short persistence outage and preserve already visible task content.
Deliver the clock, route behavior, accessible recovery presentation, and desktop/phone regressions together.

## In scope

- Add the private visible-time clock and the retry policy from the Task navigation recovery design section.
- Keep one shared promise and distinguish deadline exhaustion from navigation cancellation.
- Preserve the last temporary failure during pending recovery so the notice remains stable.
- Carry temporary classification through route errors and retain only the current eligible task projection.
- Keep selected-session and draft continuity without replaying old route hydration data.
- Show progress during cold recovery and an in-flow notice over retained content.
- Change `TaskNavigationReadFeedback` from absolute positioning to a strip below the header and above the session tabs and panels.
- Include the strip in mobile header-offset accounting and prove that it cannot cover the header or composer.
- Keep Retry's label stable, disable duplicate recovery, and leave overview navigation available.
- Update every affected timing fixture, including existing tests that exhaust after three failed reads.
- Update translated availability copy and add public task-recovery guidance after behavior works.

## Out of scope

Backend code, health semantics, writer attribution, generic retries, mutation queues, new configuration, and unrelated UI changes.

## Acceptance

1. A thirteen-second structured persistence outage and the fifteen-second boundary recover without another user event. Never-settling reads stop at the cycle deadline.
2. Current task content survives temporary failures during retry and exhaustion. A permanent error or invalidated scope removes retention. Session and draft ownership stay intact.
3. Desktop and phone show progress, reachable overview navigation, and stable Retry behavior. Exhaustion and manual recovery remain functional within the declared request budget.

## ASCII UI preview

Use [UI-01 and UI-02](plan.md#ascii-ui-preview) and their acceptance mappings.

```text
UI-01, desktop and phone
Task temporarily unavailable
Retrying...
[Retry (disabled)]
[Back to task overview]

UI-02, desktop
[Temporary availability notice. Retrying...  Retry (disabled)]
[Current task, selected session, and draft                  ]

UI-02, phone
[Temporary availability notice.]
[Retrying...                   ]
[      Retry (disabled)        ]
[Selected conversation / draft ]
```

Exhaustion enables Retry. Success removes the notice without remounting the conversation.
Retain one route scroll owner and safe-area clearance.
Phone actions measure at least 44px. Ordinary fine-pointer desktop actions measure 28px.
The phone notice wraps in normal flow. It does not open a drawer or cover the composer.
Copy and spacing are illustrative. All product copy uses translation keys.

## Regression sequence

Start with a fake-timer owner regression for a thirteen-second outage.
The current implementation fails at about seven seconds and never issues the later recovery attempt.
Add a route/component regression with a current eligible projection and an exhausted temporary read.
The current route drops the projection because it only preserves projections during loading.
Record both expected failures before changing production behavior.

Cover the following clock cases with fake timers:

- Prompt failures respect attempt spacing and the five-attempt cap.
- Slow or unresolved reads obey both the individual deadline and the total visible-time budget.
- Hidden time pauses the budget and retry delay. An existing attempt still has its own deadline.
- Returning to a hidden active cycle resumes its remaining budget and sends no duplicate request.
- Retry-After beyond the remaining budget never causes an early retry.
- Deadline expiry aborts task/session work, publishes temporary exhaustion, and rejects late success.
- Navigation, authentication/workspace changes, unmount, and final release dispose owned timers and listeners.
- Manual and foreground recovery share an active cycle and can start one new cycle only under the existing rules.

Keep the existing session-list fallback and shared-consumer cancellation assertions.
Test retained current projections and loaded details separately.
Add negative retention cases for another task, changed scope, 404, 401/403, and invalid requested-session ownership.
Do not use a previous route snapshot as a fallback for a different destination.

## Verification

Run the RED cases first, then the full focused block after implementation.
Run each E2E command sequentially. The managed runner builds fresh web and backend artifacts.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)` once.

```bash
(cd apps/web && pnpm exec vitest run lib/state/task-navigation-reads.test.ts lib/state/task-navigation-reads-retry.test.ts lib/state/task-navigation-reads-budget.test.ts lib/state/task-navigation-recovery-clock.test.ts src/task-detail-route.test.tsx src/task-detail-route-session-selection.test.tsx src/task-detail-route-recovery.test.tsx src/task-detail-route-view.test.ts components/task/task-page-content.test.tsx components/task/task-page-content-permanent-read.test.tsx components/task/task-removal-boundary.test.tsx components/task/task-page-mobile-feedback.test.ts components/task/task-page-content-helpers.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/state/task-navigation-reads.ts lib/state/task-navigation-attempt.ts lib/state/task-navigation-recovery-clock.ts src/task-detail-route.tsx src/task-detail-route-state.ts src/task-detail-route-view.ts src/task-detail-route-recovery.ts components/task/task-page-content.tsx components/task/task-page-inner.tsx components/task/task-page-inner-helpers.ts components/task/task-navigation-read-feedback.tsx components/task/task-page-content-helpers.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium tests/task/task-transient-read-recovery.spec.ts tests/task/task-loading-state.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome tests/task/mobile-task-transient-read-recovery.spec.ts tests/task/mobile-task-loading-state.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

All delivered test suites and changed source files are included above.
Capture a desktop and phone recovery state and record the actual discovered and passing E2E counts.
The narrow fine-pointer phone viewport also needs the 44px target assertion when controls use responsive sizing.

## Files likely touched

- `apps/web/lib/state/task-navigation-reads.ts` and `task-navigation-reads.test.ts`.
- New `apps/web/lib/state/task-navigation-recovery-clock.ts` and its test.
- `apps/web/src/task-detail-route.tsx`, `task-detail-route-state.ts`, and `task-detail-route-view.ts`.
- `apps/web/src/task-detail-route.test.tsx` and `task-detail-route-recovery.test.tsx`.
- `apps/web/components/task/task-page-content.tsx`, its test, and `task-page-inner.tsx`.
- `apps/web/components/task/task-navigation-read-feedback.tsx` and `task-page-content-helpers.ts` with its existing test.
- `apps/web/e2e/tests/task/task-transient-read-recovery.spec.ts` and its `mobile-` counterpart.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/common.json` and generated pseudo catalogs.
- `docs/public/tasks-and-workflows.md`, under Troubleshooting.

## Dependencies

None. The existing shared read owner, projection selector, and translated recovery surfaces are already present.

## Risks

- A budget timer that aborts the navigation controller can accidentally suppress the failed snapshot.
- Resetting the clock on a render or foreground event can create unbounded recovery.
- Retained content can leak across task or workspace boundaries without the existing projection eligibility checks.
- Existing three-failure tests must still prove exhaustion, rather than accidentally testing an intermediate retry.
- A timing change can amplify optional session reads unless their existing shared ownership remains intact.

## Parallelism

`sequential`

## Inputs

- REQ-PLATFORM-INTERACTIVE-READS-005 and AC-005.1 through AC-005.10 in the linked requirement.
- Task navigation recovery in the linked system design.
- `TaskNavigationReads`, `useTaskRouteProjection`, `TaskLoadErrorState`, and current desktop/phone recovery tests.
- The completed task-navigation and journey-loading packages linked from the plan.
- `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer` before implementation and product checks.

## Results

Implemented the shared visible-time recovery clock and extended `TaskNavigationReads`
to one initial attempt plus four retries at 2, 5, 10, and 10 seconds within a
30-second visible-tab budget. Hiding the tab pauses both the retry delay and the
budget; an in-flight read keeps its own ten-second deadline. Each pending delay
accounts for its own visible time, so hiding after a slow attempt cannot shorten a
later retry or a server Retry-After value. Budget expiry aborts outstanding task
and session work, preserves a temporary failed snapshot, and rejects late
success. Retry-After larger than the remaining budget waits for budget exhaustion
instead of retrying early. Manual recovery starts one new cycle, and repeated
triggers share the active promise.

`TaskDetailRouteState` now carries temporary classification. Route rendering keeps
the current eligible projection during loading and temporary failures, without
replaying a previous route snapshot or hydration payload. A later permanent
failure from the current shared cycle invalidates the retained projection and any
loaded route view, while the page shows the generic unavailable surface. Permanent
failures do not leave a deleted or inaccessible task looking available. The
cold-load recovery surface shows localized progress with a disabled Retry and the
overview link. The retained-content notice is now an in-flow strip below the task
header, participates in the phone header-offset calculation, and keeps the Retry
accessible name stable. Neutral availability copy replaced the connection-
attribution sentence in all seven catalogs.

Verification passed:

- Focused Vitest: 144 tests across 13 files, including later-delay pause accounting
  and temporary-to-permanent invalidation regressions.
- `pnpm run typecheck`.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, and `pnpm run i18n:check`.
- Targeted ESLint with no errors.
- Chromium recovery and loading specs: 8 passed.
- Mobile Chrome recovery and loading specs: 6 passed.
- Public-doc validation and specification validation.
- `git diff --check`.
