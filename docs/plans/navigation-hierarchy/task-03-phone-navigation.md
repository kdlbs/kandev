---
id: "03-phone-navigation"
title: "Make phone navigation and issue access discoverable"
status: done
wave: 3
depends_on:
  - "02-task-panel"
plan: "plan.md"
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-003
acceptance_criteria:
  - AC-UI-NAV-HIERARCHY-001.3
  - AC-UI-NAV-HIERARCHY-001.6
  - AC-UI-NAV-HIERARCHY-003.1
  - AC-UI-NAV-HIERARCHY-003.2
  - AC-UI-NAV-HIERARCHY-003.3
  - AC-UI-NAV-HIERARCHY-003.4
  - AC-UI-NAV-HIERARCHY-003.5
  - AC-UI-NAV-HIERARCHY-003.6
system_design:
  - ../../specs/ui/system-design/navigation-hierarchy.md
---

# Task 03: Phone navigation and issue access

## Summary

Place phone tool navigation above the contextual task list and expose a labelled
primary New Task. Prove the complete route to GitHub issues, including saved layouts.

## In scope

- TDD for default and saved phone composition, single launch, hidden-action plus
  fallback, focus handoff, and dialog lifetime.
- Keep workspace selection fixed above the one content scroller. Put tools before
  `MobileTaskNavigationProvider`'s outlet without replacing its controller.
- Render the primary New Task through the existing launch subscription; condition
  only the embedded Tasks plus. Preserve Office and task-title-picker entry points.
- Retain plugin toolbar deduplication, custom shortcuts, hidden Home quick actions,
  availability, setup, workspace isolation, and loading/retry behavior.
- Reconcile the documented old phone order in unified-mobile-navigation and
  sidebar-customization requirements/designs. Link this successor from the three
  earlier companion plans; keep their completed results as historical evidence.
- Update web scoped guidance and the how-to in `docs/public/use-kandev.md`.
  README/screenshot catalog already inspected: no label-specific rewrite needed
  unless actual implementation makes one of their existing descriptions false.

## Out of scope

New provider queries/routes, a second issue browser, new phone global tabs,
provider settings changes, desktop preference rewrites, and marketing screenshots.

## Acceptance

1. Default/saved phone menus satisfy 003.1-003.4, including hidden New Task fallback
   and an actual Issues list reached from both Home and a task workbench.
2. Tests prove contained scrolling, touch hitboxes, focus return, long translated
   labels, loading/error isolation, workspace switching, and an unsaved creation
   draft surviving 767px -> 768px -> 767px without duplicate dialog hosts.
3. Public/scoped documentation and successor specifications describe the delivered
   order; targeted mobile/desktop regressions and localization checks pass.

## ASCII UI preview

UI-02/UI-03 excerpt. Full [preview](plan.md#ascii-ui-preview).

```text
Menu                           x
Workspace v                       fixed
--------------------------------
[ + New Task                   ]  primary
Home
Quick Chat | Quick terminal
Automations                    >
Canvases                       >
Integrations                   v
  GitHub                          -> existing Issues selector
  Integration settings            visible even without providers
[eligible plugins/custom groups]
--------------------------------
TASKS v [All tasks v] [filter *]
  [compact tasks and actions]
Utilities / Settings
[safe area]
```

Only the body scrolls. Initial Integrations remains collapsed; expanded shown.
Tasks stays initially expanded and retains state while mounted. If New Task is
hidden, keep the Tasks plus. The task-title picker remains separate. Maps to
003.1-003.6; actual account identity is conditional on authentication.

## Verification

From repository root:

```bash
(cd apps/web && pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx components/integrations/mobile-integrations-section.test.tsx components/task/mobile/session-task-switcher-sheet.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-navigation-hierarchy.spec.ts tests/layout/mobile-menu-hierarchy.spec.ts tests/layout/mobile-unified-navigation.spec.ts tests/layout/mobile-navigation-tasks.spec.ts tests/settings/mobile-sidebar-customization.spec.ts tests/github/mobile-github-sidebar.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/navigation-hierarchy.spec.ts tests/settings/sidebar-customization.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/navigation components/integrations/integrations-menu.tsx components/task/mobile/task-picker-surface.tsx components/task/mobile/session-task-switcher-sheet.tsx --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

New mobile scenarios cover 360/393/767px phones, a 767px fine-pointer viewport,
the 768px boundary, both themes, Portuguese labels, long task lists, a configured
and unconfigured workspace, and task-load errors with working tool navigation.
Use `.tap()` on mobile, actual keyboard dismissal, `boundingBox` and hit testing.
Use `MobileGitHubPage` for Issues selection and seed mock issues through `ApiClient`.
Search mobile specs for replaced task-create controls and update only obsolete
placement assertions; preserve every retained capability scenario and include
any additional affected spec in this verification block before marking done.

## Files likely touched

- `apps/web/components/navigation/app-nav-sheet.tsx`, `app-nav-sections.tsx`,
  `app-nav-surface.tsx`, `mobile-sidebar-layout-navigation.tsx`,
  `mobile-task-navigation-provider.tsx` and focused tests.
- `apps/web/components/task/mobile/task-picker-surface.tsx`,
  `session-task-switcher-sheet.tsx` and tests.
- `apps/web/components/integrations/integrations-menu.tsx` and its mobile tests.
- New `apps/web/e2e/tests/layout/mobile-navigation-hierarchy.spec.ts` and existing
  mobile menu/saved-layout tests named above.
- Locale files as required, `apps/web/AGENTS.md`, `docs/public/use-kandev.md`.
- Existing unified-mobile-navigation/sidebar-customization specification pairs
  and their historical companion plans' successor notes.

## Dependencies

Task 02. Task 01 supplies action/disclosure styling and a tested creation host.

## Risks

The prior saved-navigation repair deliberately put Tasks first. This task changes
that product choice while retaining saved visibility and resource availability.
Avoid portaling a second scroller, unmounting dialog drafts, or duplicating the
New Task subscription. Do not inspect or alter live provider credentials.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/navigation-hierarchy.md), REQ-UI-NAV-HIERARCHY-003.
- [Design](../../specs/ui/system-design/navigation-hierarchy.md), Phone composition/Handoff and recovery.
- Existing `AppNavSurface`, task-navigation provider, and `MobileGitHubPage`.
- [Previous saved-phone package](../mobile-saved-navigation/plan.md) and
  [unified phone package](../unified-mobile-navigation/plan.md).

## Results

Implemented the shared primary action, fixed workspace picker, tools-before-Tasks ordering, optional canvas disclosure, and saved-layout visibility fallback. Existing task-title picker, provider issue navigation, workspace isolation, quick actions, plugin toolbar, and persistent task-dialog host are preserved.

Validation passed:

- Task-defined phone component tests, saved-layout projection tests, and the creation-host/provider tests.
- Existing menu hierarchy, unified navigation, embedded tasks, saved customization, and GitHub phone browser suites.
- New Home/workbench to Issues path with 21 tasks at 360px/393px/767px; fixed workspace and single-scroller containment.
- Creation draft preservation across 767px → 768px → 767px.
- Hidden primary-action fallback with unchanged saved layout, plus the uncustomized canvas disclosure using a fresh workspace.
- Error recovery, Portuguese labels, dark/light controls, and the narrow mouse/tablet touch boundary checks.

Targeted checks use the managed production-build runner with one worker. Final browser build includes the full phone-range task-action and group-header fixes.


## PR review and CI remediation (2026-09-30)

Phone Canvases disclosure uses a unique controlled-content ID. Browser coverage
verifies that relationship and collapsed/expanded visibility. The hidden-primary
action test references the creation-fallback and saved-layout criteria. Existing
GitHub task-view coverage opens the visible primary New Task action; localized
trailing-slot tests use the shared 768px phone boundary. The recovery test
measures retry/details controls before releasing the injected history failure,
preserving a deterministic opportunity to interact. Final browser validation
and CI evidence are recorded in the plan and linked PR.
