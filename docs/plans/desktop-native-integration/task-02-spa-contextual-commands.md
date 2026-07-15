---
id: "02-spa-contextual-commands"
title: "SPA contextual command resolver"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 02: SPA Contextual Command Resolver

## Acceptance

- A single bridge host mounted by `AppShell` receives typed desktop menu events.
- Close Context dismisses exactly one topmost dismissible overlay, otherwise closes the active
  file/diff/commit/preview document through its existing dirty-state guard, otherwise no-ops.
- Alert dialogs and task/session/chat/terminal/structural panels are never implicitly closed.
- Settings navigates to the existing `/settings/general` route. New Task invokes the existing
  task-create flow when an active workspace allows it. No duplicate task-create UI is introduced.
- Non-Tauri browser and mobile behavior is unchanged.

## Files Likely Touched

- `apps/web/src/app-shell.tsx`
- New `apps/web/lib/desktop/` bridge and contextual-close modules
- `apps/web/components/global-commands.tsx`
- `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx` or the shared task-create action
- `apps/web/lib/state/dockview-store.ts`
- Existing document tab close helpers under `apps/web/components/task/`
- Focused `*.test.ts(x)` files and desktop-command E2E coverage

## Verification

```bash
cd apps/web && rtk pnpm run typecheck
cd apps/web && rtk pnpm test desktop
cd apps && rtk pnpm --filter @kandev/web lint
```

Tests must cover overlay priority, one-layer-only dismissal, alert exclusion, dirty document close,
eligible document types, structural-panel no-op, Settings navigation, New Task availability, and
absence of listeners/actions outside desktop mode.

## Output Contract

Implement the plan's frozen `v1` adapter contract against a test double before native integration,
update this task to `done`, and check its plan item only after verification passes.
