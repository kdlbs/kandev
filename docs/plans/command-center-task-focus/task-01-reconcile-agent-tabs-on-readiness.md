---
id: "01-reconcile-agent-tabs-on-readiness"
title: "Reconcile Agent tabs on workbench readiness"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-agent-tab-reconciliation.md"
---

# Task 01: Reconcile Agent Tabs on Workbench Readiness

## Outcome

Opening a task with several sessions renders every Agent tab on the first task
view, including when task hydration finishes before Dockview becomes ready.

## In scope

- Add a permanent hook regression for API-late readiness.
- Make `useAutoSessionTab` react to Dockview API readiness while preserving its
  existing session and active-panel reconciliation.
- Add a command-panel multi-session Playwright scenario with no page reload.

## Exclusions

- Session lifecycle, ordering, or backend changes.
- Saved layout geometry and non-Agent active-panel rules.
- Mobile session-control redesign.

## Requirements and design

- `REQ-UI-TASK-AGENT-TAB-RECONCILIATION-001`
- `AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.1` through
  `AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.5`
- `docs/specs/ui/system-design/task-agent-tab-reconciliation.md`

## Implementation acceptance

1. A hook test fails before the production change when sessions hydrate before
   the Dockview API, then passes after the API is published.
2. API readiness triggers one reconciliation path that reads the latest task
   and session state. Existing session-key updates continue to reconcile.
3. Cmd+K opens a seeded multi-session task and shows all Agent tabs without a
   reload.

## TDD and verification

1. RED: add the API-late hook case, run it against current production code, and
   record the missing reconciliation assertion.
2. Unit GREEN: `cd apps/web && pnpm exec vitest run components/task/dockview-session-tabs.hook.test.tsx components/task/dockview-session-tabs.test.ts`
3. Typecheck: `cd apps/web && pnpm run typecheck`
4. E2E GREEN: `cd apps/web && pnpm e2e:run --host --no-build tests/session/multi-session-ux.spec.ts --grep "command panel navigation renders all existing session tabs"`

Make sure that Playwright discovers the focused scenario. Then accept the
browser result as evidence.

## Files likely touched

- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-session-tabs.hook.test.tsx`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`

## Dependencies and parallelism

No code dependency. Execute in the primary session. Do not delegate unless the
user explicitly authorizes implementation agents.

## Results

Completed on 2026-08-26.

### RED evidence

- `pnpm exec vitest run components/task/dockview-session-tabs.hook.test.tsx`
  failed 1 test because publishing the late Dockview API did not trigger a
  reconciliation.

### GREEN evidence

- `pnpm exec vitest run components/task/dockview-session-tabs.hook.test.tsx components/task/dockview-session-tabs.test.ts` passed 2 files and 35 tests.
- `pnpm run typecheck` passed.
- `pnpm e2e:run --host --no-build tests/session/multi-session-ux.spec.ts --grep "command panel navigation renders all existing session tabs"` passed 1 Chromium test.

The implementation reads the current application state inside the existing
reconciliation path and only adds the Dockview API readiness subscription.
No backend or mobile surface changed.
