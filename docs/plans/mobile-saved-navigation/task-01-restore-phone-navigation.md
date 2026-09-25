---
id: "01-restore-phone-navigation"
title: "Restore saved phone navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-MENU-007
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
acceptance_criteria:
  - AC-UI-MOBILE-MENU-007.2
  - AC-UI-MOBILE-MENU-007.3
  - AC-UI-MOBILE-MENU-007.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.5
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 01: Restore saved phone navigation

## Summary and scope

Correct the saved phone menu's placement of Tasks and built-in resource sections.
Preserve custom groups, saved visibility, plugin ownership, desktop order, and
domain availability. No backend or persistence changes.

## Acceptance

- First sidebar toggle save and reload preserve task-before-tools ordering.
- Integrations and Automations retain labelled disclosures and empty setup;
  Canvases and custom groups remain usable without anonymous built-in icon strips.
- Production-build phone and desktop tests pass; isolated seeded screenshots
  demonstrate populated Tasks and expanded integrations in dark and light themes.

## ASCII UI preview

UI-01: phone menu, excerpt from the [full preview](plan.md#ascii-ui-preview).

```text
Menu                         x
Workspace
Home
Quick Chat | Quick terminal
Tasks v                      +
  [task rows]
Automations >
Canvases >
Integrations v
  GitHub
  GitLab
  Integration settings
Utilities
```

Fixed heading; one scroller; 44px touch controls. UI-02 desktop retains saved
nodes above Tasks. Covers the acceptance criteria in frontmatter.

## Files likely touched

- `apps/web/components/navigation/app-nav-sections.tsx`
- `apps/web/components/navigation/mobile-sidebar-layout-navigation.tsx`
- `apps/web/components/navigation/mobile-canvases-section.tsx`
- `apps/web/components/integrations/integrations-menu.tsx`
- `apps/web/components/app-sidebar/shortcut-section.tsx`
- `apps/web/e2e/tests/settings/mobile-sidebar-customization.spec.ts`
- `apps/web/AGENTS.md` and the owning specifications/public navigation guide.

## Verification

```bash
make build-web
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-sidebar-customization.spec.ts tests/layout/mobile-menu-hierarchy.spec.ts tests/integrations/mobile-integrations-nav.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/settings/sidebar-customization.spec.ts -- --retries=0)
(cd apps/web && pnpm exec vitest run components/integrations/integrations-menu.test.ts components/navigation/app-nav-sheet.test.tsx components/navigation/mobile-automations-section.test.tsx components/navigation/mobile-canvases-section.test.tsx components/app-sidebar/shortcut-section.test.tsx components/kanban/mobile-menu-utility-actions.test.tsx components/navigation/destination-rows.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run focused lint/unit tests for changed files and any new helper. Screenshot
inspection covers 393px, narrow phones, and 767px; confirm desktop at 768px+.

## Dependencies and parallelism

None. Sequential implementation in the primary session.

## Risks

Saved plugin nodes and independent custom groups must preserve their visibility;
task/dialog state remains owned by the existing retained controller.

## Results

- Red: saved Home visible/hidden initially placed Automations above Tasks. The
  populated first-toggle flow now covers persisted visibility, one menu scroller,
  44px targets, settings navigation, and task activation.
- Red: the redundant New Task row was visible; regular menus now keep task
  creation in the existing task heading, while Office retains its creation entry.
- Red: Canvas settings appeared twice. The legacy workspace-actions block now
  defers to saved layout rendering; hiding Canvases survives save/reload.
- Green: the managed production-build mobile command ran with the disposable
  screenshot spec in the same invocation: 17 passed (16 permanent regressions
  plus capture), retries disabled. A final capture-only run regenerated all seven
  assets after visual refinement. The desktop command passed all 3 tests.
- Green: 58 focused unit tests across the seven listed suites. Typecheck, focused
  ESLint, `i18n:check`, docs catalog validation, all spec lint, public-docs
  validation, harness validation, and `git diff --check` passed.
- Visual inspection: seeded phone task list, expanded provider/settings rows,
  dark and light themes, 360px/393px/767px phones, and the 1280px desktop sidebar.
  Runtime data and synthetic identities belonged to disposable E2E backends;
  screenshots use actual rendered UI and no DOM/CSS alterations.
- Public navigation documentation, sidebar requirements/design, and scoped
  frontend guidance updated with the phone composition contract.

### PR accessibility remediation

A regression test reproduced the Canvas disclosure pointing to a missing panel
while collapsed. The panel now remains mounted with `hidden`, preserving its
accessible relationship throughout open/close transitions. The 58 focused unit
tests, 16 mobile regressions, typecheck, and affected ESLint pass after the fix.
The shared automation identifier remains paired with its owning disclosure, and
canvas URLs remain guaranteed by `buildShortcutCatalog` and `canvasHref`; no
placeholder navigation is introduced.
