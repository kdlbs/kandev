---
id: "02-topbar-sidebar"
title: "Topbar and sidebar actions"
status: done
wave: 2
depends_on:
  - "01-action-api-composers"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-002
  - REQ-PLUGINS-ACTION-UX-003
acceptance_criteria:
  - AC-PLUGINS-ACTION-UX-001.1
  - AC-PLUGINS-ACTION-UX-001.2
  - AC-PLUGINS-ACTION-UX-001.3
  - AC-PLUGINS-ACTION-UX-002.1
  - AC-PLUGINS-ACTION-UX-002.2
  - AC-PLUGINS-ACTION-UX-002.3
  - AC-PLUGINS-ACTION-UX-002.4
  - AC-PLUGINS-ACTION-UX-002.5
  - AC-PLUGINS-ACTION-UX-002.7
  - AC-PLUGINS-ACTION-UX-003.3
  - AC-PLUGINS-ACTION-UX-003.4
  - AC-PLUGINS-ACTION-UX-003.5
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
---

# Task 02: Topbar and sidebar actions

## Summary

Apply the shared action renderer to both topbars and sidebar workspace actions.
Retain the existing phone Plugins section and task/workspace selection.

## In scope

- Supply explicit surface context in MainTopBarPluginActions, TaskTopBarPluginActions, and AppSidebarWorkspaceActions.
- Move the matching ordinary native cluster controls to shared surface styles.
- Keep legacy selectors and wrappers compatible while excluding the new action marker from conflicting rules.
- Exercise two actions per registration, mixed registrations, long labels, null task contributions, and animated bounded glyphs.
- Extend fixture and browser coverage for desktop, phone, breakpoint boundaries, and wide coarse-pointer controls.

## Out of scope

Status items, sidebar-footer navigation changes, new menu destinations, and official plugin migrations.

## Acceptance

1. New plugin controls and same-role native controls share geometry and state styles in both topbars and sidebar actions.
2. Phone Plugins entries preserve task replacement, workspace fallback, and independent sidebar actions without duplicate controls.
3. Legacy controls retain behavior and styles. New controls meet touch and containment checks.

## ASCII UI preview

UI-01 / UI-02: Topbar and sidebar. See the [combined previews](plan.md#ascii-ui-preview).
The frontmatter criteria apply to this excerpt. Labels and glyphs are illustrative.

```text
Desktop: [native tool] [plugin] [plugin 63%]
Sidebar: [New task] [Quick terminal] [Quick chat] [plugin]
Phone app menu:
  Plugins
  [task plugin] [workspace-only plugin] [sidebar plugin]
```

Keep current wrapping, selection, safe-area handling, and one menu scroll owner. Null task contributions retain workspace fallback.

## Verification

Run from the repository root. Use TDD for new behavior and record the initial
behavioral failure. New test paths in this order are files to create.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/plugins/plugin-action.test.tsx components/plugins/plugin-slot.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/task/task-top-bar.test.tsx components/app-sidebar/app-sidebar-primary-nav.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/plugin-action-ux.spec.ts --grep chrome)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-plugin-action-ux.spec.ts --grep chrome)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-plugin-topbar.spec.ts e2e/tests/plugins/mobile-plugin-sidebar-workspace-actions.spec.ts)
```

Run any other test file changed by this work order with the same targeted runner.
For rendered changes, inspect the focused desktop and phone screenshots against
the assigned previews. Do not infer CSS geometry from unit tests.

## Files likely touched

- `apps/web/components/kanban/{main-top-bar-plugin-actions.tsx,kanban-header.tsx}`
- `apps/web/components/task/{task-top-bar-plugin-actions.tsx,task-top-bar.tsx,task-top-bar.test.tsx}`
- `apps/web/components/app-sidebar/{app-sidebar-workspace-actions.tsx,app-sidebar-new-task-item.tsx}`
- `apps/web/components/plugins/{mobile-plugin-nav-section.tsx,mobile-plugin-nav-section.test.tsx,plugin-action-surface.tsx}`
- Shared surface modules, plugin-action tests, and fixture bundle from Task 01
- Both new action UX E2E files from Task 01

## Dependencies

01-action-api-composers.

## Risks

Do not treat a context provider as visible slot content. Preserve PluginSlotPresence null detection and registration keys.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/plugin-action-ux.md)
- [System design](../../specs/plugins/system-design/plugin-action-ux.md)
- [Plan and source audit](plan.md)
- Existing plugin-slot, SDK consumer, and packaged plugin E2E patterns.

## Results

Complete. Both topbars and sidebar workspace actions now receive explicit
surface context without changing slot keys or plugin context. New plugin Actions
retain 28px fine-pointer topbar and 24px fine-pointer sidebar geometry, with
44px touch sizing; the desktop sidebar quick actions, task right-panel toggle,
debug action, and tablet quick actions share the native surface renderer.
Legacy buttons remain supported, and the new action marker is excluded from
legacy sizing selectors.

Validation passed:

- Focused web tests: 10 files, 119 tests, including slot context, state, mobile
  navigation selection, task topbar, sidebar, and native controls.
- `(cd apps/web && pnpm run typecheck)`
- Managed chromium chrome E2E: topbar and sidebar geometry, border, radius,
  padding, 16px/14px glyphs, and group gaps passed.
- Managed mobile-chrome action E2E: phone Plugins actions and the 44px target
  contract passed at 390px, 800px coarse-pointer tablet, and 1200px coarse-pointer
  sidebar layout.
- Existing mobile plugin topbar/sidebar E2Es: 3 tests passed.
- Desktop and phone chrome screenshots were inspected against UI-01/UI-02.

The touch Actions keep their accessible names without opening an overlay
tooltip on tap, so the phone Plugins sheet stays readable after activation.

Original PR-head CI found that the Quick Chat focus E2E assumed keyboard focus
always uses an outline. The migrated sidebar action uses a visible focus ring,
so the test now accepts either a changed outline or ring shadow while still
checking silent focus. Its focused managed Chromium check passed 1/1.

Review remediation (2026-09-25): `ActionGroup` now bounds and wraps its
topbar children for mobile presentation and coarse-pointer topbars while
preserving the fine-pointer desktop gap. The phone fixture includes four
actions, including two long values; the E2E test checks every control stays
inside the actual Plugins section and can be tapped. It also checks the real
SVG geometry at 16px for topbar and 14px for sidebar Actions, including a
component-rendered glyph. The updated mobile action suite passed 5/5 tests and
the updated Chromium action suite passed 4/4 tests.

PR review remediation (2026-09-25): `MainTopBarPluginActions` now memoizes its
surface context and `PluginSlot` subtree. An unchanged topbar rerender therefore
does not re-render plugin components. The focused topbar regression passed in
the combined renderer suite (3 files, 19 tests):
`(cd apps/web && pnpm exec vitest run components/plugins/plugin-action.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/actions/surface-action-styles.test.ts)`.
Also passed: `(cd apps/web && pnpm run typecheck)` and focused ESLint on the
changed renderer, topbar, style, test, and action UX E2E files.
