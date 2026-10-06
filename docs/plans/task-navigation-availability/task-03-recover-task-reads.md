---
id: "03-recover-task-reads"
title: "Recover temporary task reads"
status: done
wave: 3
depends_on:
  - "02-coalesce-inbox-refreshes"
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
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 03: Recover temporary task reads

## Summary

Distinguish transient navigation failures from missing tasks. Recover through one
shared bounded retry owner, preserving the current task and session selection.

## In scope

- Classify network failures and 429/502/503/504 as temporary. Read persistence
  codes from `ApiError.body.code`. Keep 404, auth, parse, and abort semantics.
- Add cancellation and shared recovery state to the navigation identity owner.
  Route hydration and `useTaskDetails` consume that owner; neither starts its own loop.
- Allow two automatic retries after 2 and 5 seconds, respecting Retry-After.
  Manual Retry starts a new bounded cycle; foreground recovery coalesces.
- Suspend hidden-tab retries and invalidate work on navigation, identity change,
  and unmount. Preserve successful task details and session-list fallback semantics.
- Add localized temporary failure and refresh notices, Retry, and status announcements.
  Keep the permanent missing-task recovery view and workspace-aware overview link.
- Add desktop and phone rendered regressions, with valid non-primary session restoration.

## Out of scope

Automatic page reload, new API routes, mutation retries, authorization changes,
agent launch, new task records, or separate desktop/mobile recovery state.

## Acceptance

1. Structured 503 produces temporary copy, two bounded automatic attempts, and
   working manual recovery. A 404 retains the existing view with no automatic retry.
2. Mixed recovery triggers share one read. Task/identity switches fence pending
   retries and late responses. Successful recovery retains the valid selected session.
3. Loaded details survive failed refresh with a visible notice. Desktop and phone
   show reachable recovery controls, announcements, and no horizontal overflow.

## ASCII UI preview

Use [UI-01 and UI-02](plan.md#ascii-ui-preview), mapped to AC-005.1/.2/.4/.5/.6/.7.

```text
UI-01, desktop
Task temporarily unavailable
We could not load this task. Try again.
[Retry]  [Back to task overview]

UI-01, phone
Task temporarily unavailable
We could not load this task. Try again.
[              Retry              ]
[      Back to task overview       ]

UI-02, loaded task
[Temporary connection problem. Try again.  Retry]
[Existing task and selected session content]
```

Retrying announces progress and disables duplicate actions. Exhaustion leaves
manual Retry. Phone wraps UI-02 and puts Retry below its explanation.
Control order and semantics are required; wording and spacing are illustrative.
Use the missing-task surface and `task-layout.tsx` as nearby mobile precedents.
Keep one route scroll owner, safe-area handling, and at least 44px phone targets.
Fine-pointer desktop buttons use the existing 28px primitive size.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/state/task-navigation-reads.test.ts src/task-detail-route.test.tsx src/task-detail-route-recovery.test.tsx components/task/task-page-content.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --project chromium -- tests/task/task-transient-read-recovery.spec.ts tests/task/task-loading-state.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/task/mobile-task-transient-read-recovery.spec.ts tests/task/mobile-task-loading-state.spec.ts)
```

Run the E2E commands sequentially. Capture one desktop and one phone failure
surface during these tests. Use causal waits for injected HTTP responses and
fake timers for unit retry checks. Rebuild before browser testing.

## Files likely touched

- `apps/web/lib/state/task-navigation-reads.ts` and its tests.
- `apps/web/src/task-detail-route.tsx`, its route-state/view/recovery helpers, and tests.
- `apps/web/components/task/task-page-content.tsx` and its tests.
- A focused recovery helper beside the navigation owner if lint limits require it.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/common.json`.
- New desktop/phone recovery specs in `apps/web/e2e/tests/task/`.

## Dependencies

Task 02 reduces background request amplification before navigation retries ship.

## Risks

Aborting a shared request from one consumer can break another. The navigation
owner controls cancellation. Treat failed session enrichment as unavailable,
not an authoritative empty list. Do not reset retry budgets on ordinary renders.

## Parallelism

`sequential`

## Inputs

- Task navigation recovery in the platform design.
- Existing missing-task route recovery requirements and loading E2E tests.
- Stats temporary-error classification and `ApiError.retryAfterSeconds`.
- Mobile parity and E2E skills before implementation.

## Results

Implemented shared task-navigation reads and a single bounded retry cycle for
route hydration and task-detail refreshes. Network failures, 429/502/503/504,
and the structured `persistence_unavailable` code retry twice after 2 and 5
seconds, respecting `Retry-After`; 404, auth, parsing, and abort failures do not
retry. An exhausted temporary cycle can start one new bounded cycle per
foreground episode; ordinary refreshes reuse the failure. Each attempt owns a
child abort signal, so a task failure cancels its pending session-list sibling
before the next attempt. Pending work is cancelled on navigation, identity
change, and final consumer release. Hidden tabs suspend the retry timer, and
failed optional session-list reads retain the existing empty-session fallback
without hiding task identity failures.

Temporary initial failures keep the permanent missing-task route semantics and
show localized Retry and workspace-aware overview actions. Failed refreshes
preserve loaded task and selected-session content with an in-flow notice. Retry
status is announced, and desktop/phone recovery controls are covered by rendered
regressions. Foreground refreshes of an already loaded route remain lightweight;
manual recovery of an initially failed route still hydrates and enriches it.
The route captures the owned selected session for the navigation generation so
real task-page synchronization cannot clear it while identity is unavailable.
All supported locales received the new copy.

Verification passed:

- Focused Vitest: 50 tests across navigation reads, route hydration and recovery,
  and task-page content.
- Web typecheck, i18n checks, `make build-web`, and `make build-backend`.
- Targeted ESLint completed with no warnings or errors.
- Chromium: 6 recovery/loading tests passed, including initial failure recovery
  and secondary-session retention after refresh recovery.
- Mobile Chrome: 5 recovery/loading tests passed, including initial failure
  recovery, 44px Retry controls, viewport containment, and no horizontal overflow.
