---
id: "03-build-responsive-task-row-settings"
title: "Build responsive task-row settings"
status: complete
wave: 2
depends_on: ["01-persist-task-row-presentation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-task-row-presentation.md"
---

# Task 03: Build Responsive Task-Row Settings

## Acceptance

- The view editor adds one collapsed **Task row** section with an accurate summary, details toggle,
  field visibility and ordering, and right-side choice.
- Section expansion is transient. Presentation changes use the existing draft and save controls.
- Desktop uses the anchored popover. Phone and tablet use an inset, safe-area-aware bottom drawer
  with one scroll body and 44 pixel controls.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/task/sidebar-filter/sidebar-filter-popover.test.tsx components/task/sidebar-filter/task-row-settings.test.tsx components/task/sidebar-filter/use-sidebar-view-popover.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:zh-hant
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
```

## Files Likely Touched

- `apps/web/components/task/sidebar-filter/sidebar-filter-popover.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-filter-bar.tsx`
- `apps/web/components/task/sidebar-filter/task-row-settings.tsx`
- `apps/web/components/task/sidebar-filter/sidebar-view-editor.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/hooks/use-responsive-breakpoint.ts`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/zh-hk/task.json`
- `apps/web/src/locales/zh-tw/task.json`
- Focused tests beside the editor components

## Dependencies

Task 01.

## Parallelism

`parallel-safe` with Task 02 after Task 01. This task owns the settings surface and locale files.
Task 02 owns the task-row renderer.

## Inputs

- The editor, responsive, and accessibility rules in the spec.
- The existing `ViewHeaderRow`, draft update actions, and mobile task-switcher sheet.
- Existing `@dnd-kit/core`, `@dnd-kit/sortable`, and `@dnd-kit/utilities` dependencies.
- The repository mobile UI language and existing responsive breakpoint hook.

## TDD Sequence

1. Add a test that the section starts collapsed and expansion alone does not call `updateDraft`.
2. Add tests for each compact summary, visibility patch, trailing patch, pointer reorder, and keyboard
   reorder. Record the expected failures.
3. Add responsive tests that select a popover for desktop and a drawer for phone and tablet.
4. Extract shared editor content and add the collapsed task-row editor.
5. Add the responsive drawer with one scroll owner, safe-area padding, close behavior, focus return,
   and 44 pixel controls.
6. Add localized copy in five catalogs and generate the Traditional Chinese variants.
7. Run focused tests, typecheck, the locale audit, and the i18n ratchet. Record exact results.

## Risks

- Reusing a popover inside the mobile sheet can create nested focus traps. The breakpoint must select
  one top-level editor surface.
- A drag handle can start task or saved-view reordering if events escape the editor context.
- Drawer content can gain two vertical scroll owners if the current popover overflow class remains
  on shared content.
- Summary plurals require locale plural keys and a numeric `count` value.

## Output Contract

Report RED failures, desktop and mobile surface behavior, accessibility details, locale files,
files changed, exact test and i18n results, blockers, risks, and synchronized task and plan status.

## Results

RED editor tests covered collapsed state, draft patches, summaries, ordering, and desktop/mobile
surface selection. The shared editor now uses an anchored desktop popover and an inset mobile/tablet
drawer with one scroll owner, safe-area padding, focus restoration, sortable fields, keyboard and
touch sensors, and 44 pixel controls. Settings coverage passed with 12 tests across 4 files;
typecheck, `i18n:check`, and `i18n:ratchet` passed. The repository Traditional Chinese wrapper still
reports its pre-existing `agents` residual warning, so the scoped task conversion command was used
and completed with zero task residual warnings.
