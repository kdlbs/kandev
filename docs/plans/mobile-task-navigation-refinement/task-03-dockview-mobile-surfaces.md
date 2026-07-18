---
id: "03-dockview-mobile-surfaces"
title: "Dockview mobile surfaces"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 03: Dockview mobile surfaces

## Acceptance

- Task switcher is an inset bottom card with internal scrolling and full existing task/filter/workspace/action parity.
- Active-session pill shows active agent icon beside label and updates when session changes.
- Nested menus/dialogs remain selectable and return focus safely.

## Files likely touched

- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.test.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.test.tsx`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- `apps/web/e2e/tests/session/mobile-handoff.spec.ts`

## Inputs

- Spec bottom-card and active-agent scenarios.
- Existing `MobilePickerSheet`, `AgentLogo`, and `buildSessionRows` patterns.

## Verification

```bash
cd apps && NODENV_VERSION=24.12.0 pnpm --filter @kandev/web test -- --run components/task/mobile/session-task-switcher-sheet.test.tsx components/task/mobile/mobile-sessions-section.test.tsx
NODENV_VERSION=24.12.0 make build-web
cd apps/web && NODENV_VERSION=24.12.0 pnpm e2e:run tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-sidebar-views.spec.ts tests/session/mobile-handoff.spec.ts
```

## Output contract

Report behavior, tests, files, blockers/risks; mark task and plan item done.
