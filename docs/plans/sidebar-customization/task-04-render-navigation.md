---
id: "04-render-navigation"
title: "Render shortcut sections across navigation surfaces"
status: done
wave: 4
depends_on:
  - "03-edit-layouts"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-001.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-002.6
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-004.5
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.4
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 04: Render shortcut sections across navigation surfaces

## Summary

Apply saved layouts to desktop and phone navigation. Preserve the agreed header/list behavior, fixed task region, and every shortcut through overflow and the collapsed rail.

## In scope

- Saved projection, disclosure/action separation, plugin placement visibility, workspace-qualified collapse, overflow and rail menus.
- Phone menu integration, customization entry point, empty/unavailable states, focus and navigation behavior.
- Update user navigation documentation and screenshot references for implemented layout behavior.

## Out of scope

- Activity polling and new automation semantics.

## Acceptance

- The exact four-icon header opens each target and independently expands the same ordered labelled list.
- Hiding default sections leaves pinned shortcuts functional; protected navigation and palette behavior remain intact.
- Desktop narrow/rail and phone overflow keep every shortcut reachable without clipped controls.

## ASCII UI preview

See the [combined previews](plan.md#ascii-ui-preview). These views implement the
acceptance criteria listed above.

### UI-04: Expanded desktop sidebar, section folded and unfolded

Entry: workspace navigation after saving the layout.

```text
Default                              [collapse]
> Shortcuts                 [GH][S][C][A]
Tasks                           All tasks
  ...

v Shortcuts                 [GH][S][C][A]
    GitHub
    Slack
    My canvas
    My automation
Tasks                           All tasks
  ...
```

The label/chevron toggles expansion; sibling icons navigate directly. Tasks
retains the remaining scroll space. Header and list use the same ordering.

### UI-05: Narrow header, collapsed rail, and unavailable target

```text
> My longer group     [GH][S][...]

56px rail: [Group]
  opened menu:
    GitHub
    Slack
    My canvas
    My automation

v Shortcuts              [GH][?][C][A]
    GitHub
    Unavailable                 [disabled]
    My canvas
    My automation
```

More and the rail launcher reveal all remaining items with names. Saved
unavailable references remain removable in settings. A new empty group shows
its name; its editor offers Add shortcut. Icons never shrink to force a fit.

### UI-06: Phone navigation drawer

Entry: app menu; same layout, dedicated touch composition.

```text
Menu                              [close]
Workspace: Default
v Shortcuts
  [GH]    [S]    [C]    [A]    [...]
    GitHub
    Slack
    My canvas
    My automation
Tasks
Settings
Customize sidebar
```

The fixed menu header sits above one scrolling body. The icon strip occupies a
separate line to retain 44px hit targets. Folding hides labelled entries only.
Navigating closes the menu. Disclosure and overflow actions keep it open.

## Verification

Run from the repository root. Install dependencies once with
`(cd apps && pnpm install --frozen-lockfile)` if this worktree has no install.
Use TDD for new logic and the named E2E scenarios. Managed E2E commands rebuild
assets; run desktop and phone commands sequentially without worker overrides.

```bash
(cd apps/web && pnpm exec vitest run components/app-sidebar/shortcut-section.test.tsx components/app-sidebar/app-sidebar.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/sidebar-shortcuts.spec.ts tests/settings/sidebar-customization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-sidebar-shortcuts.spec.ts tests/settings/mobile-sidebar-customization.spec.ts)
git diff --check -- docs/public
```

## Files likely touched

- `apps/web/components/app-sidebar/app-sidebar.tsx`
- `apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx`
- `apps/web/components/app-sidebar/shortcut-section.tsx (new)`
- `apps/web/components/navigation/app-nav-sections.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/plugins/plugin-nav-items.tsx`
- `apps/web/e2e/tests/layout/*sidebar-shortcuts.spec.ts (new)`
- `docs/public/use-kandev.md`
- `docs/public/configuration.md`

## Dependencies

03-edit-layouts.

## Risks

Section lists own scrolling and may own fetch effects. Preserve task height and required data subscriptions while hiding optional navigation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md), IDs above.
- [System design](../../specs/ui/system-design/sidebar-customization.md), corresponding sections.
- [Plan](plan.md), test matrix and previews.
- Scoped `apps/web/AGENTS.md` and `apps/backend/AGENTS.md` where applicable.

## Results

Implemented saved-layout rendering for desktop, the collapsed rail, and the
phone navigation menu while preserving Tasks, required inbox entries, and
Office composition. Shortcut section headers keep direct actions separate from
disclosure, and overflow plus unavailable states remain reachable. Navigation
component tests, the untouched Office phone fallback test, workspace-picker
guard coverage, the dedicated desktop and phone customization E2E scenarios,
the existing five-test desktop sidebar regression suite, the existing five-test
mobile regression suite, and the Vite build passed.
