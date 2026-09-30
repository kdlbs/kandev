---
id: "01-shared-navigation"
title: "Unify navigation and preserve entry points"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-MOBILE-MENU-001
  - REQ-UI-MOBILE-MENU-002
acceptance_criteria:
  - AC-UI-MOBILE-MENU-001.1
  - AC-UI-MOBILE-MENU-001.2
  - AC-UI-MOBILE-MENU-001.3
  - AC-UI-MOBILE-MENU-001.4
  - AC-UI-MOBILE-MENU-001.5
  - AC-UI-MOBILE-MENU-001.6
  - AC-UI-MOBILE-MENU-002.1
  - AC-UI-MOBILE-MENU-002.2
  - AC-UI-MOBILE-MENU-002.3
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 01: Unify navigation and preserve entry points

## Summary

Deliver one functional phone navigation contract across current shells.
Integrate the shared app menu, task-title picker, and listing-options entry
together so no capability loses its entry point between changes.

## In scope

- Shared phone Drawer with manifest-based navigation, workspace picker, local
  navigation sections, existing workspace actions, and hoisted dialogs.
- Task-title trigger and explicit selection/handoff callbacks to the existing
  task-switcher owner; no duplicate full task picker on the task workbench.
- Local View options and existing listing/search/saved-view behaviors.
- Localization, affected test-entry migration, rendered checks, public docs.

## Out of scope

Pinned shortcut content (Task 02), task mutation redesign, new persistence,
backend changes, desktop/tablet redesign, publication.

## Acceptance

- All covered menu/title/options criteria pass through their actual phone
  entry points while existing desktop/tablet behavior remains available.
- Existing dialogs retain drafts/focus, task history retains origin policy,
  and workspace-mode/permission gates remain enforced.
- Required checks pass; screenshots match the structural previews; all changed
  regression scenarios run and results are recorded.

## ASCII UI preview

### UI-01 / UI-02 / UI-03 / UI-05: Shared shell and retained contexts

```text
Phone listing: [Context] [Menu]    Phone task: [Back] [Task v] [Menu]
               [View options]                [Session v]

Menu -> Workspace / Destinations / Local nav / Actions / Utilities
Task v -> existing task drawer and row actions
View options -> listing modes / search / display / saved-view actions

Desktop: [Existing sidebar] | [Existing header and Dockview]
Tablet:  existing composition
```

Use the full [UI-01 through UI-05 previews](plan.md#ascii-ui-preview), except
UI-04 and the optional pins block. Maps to every criterion in this work order.
Menus have a fixed heading and one internal scroller. Phone hit targets are
44px minimum; long title truncation cannot hide the hamburger or picker.

## Verification

From the repository root. Bootstrap once in a fresh worktree. Use TDD for changed
logic and new E2E; initially run the narrow new test to observe the intended
failure. Commands below are the final checks after implementation. The managed
runner builds current assets and enforces bounded workers.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/navigation/app-nav-sheet.test.tsx components/kanban/kanban-header-mobile.test.tsx components/kanban/mobile-menu-sheet.test.tsx components/task/mobile/session-mobile-top-bar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-unified-navigation.spec.ts)
python3 - <<'PY'
from pathlib import Path
import subprocess
paths = Path('docs/plans/unified-mobile-navigation/affected-mobile-specs.txt').read_text().splitlines()
assert paths and all((Path('apps/web/e2e') / p).is_file() for p in paths)
subprocess.run(['pnpm', 'e2e:run', '--project', 'mobile-chrome', *paths], cwd='apps/web', check=True)
PY
(cd apps/web && pnpm e2e:run --project chromium tests/layout/compact-desktop-responsive.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run targeted ESLint on changed TS/TSX files, and any additional changed unit
suite beyond the four named above. Record their exact paths and commands in
Results. Do not overlap E2E runs. Render phone menu/title/options at the
configured mobile device and 767px; check wider presentation at 768px and 820px.
Inspect screenshots and focus/scroll behavior. Include task title loading,
long names, Office/Settings navigation, and task-dialog rotation.

## Implementation files

Paths relative to `apps/web/`:

- `components/navigation/app-nav-sheet.tsx`, `app-nav-trigger.tsx`,
  `app-nav-sections.tsx`, `use-task-view-navigation.tsx`, `destination-rows.tsx`.
- `components/kanban/kanban-header-mobile.tsx`, `mobile-menu-sheet.tsx`,
  `mobile-listing-menu-actions.tsx`; obsolete `mobile-listing-menu-button.tsx`
  is removed. `MobileMenuSheet` retains wider behavior and has a phone listing-only mode.
- `components/task/mobile/session-mobile-top-bar.tsx`,
  `session-mobile-layout.tsx`, `session-tablet-layout.tsx`, `responsive-task-picker.tsx`,
  `task-sheet-selection-context.tsx`, `session-task-switcher-sheet.tsx`, and
  `components/task/task-layout.tsx`.
- Relevant tests and E2E helpers/page objects; locale catalogs in
  `src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/` and generated pseudo locale.
- Repository `docs/public/sessions-and-review.md`; compatibility guidance in
  `apps/web/AGENTS.md` if its phone-navigation description changes.

## Dependencies

None. Existing GitHub Task views behavior is a compatibility input, not an
unfinished dependency.

## Risks

Menu and task controllers compete for focus; wrong callback changes browser
history; option extraction hides recovery/creation controls. Search all old
selectors before changing them and preserve the old tests' user outcomes.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/ui/requirements/unified-mobile-navigation.md),
  [design](../../specs/ui/system-design/unified-mobile-navigation.md).
- Existing `AppNavSheet`, `MobileMenuSheet`, `SessionTaskSwitcherSheet`,
  `mobile-kanban-topbar.spec.ts`, and `mobile-task-view-access.spec.ts`.
- Root/scoped AGENTS, mobile-parity, tdd, e2e, and docs-maintainer skills.

## Results

Implemented. The [plan verification results](plan.md#verification-results) record
exact commands and outcomes: 109 focused unit tests pass, and all 242 affected
mobile scenarios have passing evidence across the broad run and focused reruns. Both compact-desktop browser checks
also pass.
Typecheck, targeted ESLint, translation completeness, and i18n ratchet pass.
Phone screenshots were inspected at 393px and 767px, including menu/options
containment and pin loading/error/denial states. Task creation survives rotation;
workspace metadata refreshes preserve the workbench and cached chat scroll state.
No backend contract or preference storage was added.
