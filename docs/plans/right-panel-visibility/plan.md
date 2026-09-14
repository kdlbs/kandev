---
created: 2026-09-13
status: implemented
requirements:
  - REQ-UI-RIGHT-PANEL-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/right-panel-visibility.md
legacy_specs: []
---

# Implementation plan: Contextual right-pane visibility

## Overview

The toggle hides and restores the rightmost region in the current layout.
Plan Mode targets Plan, Preview Mode targets Browser, VS Code targets its editor, and Default targets its complete right stack.
The user requested this correction on 2026-09-14 and supplied screenshots showing the Plan and Preview layouts.
Task 02 implements the correction. Historical Task 01 results remain context only and do not replace the
contextual behavior evidence below.

## Baseline and ownership

Current baseline: `03fe74e955a869a30819e6d9b3f59c83bb1c3c45`.
Earlier commits: `88241db53` implemented the persistent toggle; `03fe74e95` addressed review findings.
UI continues to own the layout contract. Task, agent, terminal, and portable layout-profile ownership do not change.

`toggleRightPanels` currently filters legacy right-owned columns and reconstructs `defaultLayout()` on Show.
The new implementation selects from live geometry and retains the removed subtree for restoration.
The user explicitly replaced the previous Files/Changes/Terminal-only behavior.
This also supersedes compact-mode creation of a standard sidebar and the exclusion of custom-pane restoration.

## Scope

### In scope

- Geometric target selection and exact pane restoration for built-in and custom Dockview arrangements.
- Per-environment hidden-pane recovery with existing environment layout persistence.
- Reset/preset invalidation, task switching, nested splits, active Agent protection, duplicate prevention, and maximize guards.
- Existing header placement, localized state explanations, touch targets, and tablet/phone parity.
- Focused regression coverage and public instructions updated during implementation.

### Out of scope

- Backend settings, new breakpoints, phone sidebars, arbitrary undo history, and terminal process lifecycle changes.
- Changes outside the task workbench visibility contract, such as backend preferences, new breakpoints,
  phone sidebars, undo history, or terminal lifecycle behavior.

## Technical approach

Follow the revised [requirements](../../specs/ui/requirements/right-panel-visibility.md) and
[system design](../../specs/ui/system-design/right-panel-visibility.md).
Select the final child of the outer horizontal workbench split, retaining its full nested subtree.
When a hidden descriptor exists, Show restores it before any further target selection.
Persist recovery metadata atomically with the environment layout; never copy it into another environment or portable profile.
Keep remaining live edits when restoring. Do not call `defaultLayout()` as a Show fallback.
A single region without retained recovery data has no toggle target.

## ASCII UI preview

### UI-03: Contextual right-pane toggle

Entry: a desktop workbench, including large tablets. Header stays fixed; pane content owns scrolling.

```text
PLAN MODE: shown
+---------------------------------------------------+
| Task             [Hide right pane] [Layouts v]     |
+--------------------------+------------------------+
| Agent                    | Plan                   |
+--------------------------+------------------------+

PLAN MODE: hidden
+---------------------------------------------------+
| Task             [Show right pane] [Layouts v]     |
+---------------------------------------------------+
| Agent fills the released width                    |
+---------------------------------------------------+
Show restores Plan with its tabs and split state.

PREVIEW MODE: shown
+---------------------------------------------------+
| Task             [Hide right pane] [Layouts v]     |
+--------------------------+------------------------+
| Agent                    | Browser                |
+--------------------------+------------------------+
Hide -> Agent fills width. Show -> the same Browser.

DEFAULT: shown                 VS CODE: shown
+---------------+-----------+  +---------------+-----------+
| Agent         | Files     |  | Agent         | VS Code   |
|               | Changes   |  |               |           |
|               +-----------+  +---------------+-----------+
|               | Terminal  |
+---------------+-----------+
Default toggles the whole right stack. VS Code toggles VS Code.

CUSTOM: three side-by-side regions
+---------------+-----------+---------------+
| Agent         | Plan      | Browser       |
+---------------+-----------+---------------+
Hide removes Browser only; the next click restores Browser.
It does not continue removing Plan.

SINGLE REGION: no retained hidden pane
+---------------------------------------------------+
| Task       [right-pane icon disabled] [Layouts v]  |
+---------------------------------------------------+
| Agent / Files / Changes tabs in one group          |
+---------------------------------------------------+
Explanation: No separate right pane to hide.
```

Button labels in this drawing stand for localized tooltips and accessible names; the actual header keeps its existing icon.
The left navigation toggle remains independent. Spacing is illustrative; group identity and hide/show results are required.
Initialization and maximize retain their existing disabled states. A hidden target always takes precedence over a new hide target.

### UI-02: Phone and tablet fallback

```text
Phone: one active surface       Narrow tablet fallback
+-------------------------+     +----------------+-----------+
| Chat / Files / Terminal |     | Chat/Plan/...  | Files     |
|                         |     |                | Terminal  |
+-------------------------+     +----------------+-----------+
| Existing bottom nav     |     Persistent header toggle hides
+-------------------------+     and restores this right stack.
```

Phone navigation, safe areas, and full-screen content remain unchanged. No phone toggle is added.
UI-03 maps to AC-UI-RIGHT-PANEL-VISIBILITY-001.1-.5 and .7-.10; UI-02 maps to .3, .5, and .6.

## Tests and E2E matrix

Task 02 owns these cases and exact commands. Use production-shaped Agent panels and real serializer output.

| Scenario                                               | Required evidence                                                                         |
| ------------------------------------------------------ | ----------------------------------------------------------------------------------------- |
| Plan, Preview, VS Code, Default                        | Hide releases width; Show restores the exact target, not a Files sidebar                  |
| Custom three-column and nested target                  | Only outer right region toggles; tabs, parameters, selected tabs, tree, and width survive |
| One group, vertical-only split, rightmost active Agent | Disabled explanation; no deletion or fabricated sidebar                                   |
| Repeated hide/show                                     | Alternates the same target; unique panel IDs; remaining center content survives           |
| Reopen a hidden panel elsewhere                        | Live instance wins; no duplicate or whole-layout reset                                    |
| Hidden reload and A/B environment switch               | Correct label and correct per-environment target survive                                  |
| Hidden Plan then select Preview or Reset               | Old target is discarded; only the new arrangement determines the next action              |
| Legacy, malformed, or unavailable panel metadata       | Valid visible layout survives; no stale panel resurrection                                |
| Maximize, rapid clicks, late callbacks                 | No overlay capture or cross-environment mutation                                          |
| 1280px coarse, 900px fine/coarse, phone                | Touch geometry, keyboard focus, no overflow, and unchanged phone navigation               |

## Work orders

- [x] [Task 01: Persistent toggle](task-01-persistent-toggle.md). Completed historical scope; superseded behavior is identified there.
- [x] [Task 02: Contextual right-pane selection and restoration](task-02-contextual-right-pane.md). Done; depends on Task 01.

Execute Task 02 as one sequential vertical slice in the primary session.

## Current revision verification

Task 02 implementation and validation completed on the current branch.

- Focused unit suite: 10 files, 130 tests passed.
- `pnpm run typecheck` and `pnpm run lint` from `apps/web` passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web` passed.
- Managed Chromium E2E matrix: 12 tests passed across right-pane, tablet-persistence, and compact-desktop scenarios.
- Managed mobile-Chromium E2E: 1 phone test passed.
- The managed E2E builds passed; Vite emitted only the repository's existing chunk-size and dynamic-import warnings.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs` passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

The browser runs cover Default, Plan, Preview, compact single-region, maximize/exit, keyboard focus, tablet,
and phone behavior. The broader matrix still includes resize handoffs, the 1280-pixel coarse-pointer case,
all four sidebar combinations, archived-task restoration, browser-level mixed-center fixtures, and the
wider-to-phone handoff; those remain separate coverage beyond this implementation run.

## Historical Task 01 verification

The previous standard-sidebar implementation was completed before the 2026-09-14 behavior correction. The new control is shared by desktop and tablet adapters, the tablet right column is conditional, compact desktop can reopen it, and phone navigation keeps its existing full-screen composition.

Checks passed:

- `pnpm install --frozen-lockfile` from `apps`.
- Review-focused unit tests after fixup: 9 files, 102 tests; the store-focused follow-up passed 45 tests in 2 files.
- Full web unit suite: 2,058 files, 17,788 passed and 4 skipped tests.
- `pnpm run typecheck` and `pnpm run lint` from `apps/web`.
- Targeted ESLint for changed source and browser files.
- Prettier check for changed TypeScript, TSX, and JSON files.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web`.
- `pnpm --filter @kandev/web build:vite` from `apps`.
- Managed Chromium E2E fixup run: 5 right-panel tests passed, including compact reload, maximized disabled/exit/reload, tablet persistence, and keyboard activation.
- Managed mobile-chrome E2E fixup run: 1 test passed with Pixel 5 device and coarse-pointer assertions.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 46 published documents validated.
- `python3 scripts/list-docs.py validate`: 267 decisions and 896 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.

The implementation adds localized labels in all five supported catalogs and updates the public task-workspace instructions.

The assertions above cover the review regressions. The retained browser matrix is broader than this run. The tablet component test uses mocked panel primitives and persistence callbacks, so it proves conditional composition and center identity only; the browser test proves the stored visibility round trip, while saved split geometry remains in the retained matrix. The component test proves focus retention after a click and the disabled maximized accessibility wrapper; the browser test proves native Enter and Space activation plus maximized exit/reload recovery. Resize handoffs, archived-task restoration, the 1280-pixel coarse-pointer case, all four sidebar combinations, mixed center/right browser fixtures, and the wider-to-phone handoff remain planned coverage.

New files were inspected explicitly; work-order references resolve to the new requirement and design.
Implementation commands and browser results are recorded above and in the completed work order.

## Risks

- Column names can describe panel contents rather than physical placement; selection must use actual split geometry.
- Tree-based serialization can reintroduce removed panels if flat groups and nested trees disagree.
- A whole-layout restore can overwrite edits made while the pane was hidden; reinsert only the retained target.
- Recovery metadata can be lost by a save path that only serializes Dockview JSON; cover every environment save and restore path.
- Default-only width enforcement must not resize restored Plan, Browser, or custom panes incorrectly.

## Public documentation

Task 02 updates `docs/public/tasks-and-workflows.md` to describe the active layout target and single-region disabled state.
Requirements, system design, plan, work order, implementation, tests, and public instructions now describe the same behavior.
