---
id: "01-atomic-session-handoff"
title: "Make shared-environment session handoff atomic"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 01: Make Shared-Environment Session Handoff Atomic

## Acceptance

- Switching between tasks in the same environment adds the incoming task's session panel to the outgoing session panel's group and tab index before closing the outgoing panel.
- Closing the outgoing task's final session panel cannot destroy the center group or change the Dockview root orientation.
- The existing active-panel policy is preserved when Files, Changes, or another non-session panel is active.
- Stale session panels are removed, the incoming session panel is unique, and existing cross-environment restoration behavior remains unchanged.
- Reloading after the task switch restores the same healthy horizontal-root layout and right-column vertical split.

## TDD Sequence

1. RED: add a session-tabs unit regression whose fake Dockview destroys a group when its final panel closes; prove the current close-before-add ordering loses the group.
2. RED: add a desktop Playwright regression that switches between two tasks sharing an environment and asserts the live and reloaded Dockview tree semantics.
3. GREEN: anchor the incoming session panel in the first stale session panel's live group and tab index before stale cleanup.
4. REFACTOR: remove any duplication that can be shared safely with cross-environment stale-session replacement, while preserving focused tests.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/dockview-session-tabs.test.ts
cd apps && pnpm --filter @kandev/web test -- components/task/dockview-session-tabs.test.ts lib/state/dockview-env-switch-action.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/layout/saved-layout-session-isolation.spec.ts
```

## Files likely touched

- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-session-tabs.test.ts`
- `apps/web/e2e/tests/layout/saved-layout-session-isolation.spec.ts`
- `apps/web/e2e/pages/session-page.ts` if the tree assertion belongs in the shared page object

## Dependencies

None.

## Inputs

- `docs/specs/ui/requirements/task-layout-profiles.md`, especially `Persistence guarantees` and the shared-environment task-switch scenario.
- The atomic stale-session replacement in `apps/web/lib/state/dockview-env-switch.ts`.
- The existing same-environment early return in `apps/web/lib/state/dockview-store.ts`.

## Output contract

Report the exact failing regression, the panel event order before and after the repair, files changed, unit/typecheck/E2E command results, and any remaining ambiguity for layouts contaminated with stale sessions in multiple groups.

## Result

- RED proved that the prior handoff emitted only `close:session:outgoing`, removed `group-center`, and left no incoming session panel.
- GREEN emits `add:session:incoming` before the outgoing close, preserves `group-center`, and retains the right-side Files or Changes above Terminal split.
- Focused unit tests, TypeScript typecheck, and the production-build Chromium regression passed on 2026-07-27.
