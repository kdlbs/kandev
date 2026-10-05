---
created: 2026-10-05
status: complete
requirements:
  - REQ-AGENTS-PAGE-OPTIONS-001
system_design:
  - ../../specs/agents/system-design/agent-page-options.md
legacy_specs: []
---

# Implementation plan: Agent page options

## Overview

Move the approved navigation preference into Options beside Terminal.
One sequential work order delivers the complete desktop and phone interaction, translations, tests, and public guidance.
The reviewed sketch settles the surface, wording, control order, and immediate-save behavior.

## Scope

### In scope

- A desktop dialog and phone drawer with the existing visibility preference.
- Toolbar placement, focus and dismissal behavior, localization, and targeted verification.
- A short explanation in the public agents guide.

### Out of scope

- Additional preferences, new persistence, backend changes, and unrelated profile or toolbar changes.
- Subagents, commits, pushes, and PR publication.

## Technical approach

`InstalledAgentsHeader` mounts the new route-local `AgentOptionsDialog` before Terminal.
The component owns the shared open state and active Dialog/Drawer branch.
`HideDisabledAgentProfilesSetting` supplies the existing control inside that surface.
Remove its standalone placement from `InstalledAgentsSection`.
Keep `useHideDisabledAgentProfilesInNav` and the navigation branch builder unchanged.

The older navigation requirement now reflects the current UI placement and label.
Its filtering criteria and IDs remain intact, and its existing preference is scoped to the browser profile.
The public guide now explains the Options entry point and browser-profile scope.

## ASCII UI preview

Structure and action order are required. Spacing and icon drawings are illustrative.
Copy uses localization. The existing settings page retains its current scroll owner.

### UI-01: Desktop page, current and proposed closed state

Current placement is verified in `InstalledAgentsSection`.
Maps to AC-AGENTS-PAGE-OPTIONS-001.1 and .8.

```text
CURRENT
Installed Agents                 [Terminal] [Rescan] [+ Add custom agent]
+---------------------------------------------------------------------+
| Hide disabled agent profiles from left panel navigation        [OFF] |
+---------------------------------------------------------------------+
| Agent cards and profile rows                                        |
+---------------------------------------------------------------------+

PROPOSED
Installed Agents       [Options] [Terminal] [Rescan] [+ Add custom agent]
+---------------------------------------------------------------------+
| Agent cards and profile rows                                        |
+---------------------------------------------------------------------+
```

### UI-02: Desktop Options, open state

Entry: UI-01 Options. Maps to AC-AGENTS-PAGE-OPTIONS-001.2, .3, .5, .6, and .7.

```text
+------------------------------------------------+
| Agent options                              [x] |
|                                                |
| Hide disabled profiles                         |
| from navigation                          [OFF] |
| Disabled profiles remain available             |
| on this page.                                  |
|                                                |
| Changes apply immediately.                     |
|                                       [ Done ] |
+------------------------------------------------+
```

Successful toggle: `[OFF]` becomes `[ON]`; the dialog stays open.
Done, Close, or Escape dismisses and returns focus to Options.

### UI-03: Phone page and open Options drawer

Entry: Options in the wrapping toolbar. Maps to AC-AGENTS-PAGE-OPTIONS-001.3 through .8.

```text
PHONE PAGE                       OPEN DRAWER
+-----------------------------+  +-----------------------------+
| Installed Agents            |  | Dimmed agents page          |
| [Options] [Terminal]        |  |                             |
| [Rescan] [+ Add custom agent]|  | +-------------------------+ |
|                             |  | |           ---           | |
| Agent cards                 |  | | Agent options           | |
| and profile rows            |  | |                         | |
+-----------------------------+  | | Hide disabled profiles  | |
                                 | | from navigation   [OFF] | |
                                 | | Disabled profiles remain| |
                                 | | available on this page. | |
                                 | |                         | |
                                 | | Changes apply           | |
                                 | | immediately.            | |
                                 | | [        Done         ] | |
                                 | +-------------------------+ |
                                 +-----------------------------+
```

The drawer uses a fixed header and footer, one scrollable body, and bottom safe-area clearance.
The exact toolbar wrapping depends on translated labels. Control order remains stable.
Options, Done, and the switch interaction have at least 44px touch targets.

## Tests

| Acceptance | Evidence |
| --- | --- |
| .2, .3, .6, .7 | Update `hide-disabled-agent-profiles-setting.test.tsx`: label, description, stored value, and immediate setter call |
| .7, .8 | Existing `use-hide-disabled-agent-profiles-in-nav.test.ts` and `use-local-storage-boolean.test.ts` retain default, persistence, event, and error coverage |
| Existing page behavior | Run `page.test.tsx`; update its child mock if needed |

## E2E tests

| Flow | AC references | File and project |
| --- | --- | --- |
| Toolbar order, no inline row, dialog toggle, Done/Close/Escape, focus return, reopen/reload, view-only access | .1, .2, .3, .5, .7, .8 | `tests/settings/agent-page-options.spec.ts`, chromium |
| Existing disabled-profile navigation filtering through Options | .3, .8 | `tests/settings/hide-disabled-agent-profiles-nav.spec.ts`, chromium |
| Content-sized drawer, visible disabled profile rows, saved value, dismissal, focus, safe area, containment, 44px targets | .3, .4, .5, .7, .8 | `tests/settings/mobile-agent-page-options.spec.ts`, mobile-chrome |
| 767px fine-pointer drawer, resize to 768px dialog and back with value preserved | .4, .7 | `tests/settings/agent-page-options.spec.ts`, chromium |
| Coarse-pointer tablet dialog keeps Options, switch, Done, and Close touch-sized | .4, .5 | `tests/settings/agent-page-options.spec.ts`, chromium `tabletTestPage` |

Use disposable profiles or restore the exact baseline after a mutation.
Open Options and dismiss it before navigating in the existing navigation test.
Use fresh managed builds and run desktop and phone projects sequentially.
Capture both open surfaces and compare their structure with UI-02 and UI-03.

## Work orders

- [x] [Task 01: Move the agent preference into Options](task-01-options-surface.md)

## Verification results

Implementation completed on 2026-10-05. The toolbar now has a labelled Options
button before Terminal; the existing browser-profile preference is available in
a compact desktop dialog and an inset phone drawer. The new control remains
available to members without agent-management access, and the inline preference
row is removed.

- Focused Vitest suite: 4 files and 23 tests passed.
- Targeted ESLint with `--max-warnings 0` and Prettier checks: passed for the
  changed web and E2E TypeScript files.
- `pnpm run typecheck` and the production build used by managed E2E: passed.
- `pnpm run i18n:check`: passed; all six translated catalogs and the pseudo
  locale passed completeness checks.
- Managed Chromium E2E: 6 tests passed for toolbar order, dialog behavior,
  member access, breakpoint changes, navigation filtering, and touch-sized
  controls on a coarse-pointer tablet.
- Managed Pixel 5 mobile Chromium E2E: 1 test passed. The rendered drawer stays
  content-sized at less than 70% of the viewport for the single-preference
  surface, with an 80dvh dynamic maximum. It stays within the viewport, uses
  one internal scroll region, clears the footer safe area, and has 44px Options
  and switch targets plus a 48px Done button.
- Host-rendered desktop and phone screenshots were compared with UI-02 and
  UI-03; both match the approved structure.
- Public documentation tests: 62 passed; all 47 published pages validated.
- `python3 scripts/list-docs.py validate`: 351 decisions and 1357
  specifications validated. All 36 spec-linter tests and the full specification
  lint passed.
- `git diff --check`: passed.

## Risks

- Dialog and Drawer inherited breakpoint classes can defeat caller size overrides.
- A small Switch graphic requires a larger touch interaction area without double toggles.
- The existing navigation E2E currently assumes the switch is inline and must open Options first.
- Localized labels can change toolbar wrapping and overlay height.
