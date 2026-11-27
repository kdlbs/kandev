---
id: "03-shared-environment-status"
title: "Share environment status polling"
status: complete
wave: 3
depends_on:
  - "02-session-reconciliation-client"
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-002
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-002.1
  - AC-UI-SESSION-REFRESH-EFFICIENCY-002.2
  - AC-UI-SESSION-REFRESH-EFFICIENCY-002.3
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 03: Share Environment Status Polling

## Summary

Give task-environment status one read owner while the toolbar and composer
remain mounted. Preserve their local UI state, active/background cadence,
manual refresh, and task-switch safety.

## In scope

- Add a task/store-keyed, reference-counted shared resource for the raw live
  environment response.
- Migrate both existing hooks to that resource without changing the status
  and action props exposed to their components.
- Cover joined reads, cadence changes, last release, reset, failure, and late
  task response with unit and desktop/phone E2E tests.
- Record the final clean-browser request and main-thread comparison.

## Out of scope

- Executor backend changes, a new status control, and task-panel layout work.

## Acceptance

- Two consumers for one task produce one outstanding read and one schedule,
  using the fastest required cadence.
- Task switch, last unmount, and reset retire or invalidate the old owner.
  stale results do not appear under another task.
- Desktop and phone environment status, manual refresh, reset, and error
  behavior remain usable through the existing surfaces.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-task-environment.test.tsx hooks/domains/session/use-executor-environment-availability.test.ts hooks/domains/session/environment-live-resource.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-refresh-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-refresh-efficiency.spec.ts)
```

## Files likely touched

- `apps/web/hooks/domains/session/environment-live-resource.ts` (new)
- `apps/web/hooks/domains/session/environment-live-resource.test.ts` (new)
- `apps/web/hooks/domains/session/use-task-environment.ts`
- `apps/web/hooks/domains/session/use-task-environment.test.tsx`
- `apps/web/hooks/domains/session/use-executor-environment-availability.ts`
- `apps/web/hooks/domains/session/use-executor-environment-availability.test.ts`
  (extended)
- `apps/web/e2e/tests/session/session-refresh-efficiency.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-refresh-efficiency.spec.ts`

## Dependencies

Task 02 adds the browser scenarios to extend. The environment resource itself
does not depend on the session ETag implementation.

## Risks

- The toolbar and composer have different status presentation needs. Share
  the raw response only, and keep each hook's existing derivation and
  Kubernetes-session lookup scoped to its active session.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- `apps/web/hooks/domains/session/use-task-environment.test.tsx`

## Results

The task/store-keyed resource, mixed toolbar/composer sharing, active/background
polling modes, task-scope isolation, reset invalidation, and settled 404/error
behavior are implemented. The focused resource/hook suite passed (42 tests
across six files), as did `pnpm run typecheck`. Chromium and mobile-chrome E2E
each passed all three scenarios: unchanged session revalidation, stale response
rejection, and shared environment reads with manual refresh and reset. E2E
request counts confirm one initial request across both consumers and a new read
after refresh and reset. A matched clean-browser performance trace was not
captured in this environment, so no timing improvement is claimed.
