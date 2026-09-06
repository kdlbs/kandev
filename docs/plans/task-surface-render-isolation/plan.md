---
spec: docs/specs/ui/requirements/file-tree-chat-context.md
created: 2026-08-31
status: in_progress
---

# Fix Plan: Task Surface Render Isolation

## Outcome

Keep file-tree, sidebar, pull-request, and plugin behavior unchanged. Reduce React work by isolating
unchanged rows, virtualizing large file trees, and stabilizing derived props.

## Confirmed evidence

The enhanced Chrome trace covers 5.74 seconds. Main-thread `RunTask` work used 3.56 seconds, or
approximately 62 percent of the window. Three application stalls lasted 978 ms, 841 ms, and 837 ms.

The trace showed three broad React render waves. Each wave rendered approximately 602 file-tree
rows and 48 sidebar task rows. It also recreated the context-menu provider stack for each mounted
file row.

Changed-prop diagnostics identified these unstable values:

- File-tree rows received new `onToggleExpand` and `onDrop` functions.
- Sidebar rows received a new `onBulkMove` function.
- Plugin actions received an equal but new `sessionIds` array.
- Pull-request indicators received equal but new hydration and empty-list values.

Layout, paint, and minor garbage collection used less than 100 ms combined. The trace also included
React DevTools and CPU-profiler overhead. Therefore, final measurements use a production build
without extensions.

## Technical direction

### Stable row inputs

- Remove complete aggregate objects from file-tree callback dependencies.
- Memoize `TreeNodeItem` around all inputs that change its output or actions.
- Stabilize sidebar selection callbacks before they cross the `TaskSwitcher` boundary.
- Add render-count tests that use controlled state changes instead of timing assertions.

### Virtual file tree

- Use the existing `@tanstack/react-virtual` dependency.
- Keep `visibleRows` as the ordered flat tree model.
- Keep the current file-tree viewport as the only scroll owner.
- Measure rows because edit controls and mobile actions can change row height.
- Update reveal and restore flows so offscreen target rows mount before DOM access.

### Stable contribution props

- Keep plugin `sessionIds` stable while their ordered values remain equal.
- Memoize the pull-request hydration aggregate around its semantic inputs.
- Reuse a module-level empty pull-request list.

## Files

### File-tree render isolation

- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/file-browser-hooks.ts`
- `apps/web/components/task/file-browser-parts.tsx`
- `apps/web/components/task/file-browser-render-identity.test.tsx`
- Existing focused file-browser tests

### File-tree virtualization

- `apps/web/components/task/file-browser-parts.tsx`
- `apps/web/e2e/tests/task/large-file-tree-virtualization-helpers.ts`
- `apps/web/e2e/tests/task/large-file-tree-virtualization.spec.ts`
- `apps/web/e2e/tests/task/mobile-large-file-tree-virtualization.spec.ts`
- Existing desktop and mobile file-tree tests

### Sidebar render isolation

- `apps/web/components/task/task-session-sidebar-selection.tsx`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-session-sidebar-selection.test.ts`
- `apps/web/components/task/task-switcher.test.tsx`

### Contribution prop stability

- `apps/web/components/task/task-top-bar-plugin-actions.tsx`
- `apps/web/components/task/task-top-bar-plugin-actions.test.tsx`
- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.ts`
- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx`
- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon.render.test.tsx`

### Durable artifacts

- `docs/specs/ui/system-design/task-surface-render-isolation.md`
- `docs/plans/task-surface-render-isolation/plan.md`
- Four work orders in this plan directory

No backend, API, database, localization, dependency, or public-documentation change is required.

## Mobile design contract

The Files panel stays in its current task surface. Its existing viewport remains the only vertical
scroll owner, and the page does not gain horizontal overflow.

Each mounted file row keeps the visible mobile action and 44 CSS pixel target. Offscreen rows mount
through normal scrolling or explicit virtualizer reveal.

The sidebar row remains the primary mobile tap action. Its explicit task menu and responsive menu
surface remain unchanged.

## Test strategy

- Unit tests prove stable callback, aggregate, array, and empty-list identities.
- Component tests prove that unrelated owner updates do not rerender unchanged rows.
- Desktop E2E tests use at least 600 generated file entries.
- Mobile E2E tests prove bounded mounted rows, scrolling, file opening, and touch actions.
- Existing behavior tests cover expansion, selection, drag and drop, context actions, and sidebar
  navigation.

## Verification

Run the commands from the repository root unless a step changes directories.

```bash
cd apps
rtk pnpm --filter @kandev/web test components/task/file-browser-render-identity.test.tsx components/task/file-browser-actions.test.ts components/task/file-browser-toggle-expand.test.ts
rtk pnpm --filter @kandev/web test components/task/task-session-sidebar-selection.test.ts components/task/task-switcher.test.tsx
rtk pnpm --filter @kandev/web test components/task/task-top-bar-plugin-actions.test.tsx hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx components/github/pr-task-icon.render.test.tsx
rtk pnpm --filter @kandev/web typecheck
rtk pnpm --filter @kandev/web lint
cd ..
rtk make build-web
cd apps/web
rtk pnpm e2e:run --no-build --project chromium tests/task/large-file-tree-virtualization.spec.ts
rtk pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-large-file-tree-virtualization.spec.ts tests/task/mobile-file-tree-chat-context.spec.ts
rtk pnpm run lint:e2e-sleeps -- e2e/tests/task/large-file-tree-virtualization.spec.ts e2e/tests/task/mobile-large-file-tree-virtualization.spec.ts
cd ../..
rtk git diff --check
```

## Implementation waves

1. [Stabilize file-tree row inputs](task-01-stabilize-file-tree-rows.md)
2. [Virtualize the large file tree](task-02-virtualize-file-tree.md)
3. [Stabilize sidebar row inputs](task-03-stabilize-sidebar-rows.md)
4. [Stabilize contribution props](task-04-stabilize-contribution-props.md)

The waves are sequential because each task uses the same web validation environment. Tasks 3 and
4 do not depend on file-tree code, but this plan does not authorize subagents.

## Performance evaluation

After Wave 4, capture the same interaction from a warmed production build. Disable browser
extensions and do not include profiler startup in the selected window.

Compare these values with the original trace:

- Total main-thread `RunTask` time and long-task count.
- React renders for `TreeNodeItem`, `TaskRow`, plugin actions, and pull-request icons.
- Mounted file rows and menu providers before and after scrolling.
- Layout, paint, and garbage-collection time as regression guards.

The trace comparison is diagnostic evidence. It is not a CI timing threshold.

## Risks and exclusions

- Incorrect memo equality can show stale selection, expansion, drag, or active-file state.
- Virtual rows have variable height during rename and on touch layouts.
- Reveal logic must mount an offscreen row before it requests a DOM element.
- Overscan must keep scrolling smooth without recreating hundreds of menu providers.
- Store selectors must not return a new array on every snapshot read.
- This plan does not consolidate all file-row menus into one shared menu.
- This plan does not change WebSocket update frequency or backend state shapes.

No ADR is required. The design uses the existing virtual-list dependency and UI ownership rules.
