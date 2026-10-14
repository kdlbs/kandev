---
id: "04-progressive-task-navigation"
title: "Reveal tasks before optional hydration"
status: complete
wave: 4
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-003
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-003.1
  - AC-UI-SESSION-REFRESH-EFFICIENCY-003.2
  - AC-UI-SESSION-REFRESH-EFFICIENCY-003.3
  - AC-UI-SESSION-REFRESH-EFFICIENCY-003.4
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 04: Reveal tasks before optional hydration

## Summary

Remove optional resources from the client task-switch barrier and share outstanding task identity reads. Preserve boot hydration and all task/session ownership guards.

## In scope

- Add a client-specific progressive navigation path; retain the existing aggregate boot caller contract.
- Join route and task-surface identity reads. Reuse loaded resources and reject late enrichment through navigation/auth generations and hydration epochs.
- Add held-response regressions for settings, workflow, repositories, messages, and full session/model readiness. Cover no-session, missing-task, and A-B-A paths.

## Out of scope

No backend API, deployment, dependency replacement, or general cache migration.

## Acceptance

`reveals destination while optional enrichment is held` and `rejects late A result after A-B-A` in route tests; `shares task identity read` in new coordinator tests. Browser cases hold optional reads and select another task through desktop and phone entry points.

## ASCII UI preview

UI-01: Selecting a task while optional enrichment is pending.

```text
Desktop                         Phone
+---------+------------------+  +-------------------------+
| Task    | Destination      |  | Destination header      |
| sidebar | header           |  | Available conversation  |
|         | Available chat   |  | or local loading state  |
|         | Local loading in |  |                         |
|         | pending panels   |  | Existing bottom nav     |
+---------+------------------+  +-------------------------+
```

Before: the route loading overlay covers the destination until enrichment
settles. After: authorized destination identity appears first; pending surfaces
use their existing loading/error states. The desktop sidebar and phone task
drawer remain usable. Current panels retain their scroll owners, and phone
controls retain touch targets and safe areas. Draft/model readiness still gates
send. This structure is required; spacing is illustrative. No new copy is planned.
Maps to `AC-UI-SESSION-REFRESH-EFFICIENCY-003.1`, `.3`, and `.4`.

See the [combined plan](plan.md#ascii-ui-preview).

## Verification

Start with the named failing behavioral tests; record RED/GREEN evidence.
New test paths below are created by this order. Install workspace dependencies
once before the first package check if this checkout lacks them.

```bash
(cd apps/web && pnpm exec vitest run src/task-detail-route.test.tsx lib/ssr/session-page-state.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec vitest run lib/state/task-navigation-reads.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-switch-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-switch-efficiency.spec.ts)
```

## Files likely touched

- `apps/web/src/task-detail-route.tsx`
- `apps/web/src/task-detail-route.test.tsx`
- `apps/web/lib/ssr/session-page-state.ts`
- `apps/web/lib/ssr/session-page-state.test.ts`
- `apps/web/components/task/task-page-content.tsx`
- `apps/web/components/task/task-route-session-hydration.tsx`
- `apps/web/lib/state/task-navigation-reads.ts (new)`
- `apps/web/e2e/tests/task/task-switch-efficiency.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-task-switch-efficiency.spec.ts (new)`

## Dependencies

No functional dependency on session ETags or the environment polling work. Run sequentially in this package; Task 04 introduces the shared browser scenarios.

## Risks

Late enrichment must not overwrite live messages, profile versions, or model state. Do not turn optional message absence into an authoritative empty transcript.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- [Trace and source evidence](trace-2026-09-29-task-switch.md)

## Results

The client route now reads task identity and the session list first, renders and
hydrates a lightweight destination shell, and starts bounded optional
enrichment only after that shell hydrates. The aggregate `fetchSessionDataForTask`
entry point remains intact for boot callers. Route/task-surface identity reads
join by store, auth scope, route generation, and task; stale A results cannot be
reused after A-B-A navigation. Enrichment uses a new hydration-epoch snapshot
so the shell's own session seed does not incorrectly invalidate the fuller
snapshot, while live updates after enrichment starts remain protected.

Verification passed:

```bash
(cd apps/web && pnpm exec vitest run src/task-detail-route.test.tsx lib/ssr/session-page-state.test.ts lib/state/task-navigation-reads.test.ts components/task/task-page-content.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-switch-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-switch-efficiency.spec.ts)
```

The desktop and phone browser tests held the destination full-session read and
verified the destination task title and chat surface remained available. A
clean-browser performance comparison was not captured, so no timing
improvement is claimed.

Follow-up code-review remediation removes the synthetic empty turn history
from the route shell. Real-store tests prove successful enrichment installs
historical turn metadata after shell hydration, failed optional turn hydration
remains retryable by the normal loader, and later hydration preserves live
turns. Navigation reads carry an immutable store-scope token and the route checks
that exact owner before publishing shell or enrichment state. A deferred real
store test changes workspace scope, replaces the read owner, and confirms that
the old response publishes neither its task nor its session list.
