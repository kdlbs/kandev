---
id: "03-layout-settings-editor"
title: "Layout settings editor"
status: done
wave: 2
depends_on: ["01-saved-layout-validation", "02-layout-profile-domain"]
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 03: Layout Settings Editor

## Acceptance

- `Settings > General > Layouts` manages built-in templates and custom profiles with explicit save/cancel and error states.
- The isolated preview supports reusable panel add/remove, tab reorder, active-tab choice, and split create/move/resize without mounting runtime task panels.
- Every operation is reachable by pointer, keyboard, and touch; the mobile page has no horizontal page scroll.

## Files likely touched

- `apps/web/app/settings/general/layouts/page.tsx`
- `apps/web/components/settings/layouts/layout-settings.tsx`
- `apps/web/components/settings/layouts/layout-editor.tsx`
- `apps/web/components/settings/layouts/layout-editor-toolbar.tsx`
- `apps/web/components/settings/layouts/layout-profile-list.tsx`
- `apps/web/components/settings/general-nav.ts`
- `apps/web/components/app-sidebar/sections/settings/general-group.tsx`
- `apps/web/src/settings-routes.tsx`
- `apps/web/src/settings-routes.test.ts`

## Dependencies

- Task 01 supplies backend invariants.
- Task 02 supplies profile and validation helpers.

## Inputs

- Spec `What`, `Failure modes`, and settings scenarios.
- Existing settings route hydration, `SettingsPageTemplate`, Dockview theme, layout serializer/applier, and `@dnd-kit`/touch patterns.

## Verification

```bash
pnpm --filter @kandev/web test -- --run lib/layout/layout-profiles.test.ts src/settings-routes.test.ts
pnpm --filter @kandev/web typecheck
```

Run from `apps`.

## Output contract

Report desktop/mobile behavior, tests run, files changed, blockers, and risks; mark this task and its plan item done.
