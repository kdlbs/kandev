---
id: "02-contextual-right-pane"
title: "Toggle the actual rightmost layout pane"
status: done
wave: 2
depends_on:
  - "01-persistent-toggle"
plan: "plan.md"
requirements:
  - REQ-UI-RIGHT-PANEL-VISIBILITY-001
acceptance_criteria:
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.1
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.2
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.3
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.4
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.5
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.6
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.7
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.8
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.9
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.10
system_design:
  - ../../specs/ui/system-design/right-panel-visibility.md
---

# Task 02: Toggle the actual rightmost layout pane

## Summary

Replace standard-sidebar reconstruction with geometric target selection and exact hidden-pane restoration.
Plan Mode toggles Plan; Preview Mode toggles Browser; VS Code toggles its editor; Default toggles its complete right stack.
This is the current implementation handoff. Task 01 is historical context only.

## In scope

- Capture the rightmost outer horizontal region and its nested groups from the live layout.
- Retain the hidden region and restore it into the current arrangement without losing remaining edits.
- Persist versioned recovery metadata with the existing environment layout and audit every save/restore/reset path.
- Preserve active Agent content, unique panel IDs, selections, parameters, and bounded geometry.
- Keep maximize and readiness guards. Disable the control when no separate eligible target or retained hidden pane exists.
- Update affected copy in all locales and public instructions with the implemented behavior.
- Add the complete scenario matrix below through TDD. Update historical compact Show tests to the new disabled contract.

## Out of scope

No backend preference, portable-profile schema, new breakpoint, phone sidebar, undo history, or terminal lifecycle change.
Do not implement the superseded `defaultLayout()` fallback from Task 01.

## Acceptance

- UI-03 toggles the actual rightmost region and restores its identity and arrangement through reload and environment switches.
- Single-region and unsafe targets are disabled; presets and Reset discard stale hidden targets without introducing a default sidebar.
- All listed checks and scenario evidence are recorded; UI-02 retains tablet/phone behavior.

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

See the [combined preview](plan.md#ascii-ui-preview). Required geometry and state transitions are described in the owning requirements.

## Implementation sequence

1. Task 02 was implemented against the revised contract after reading the current implementation.
2. Add behavioral RED tests for Plan/Browser target selection and restoration against the existing default-sidebar fallback.
3. Implement selection, retained-subtree restoration, metadata lifecycle, and effective control state as one vertical slice.
4. Add production-shaped round trips for nested trees, single compact groups, A/B environments, presets, and invalid metadata.
5. Extend the browser scenarios and inspect rendered desktop, touch-tablet, and phone compositions against UI-03/UI-02.
6. Update public copy and record exact outcomes. This work order is complete after the checks below passed.

## Regression matrix and traceability

- `.1`, `.2`, `.8`, `.9`: Plan, Browser, VS Code, and Default hide/show identity, reclaimed width, selected tabs and focus.
- `.2`, `.8`, `.9`: custom three-region and nested right subtree; active Agent and mixed center tabs stay intact.
- `.3`, `.7`, `.8`: compact one-group and vertical-only arrangements; disabled controls; active Agent in the rightmost region.
- `.5`, `.9`, `.10`: hidden reload, A-hidden/B-visible round trips, changed remaining layout, and already-reopened hidden panel deduplication.
- `.7`, `.10`: maximize/exit/reload, repeated activation, old operation callback after environment switch, malformed or obsolete metadata.
- `.5`, `.10`: hide Plan then apply Preview; custom layout application and Reset; no hidden Plan resurrection.
- `.4`, `.6`: keyboard/focus, localized explanations, 44px touch bounds at 1280px and 900px, phone navigation, no horizontal overflow.

Implement pure selection and snapshot cases in proposed `lib/state/dockview-right-pane.test.ts`.
Extend existing store, serializer, persistence, adapter, and component tests named in the verification command.
Extend `e2e/tests/layout/right-panel-visibility.spec.ts` for built-in/custom round trips and browser state transitions.
Use `tabletTestPage` for coarse-pointer scenarios and the mobile-chrome project for phone coverage.
Use real captured trees and `session:<id>` Agent identities; mocked `id: right` fixtures alone are insufficient.

## Verification

```bash
# Repository root; install once if the workspace dependencies are absent.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/state/dockview-right-pane.test.ts lib/state/dockview-right-panel-visibility.test.ts lib/state/dockview-env-switch-action.test.ts lib/state/layout-manager/serializer.test.ts lib/local-storage.test.ts components/task/dockview-layout-restore.test.ts hooks/use-task-right-panels-toggle.test.ts components/task/task-right-panels-toggle.test.tsx components/task/mobile/session-tablet-layout.test.tsx lib/state/dockview-preset-persistence.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/layout/right-panel-visibility.spec.ts tests/layout/pane-persistence-tablet.spec.ts tests/layout/compact-desktop-responsive.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/layout/mobile-right-panel-visibility.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Desktop and mobile were run separately through the managed runner with repository causal waits and isolated fixtures.
The focused browser evidence is recorded below; the broader retained matrix remains explicitly identified as coverage beyond this turn.

## Files likely touched

New focused helper and tests:

- `apps/web/lib/state/dockview-right-pane.ts`
- `apps/web/lib/state/dockview-right-pane.test.ts`

Existing implementation boundaries:

- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-env-switch.ts`
- `apps/web/lib/state/layout-manager/serializer.ts`
- `apps/web/lib/state/layout-manager/types.ts`
- `apps/web/lib/state/dockview-layout-health.ts`
- `apps/web/lib/local-storage.ts`
- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/dockview-layout-restore.ts`
- `apps/web/components/task/dockview-layout-setup.ts`
- `apps/web/hooks/use-task-right-panels-toggle.ts`
- `apps/web/components/task/task-right-panels-toggle.tsx`
- Existing tests and E2E files listed in Verification.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`
- `docs/public/tasks-and-workflows.md`
- This work order and `plan.md` for statuses and exact results.

Keep tablet production changes limited to regressions required by the new shared control state.
Use existing locale generation scripts for Traditional Chinese and pseudo catalogs.

## Dependencies and inputs

Task 01 implementation and review fixup are present at `03fe74e955a869a30819e6d9b3f59c83bb1c3c45`.
Read the revised [requirements](../../specs/ui/requirements/right-panel-visibility.md) and
[system design](../../specs/ui/system-design/right-panel-visibility.md).
Read `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, and `/e2e` before implementation.
Use the existing `LayoutColumn.tree`, serializer, environment restore, and disabled-tooltip patterns.

## Risks

Tree/flat-group disagreement, overwritten remaining edits, stale environment metadata, and duplicate panel identities are primary risks.
Preserve existing runtime eligibility checks for sessions, reviews, and plugin panels during restoration.

## Parallelism

`sequential`. No agent is launched by this package update. The user will delegate implementation.

## Results

Implemented the contextual right-pane contract. The live serializer now records root orientation and the
rightmost eligible workbench subtree. Hide stores a versioned environment-scoped descriptor; Show reinserts
that exact tree, preserves remaining edits, and reconciles panels that were reopened elsewhere. Presets,
custom layouts, Reset, environment switches, and maximize restore paths invalidate or carry the descriptor
according to the contract. A single eligible region remains disabled without fabricating a default sidebar.
Phone navigation remains unchanged, and tablet uses its existing responsive owner.

Validation passed:

- Focused work-order unit command: 10 files, 130 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Managed Chromium command: 12 tests passed, including Default, Plan, Preview, compact single-region,
  maximize/exit, keyboard focus, and tablet persistence.
- Managed mobile-Chromium command: 1 test passed for phone navigation parity.
- The managed E2E builds passed with existing Vite chunk-size and dynamic-import warnings only.
- Public-document tests and validation, specification catalog validation, specification lint, and
  `git diff --check` passed.

The broader browser matrix still includes resize handoffs, the 1280-pixel coarse-pointer case, all four
sidebar combinations, archived-task restoration, browser-level mixed-center fixtures, and the wider-to-phone
handoff. Those are retained coverage boundaries and are not claimed as executed by this implementation step.

## Review remediation and current verification (2026-09-14)

The review of PR head `8b98810227f1aa5e3058d32461122eb747b86b00` identified three correctness gaps. They are
fixed in the current worktree:

- Recovery metadata now validates IDs, active selections, parameter values, finite geometry, tree/flat-group
  consistency, and layout-wide panel, group, and column uniqueness before a restore can reach Dockview.
- Programmatic layout applies retain the live serialized Dockview state and perform one bounded rollback when
  an apply mutates the grid and then fails. The toggle keeps its recovery descriptor and flags, and does not
  persist a partial result.
- Restoring a non-pinned Plan, Browser, VS Code, or custom pane reserves its captured width, clamps it to
  available space, and distributes the remaining width according to the current live proportions.
- Duplicate-panel filtering retains one-child split wrappers, so nested alternating axes and split sizes survive
  subtree pruning. Generated group IDs skip IDs already present in the live layout.

Current checks for this remediation:

- Exact work-order unit command: 10 files, 135 tests passed.
- Additional affected unit suite: 8 files, 79 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Managed Chromium command: 12 tests passed across right-pane visibility, tablet persistence, and compact
  desktop scenarios.
- Managed mobile-Chromium command: 1 phone navigation test passed.
- E2E Vite builds passed with the existing chunk-size, deprecated-option, and ineffective-dynamic-import
  warnings.
- The previous PR snapshot's failed E2E shards 2/14 and 6/14 were not attributed to these findings. The focused
  local runs above pass; any current CI failure will be handled from its current check output.
- `git diff --check` passed after the remediation edits.

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
