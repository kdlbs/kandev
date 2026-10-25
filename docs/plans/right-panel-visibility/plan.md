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

## Review remediation verification (2026-09-14)

The review findings against PR head `8b98810227f1aa5e3058d32461122eb747b86b00` are resolved in the current
worktree. Recovery metadata is rejected before mutation when IDs, active selections, parameters, geometry,
tree/flat groups, or layout-wide identities are unsafe. Layout applies retain the live serialized Dockview
state and roll back once after a mutating failure, while the contextual hidden descriptor and visibility flags
remain recoverable without partial persistence. Non-pinned restored panes reserve their captured width within
the current viewport and distribute the remaining live proportions. Duplicate-panel pruning retains split
wrappers, preserving nested axes and sizes, and generated group IDs avoid explicit live IDs.

The current remediation checks are:

- Exact work-order unit command: 10 files, 135 tests passed.
- Additional affected unit suite: 8 files, 79 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Managed Chromium E2E: 12 tests passed.
- Managed mobile-Chromium E2E: 1 test passed.
- E2E Vite builds passed with existing repository warnings only.
- The earlier PR snapshot's failed E2E shards 2/14 and 6/14 were not attributed by this remediation. The current
  local focused runs pass, and current CI output will be reviewed after the fixup push.
- `git diff --check` passed.

## PR fixup verification (2026-09-14)

The first current-head PR run for `2bcaa1faf28f55fc2cfbdac122e791cdced628f0` reached 44 passed, 9 failed,
and 0 pending checks before the repository PR helper's 45-minute deadline. The failed E2E shards were unrelated
to this change: shard 2 reported two narrow-tab-strip assertions, shard 6 reported one fork pull-request
comparison assertion, and shard 14 reported one Plan table pixel-tolerance assertion plus flaky Agent restart
and mobile file-viewer cases. The E2E and frontend aggregate failures depended on those leaf results.

The frontend leaf job also exposed one compatibility failure caused by the new transactional wrapper: an existing
`dockview-store.test.ts` mock omitted `api.toJSON()`. The wrapper now guards reduced test doubles while retaining
production rollback behavior. The targeted follow-up suite passed 11 files and 157 tests, and targeted ESLint and
typecheck passed.

The PR documentation coverage job exited without evaluator output. Replaying the evaluator locally against the
current PR event and against its trusted validator revision returned `covered`, so the status requires a fresh
workflow result after the fixup push. Aggregate checks will be re-evaluated from the new head.

## Follow-up PR fixup verification (2026-09-14)

After the reduced-test-double compatibility fix was pushed as `8e866cf518d07f8255ecbc927f90d33eb7be8edb`,
`scripts/pr-await 3661 --deadline-min 60` waited 18 minutes and reached the terminal exact-head snapshot:
47 passed, 6 failed, and 0 pending. The PR was `MERGEABLE / BLOCKED`, with no unresolved review threads.
The base branch advanced during the run, so the result is not a current-base merge-readiness claim.

The failed leaf E2E jobs were outside the contextual right-pane change:

- Shard 2 failed both existing narrow-tab-strip assertions, one for equal row widths and one for zero overflow.
- Shard 6 failed the existing fork pull-request comparison assertion because the unavailable-target notice remained.
- Shard 14 failed the existing Plan table pixel assertion, receiving `59.15625` for an expected resize delta of `60`.
  Its LSP capacity-release and mobile file-viewer failures were transient attempts of unrelated tests.
- The E2E aggregate and report-merge failures followed the failed leaf shards.
- The trusted PR documentation status job again exited without evaluator output. Local replay of the current and
  trusted evaluators returned `covered`.

The three relevant no-retry reproductions were run locally after the PR report: the Plan table test reproduced the
existing `59.15625` pixel delta failure, while the LSP capacity-release test passed one test in 48.8 seconds and the
mobile file-viewer test passed one test in 13.1 seconds. No additional source fix was justified by these unrelated
failures. The contextual right-pane unit, browser, typecheck, lint, localization, documentation, specification,
and diff checks remain passed as recorded above.

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
