---
id: "01-preserve-session-focus"
title: "Preserve session focus during Chat reconciliation"
status: done
wave: 1
depends_on: []
parallelism: sequential
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 01: Preserve Session Focus During Chat Reconciliation

## Acceptance

- Replacing a selected Chat placeholder beside Plan selects the incoming
  session's Agent panel without ever selecting Plan.
- A valid globally focused non-session panel such as Files or Changes remains
  globally focused after the Agent group's selection is established.
- Existing saved non-Agent selections, explicit session switches, tab order,
  stale-panel guards, maximized layouts, and mobile/tablet behavior remain
  unchanged.

## TDD Sequence

1. RED: extend the session-tabs fake to reproduce a center Chat/Plan group with
   Chat selected while Files is globally active; prove the current effect
   activates Plan.
2. RED: add the desktop Playwright scenario by creating an agent plan, resolving
   its environment ID, and seeding the v3 environment layout with center
   `activeView: "chat"` and global `activeGroup: "group-right-top"`; prove the
   rendered task opens Plan or loses its unseen state.
3. GREEN: place the inactive session in Chat's session-first replacement slot,
   select it before removing Chat, then restore the still-live global
   non-session panel.
4. REFACTOR: keep group-selection and global-focus preservation explicit and
   reuse the existing activation policy without widening the change.
5. Run the exact targeted unit, typecheck, and production-build E2E commands.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tab-activation.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/layout/saved-layout-session-isolation.spec.ts \
  -- --grep "preserves Agent selection while Files owns global focus"
```

## Files Likely Touched

- `apps/web/components/task/dockview-session-tab-activation.ts`
- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-session-tabs.test.ts`
- `apps/web/components/task/dockview-session-tabs.test-utils.ts`
- `apps/web/e2e/tests/layout/saved-layout-session-isolation.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The unit fake, production behavior, and browser scenario describe
one ordering-sensitive Dockview interaction and should be implemented in one
Red-Green-Refactor cycle.

## Inputs

- `docs/specs/ui/requirements/task-layout-profiles.md`, especially the desktop session-focus
  behavior, persistence guarantee, and reconciliation scenario.
- `docs/plans/dockview-session-focus-preservation/plan.md`.
- Dockview 4.13.1 `DockviewGroupPanelModel.openPanel`, `removePanel`, and
  `doSetActivePanel` behavior.
- Existing focus regressions in
  `apps/web/components/task/dockview-session-tabs.test.ts`,
  `apps/web/e2e/tests/layout/plan-panel-indicator.spec.ts`, and
  `apps/web/e2e/tests/layout/preview-tab-session-switch.spec.ts`.

## Risks

- A final-state-only assertion can miss transient Plan activation and its
  plan-seen side effect; record the activation sequence in the unit fake.
- The saved E2E precondition must prove Files owns global focus while Chat is
  selected in the center group or it will exercise the already-covered Chat
  global-focus branch.
- A captured global panel may disappear during cross-task cleanup; restore it
  only when it remains live.

## Output Contract

Report the RED assertion and activation sequence, the minimum production
change, exact files changed, unit/typecheck/E2E results, task and plan status
updates, and any remaining Dockview focus ambiguity.

## Results

- RED unit: the center selection and global focus both became `plan`, and the
  activation sequence recorded Plan.
- RED E2E: the production-build screenshot showed Plan selected, Agent
  inactive, Files unfocused, and the unseen Plan indicator consumed.
- GREEN implementation: the session replaces Chat in Chat's live group (at
  index 0 when Chat was selected), inherits its group-local selection before
  removal, and then yields global focus back to the captured live panel.
- Unit: 44 tests passed across `dockview-session-tabs.test.ts` and
  `dockview-session-tab-activation.test.ts`.
- Typecheck: `pnpm run typecheck` passed.
- E2E: the focused Chromium production-build scenario passed, including Agent
  selection, Files global focus, preserved Plan indicator, and exactly one
  session panel.
- Mobile/tablet: no rendered or responsive path changed; these viewports do not
  mount `DockviewDesktopLayout` or execute the repaired reconciliation hook.
- CI remediation: sessions anchored before reconciliation no longer re-activate
  over a saved preview/file selection, while custom layouts always replace Chat
  in Chat's actual group rather than creating a default center split.
- Regression checks: targeted unit suite (37 tests), typecheck, and focused
  Chromium scenarios for task-switch preview focus, refresh preview focus,
  returning-Agent focus, and the Plan/Files indicator all passed.
