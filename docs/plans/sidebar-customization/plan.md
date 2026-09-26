---
created: 2026-09-19
status: implemented
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-003
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
legacy_specs: []
---

# Implementation Plan: Sidebar Customization

## Overview

Deliver personal workspace layouts, named collapsible shortcut sections, and
live automation bubbles. Preserve the agreed direct-action icon header and
matching labelled list. The five work orders are implemented sequentially in
the primary session. This task does not include commits, pushes, or PR
publication.

## Scope

### In scope

- Visibility and ordering of optional navigation entries.
- Named groups mixing GitHub, registered plugin links, canvases, and automations.
- Header actions, expanded lists, drag/drop, and explicit move controls.
- Portable per-user/per-workspace settings, defaults, reset, and conflict recovery.
- Running/idle/paused bubbles, including folded groups and overflow.
- Desktop, collapsed rail, phone editor/menu, accessibility, and localization.

### Out of scope

Arbitrary plugin slot buttons, a new plugin SDK API, direct automation execution,
custom scripts/URLs, shared team layouts, task filter changes, and reordering
Office-only sections. Tasks and required inbox entries remain fixed. Header
icons with text labels are deferred in favor of the accepted icon header.

## Technical approach

1. Extend `internal/user` models, DTOs, service CAS, store, boot/WS projection,
   and settings catalog with `sidebar_layouts_by_workspace` and a scoped patch.
2. Add typed operations and a reference catalog under `apps/web/lib/sidebar/`.
   Adapt `resolveDestinations`, owner-qualified plugin IDs, canvas eligibility,
   and existing host launchers. Keep task-view settings independent.
3. Add `/settings/sidebar`, Appearance/discovery entry points, and a
   `useSettingsSaveContributor` editor with a local draft preview.
4. Replace optional composition in `AppSidebarModeNav`/`AppSidebarPrimaryNav`
   with the saved projection. Extend `AppNavSections` for phone navigation.
   Retain `TasksSection`, required inbox entries, and settings takeover.
5. Share automation summary reads and existing `automationState` derivation;
   reuse Quick Chat bubble geometry without its unread semantics.

See the [requirements](../../specs/ui/requirements/sidebar-customization.md)
and [design](../../specs/ui/system-design/sidebar-customization.md) for contracts.
The work orders below record the implementation modules and verification
evidence for this package.

## ASCII UI preview

The disclosure/action split, icon header, matching list, fixed task region,
and dedicated phone composition are structural requirements. Spacing, icons,
and example names are illustrative. All product copy uses localization.

### UI-01: Desktop editor, draft with hidden entries

Entry: Appearance > Sidebar or Customize sidebar.

```text
Sidebar                         Workspace: Default
Changes apply to your navigation in this workspace.

LAYOUT                           DRAFT PREVIEW
:: Home                 [off]    > Shortcuts [GH][S][C][A]
:: New Task              [on]    Tasks
:: Shortcuts             [on]      ...
   Name [Shortcuts        ]
   :: [GH] GitHub          [x]
   :: [S]  Slack           [x]
   :: [C]  My canvas       [x]
   :: [A]  My automation   [x]
   [+ Add shortcut]
:: Automations          [off]
:: Canvases             [off]
:: Integrations         [off]
   Tasks                [fixed]
[+ Add shortcut section]  [Restore defaults]
                 [Discard] [Save changes]*
```

`::` is a drag handle with explicit move controls. `*` is the existing shared
save control, not a new local footer. The draft preview does not change live
navigation. Fixed Tasks and required inbox entries remain outside reorder scope.

### UI-02: Phone editor, focused section

Entry: Menu > Customize sidebar. Full-page settings navigation.

```text
< Sidebar              Default
Shortcuts
Name [Shortcuts              ]

[GH] GitHub               [...]
[S]  Slack                [...]
[C]  My canvas            [...]
[A]  My automation        [...]
[+ Add shortcut]

... menu:
  Move up
  Move down
  Move to section >
  Remove

        [Save changes]*
```

Back returns to the layout list with visibility switches. Add shortcut opens a
searchable picker; selection returns to this section draft. The page has one
scroll owner and safe-area clearance. Explicit move controls are always reachable.

### UI-03: Picker and save failures, shared content

```text
< Add shortcut
[Search shortcuts...       ]
Built-in / Plugins / Canvases / Automations
[GH] GitHub
[S]  Slack
[C]  My canvas
[A]  My automation

Empty: No matching shortcuts
Loading: Loading shortcuts...
Error: Could not load shortcuts [Retry]

Save conflict: Layout changed elsewhere.
Your draft is retained. [Load latest]
Save error: Could not save changes. [Retry]
```

Desktop uses the existing picker overlay; phone uses the focused picker step.
Loading, empty, and error are alternatives, not simultaneous messages.

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

### UI-07: Activity in folded, expanded, and overflow states

```text
> Shortcuts                 [GH][S][C][A*]

v Shortcuts                 [GH][S][C][A*]
    GitHub
    Slack
    My canvas
    My automation                  Running *

> Shortcuts                 [GH][S][...*]
Rail: [Group*]
Phone:
> Shortcuts
  [GH]    [S]    [C]    [A*]
```

`*` represents an overlaid bubble, not a literal glyph. Running is blue,
idle green, and paused muted. Unknown/loading/error has a distinct neutral
indicator and explanatory text. Overflow and rail launchers aggregate hidden
running activity. No bubble click target is added; the parent opens the shortcut.

## Tests

All test names below are implementation targets. Each work order adds failing
coverage before its production changes, then records actual results.

| Acceptance criteria (AC-UI-SIDEBAR-CUSTOMIZATION-) | Proposed evidence and test cases |
| --- | --- |
| 001.2, 001.3, 004.3 | `internal/user/service/sidebar_layout_test.go`: `TestSidebarLayoutScopedCAS`, `TestSidebarLayoutResetRevision`, `TestSidebarLayoutAccess`; handlers/store round trips |
| 004.5 | Same service test: `TestSidebarLayoutValidation`; `layout-operations.test.ts`: `validates groups and moves without duplicates` |
| 001.1, 001.4, 001.5 | `layout-projection.test.ts`: `preserves protected entries and palette`; visibility/navigation E2E |
| 002.1, 002.6, 004.4 | `shortcut-catalog.test.ts`: `resolves typed targets`, `isolates plugin owners`, `retains unavailable references`, `navigates without executing` |
| 002.4, 004.1, 004.2, 004.3 | `sidebar-layout-editor.test.tsx`: `saves and discards scoped drafts`, `keeps draft after conflict`, `moves across groups`; `sidebar-layout-editor-dnd.test.ts`: group reorder, cross-group move, duplicate rejection; editor E2E |
| 002.2, 002.3, 002.5 | `shortcut-section.test.tsx`: `separates disclosure and navigation`, `shares order`, `offers overflow and rail menu`; navigation E2E |
| 003.1, 003.2, 003.3, 003.4 | `use-shortcut-activity.test.ts`: `refreshes idle pins`, `does not claim idle before load`, `rejects stale workspace results`, `shares polling`; activity E2E |
| 005.1, 005.2, 005.3, 005.4 | Phone E2E below, `typecheck`, `i18n:check`, and `i18n:ratchet` |

## E2E tests

Files live under `apps/web/e2e/tests/`. Each desktop scenario has a phone scenario
with the same values and its own composition assertions.

| Files | Project | Scenarios and acceptance coverage |
| --- | --- | --- |
| `settings/sidebar-customization.spec.ts`, `settings/mobile-sidebar-customization.spec.ts` | chromium / mobile-chrome | Added and passing. Desktop covers four picker additions, hidden Home, group reorder, save/reload, navigation rendering, and live pinned-automation activity. Phone covers the focused editor, four shortcuts, same-group move controls, touch sizing, and horizontal-overflow safety. AC 001.1-.5, 002.1/.4, 003.1, 004.1-.5, 005.1-.4. |
| Existing `automations-sidebar.spec.ts` plus sidebar unit/component tests | chromium / existing regression | Covers the established automation list lifecycle, shortcut disclosure/actions, overflow, and rail behavior. These checks remain separate from the new mixed-layout browser scenario. |

Use `test-base` and existing automation/plugin fixtures. Stub summary responses
only for deterministic failure tests; prove at least one real run lifecycle.
Phone tests assert 44px bounds, safe-area scrolling, no horizontal page overflow,
and focus return. Test just below/above 768px plus the standard phone viewport.

## Work orders

- [x] [Task 01: Persist workspace sidebar layouts](task-01-persist-layouts.md)
- [x] [Task 02: Resolve shortcut references and layout operations](task-02-resolve-shortcuts.md)
- [x] [Task 03: Build the desktop and phone settings editor](task-03-edit-layouts.md)
- [x] [Task 04: Render shortcut sections across navigation surfaces](task-04-render-navigation.md)
- [x] [Task 05: Show automation activity on shortcut icons](task-05-show-activity.md)

Dependency order: 01 -> 02 -> 03 -> 04 -> 05. No parallel agents are authorized.
Work orders contain exact commands and proposed files. Install workspace
dependencies once before implementation checks if this worktree lacks them.
Managed E2E builds fresh assets; never use `--no-build` after source changes.

## Verification results

Product checks passed:

- `(cd apps/backend && go test ./internal/user/... ./internal/settingscatalog/...)` passed.
- Focused frontend Vitest coverage passed for settings mapping, layout operations and projection, shortcut catalog, editor save/conflict flows, drag-and-drop handlers, navigation, and activity.
- `(cd apps/web && pnpm run typecheck)` passed.
- `(cd apps/web && pnpm run lint)` passed.
- `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)` passed.
- `(cd apps/web && pnpm run build:vite)` passed.
- Existing desktop regression E2E passed 5 tests across automation sidebar activity and system navigation.
- Existing mobile regression E2E passed 5 tests across settings navigation, sidebar access, and touch sizing.
- New desktop sidebar customization E2E passed 2 tests, including four shortcut editing, visibility, group reorder, save/reload, navigation rendering, and a pinned automation status transition.
- New phone sidebar customization E2E passed 1 test, including the focused editor, four shortcuts, same-group move controls, touch sizing, save/reload, and horizontal-overflow safety.
- `node scripts/validate-public-docs.mjs` and `node --test scripts/validate-public-docs.test.mjs` passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

The two dedicated sidebar-customization E2E files are now present and recorded
above. The broader `layout/sidebar-shortcuts.spec.ts` and
`layout/sidebar-shortcut-activity.spec.ts` matrix files were not added; their
covered behavior remains represented by the named existing regression and
unit/component suites. No unrun scenarios are reported as verified here.

## Risks

- Shortcut status polling must stay shared and remain active when groups fold.
- Plugin navigation links are supported; arbitrary plugin slot buttons are not.
- Concurrent edits require a conflict response instead of silent overwrites.
- Existing Office/inbox contracts require fixed entries and mode-aware defaults.
- Phone controls need a separate icon strip; squeezing the desktop header fails
  touch sizing. The saved order stays shared.

## Completion

Implementation is reconciled with the design. Requirement and design statuses
are active/current, this plan is implemented, work-order results record the
checks, and public docs describe the shipped behavior. No commit, push, or PR
is part of this task.
