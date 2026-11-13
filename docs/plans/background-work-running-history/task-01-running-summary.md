---
id: "01-running-summary"
title: "Running summary and independent inspection"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-005
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-005.1
  - AC-AGENTS-BACKGROUND-WORK-005.3
  - AC-AGENTS-BACKGROUND-WORK-005.4
  - AC-AGENTS-BACKGROUND-WORK-005.5
  - AC-AGENTS-BACKGROUND-WORK-005.8
  - AC-AGENTS-BACKGROUND-WORK-005.9
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 01: Running summary and independent inspection

## Summary

Show only running workloads in the composer chip and summary. Keep inspection
reachable through independent desktop and phone entries when the chip disappears.

## In scope

- Start with RED cases for mixed states, waiting-only and unknown-only sessions.
- Add pure shared state selectors. Audit existing count consumers and preserve
  unrelated hook semantics, backend accounting, and accepted raw workload rows.
- Remove the history chip and waiting/unknown summary badges. Keep open detail
  data separate from the filtered summary. Dismiss a summary whose last running
  row leaves and restore focus to the composer when its trigger disappears.
- Add the feature-gated managed-session desktop panel-menu item and phone Panels
  action. The phone Panels trigger must exist without installed plugins.
- Share one phone work surface with summary, overview, and detail modes.
  Independent entry opens overview directly. Close the picker before opening it.
- Keep explicit session identity, detail activation, Back, draft, and focus.
- Adapt old tests that require a history chip or unknown rows in the summary.
  Create the focused E2E files that Task 02 will extend.

## Out of scope

Panel tabs, final command layout, provider lifecycle, backend/API changes,
new flags, and output-clear behavior changes.

## Acceptance

1. Chip count and summary membership equal the running subset. All other states
   remain inspectable without a chip and retain their actual status.
2. Desktop and phone independent entries open the selected session's overview.
   Feature-disabled and passthrough-only views expose neither new entry nor chip.
3. Live updates preserve draft/open details and restore valid focus. Phone uses
   successive screens, contained scrolling, and touch-sized navigation.

## ASCII UI preview

Use [UI-01 and UI-03](plan.md#ascii-ui-preview), AC-005.1, .3, .4, .8, .9.

```text
Desktop: [2 Background Jobs] -> Running work (2)
                                build... [Running] [Open]
                                [View all work]
No running jobs: no chip.  + > Background Work -> overview.

Phone: [running chip] -> inset running summary -> full-height work surface
       Panels > Background Work -------------> full-height overview
       [Back] Job [Close] -> one content scroll region
```

Summary has no history rows or status breakdown. The independent phone picker
closes before the work surface opens. Back/Close retain the selected session and
draft. Task 02 adds tabs to this shared overview.

## Verification

From the repository root, install dependencies in `apps/` first if absent.
Run TDD, then these focused commands sequentially. Task 01 creates the named
selector and E2E files before GREEN. Do not pass with zero collected tests.

```bash
(cd apps/web && pnpm exec vitest run lib/background-work/workload-views.test.ts hooks/domains/session/use-background-work.test.ts components/task/chat/background-work/background-work.test.tsx components/task/dockview-add-panel-items.test.tsx components/task/mobile/plugin-panel-picker.test.tsx components/task/mobile/session-mobile-bottom-nav.test.tsx lib/state/dockview-background-work-panel.test.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/background-work-viewer.spec.ts tests/task/opencode-background-work.spec.ts tests/task/background-work-process-end.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-background-work-viewer.spec.ts tests/task/mobile-opencode-background-work.spec.ts tests/task/mobile-background-work-process-end.spec.ts tests/chat/mobile-cancel-turn-availability.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Run scoped ESLint on changed TS/TSX paths. Record the actual path list and result.
Generate Traditional Chinese with the repository command for any new copy.

## Files likely touched

- `apps/web/lib/background-work/workload-views.ts` and its new test.
- `apps/web/hooks/domains/session/use-background-work.ts` and its test.
- `apps/web/components/task/chat/background-work/background-work-chip.tsx`.
- `apps/web/components/task/chat/background-work/background-work-panel.tsx`.
- Shared extracted overview/phone surface under the same feature directory.
- `apps/web/components/task/dockview-add-panel-items.tsx` and its test.
- `apps/web/components/task/mobile/plugin-panel-picker.tsx` and its test.
- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx` and its test.
- `apps/web/lib/state/dockview-background-work-panel.test.ts`.
- The listed E2E files and `background-work-process-end.helpers.ts`.
- `apps/web/src/locales/` catalogs and feature component tests.

## Dependencies

None. Read the current feature gate, Dockview action, phone picker, and tests.

## Risks

Avoid mounting desktop Dockview on phones. Do not filter unknown records out of
the shared store or accidentally change prompt-admission estimates.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/background-work.md), REQ-005.
- [Shared design](../../specs/agents/system-design/background-work.md#shared-client-and-responsive-ui).
- Existing OpenCode tests and the process-end helper.

## Results

Completed 2026-11-03.

- Added shared state selectors in `apps/web/lib/background-work/workload-views.ts`
  with unique accepted rows and running-only, Active, and History membership.
  Tab selection, tab-scoped search, and their rendered states belong to Task 02.
- `useBackgroundWork` now publishes `runningWorkloads`, `activeWorkloads`, and
  `historyWorkloads` alongside the unchanged counts and raw list. The only
  count consumers were the chip and tests; prompt admission, parked activity,
  and backend accounting were not touched.
- `BackgroundWorkChip` and its summary show running work only. The history chip,
  waiting/unknown count badges, and non-running summary rows are gone. Open
  details read the raw accepted list, so a live transition does not close them.
  The summary dismisses when its last running row leaves and focuses the
  composer through the new `onFocusComposer` callback.
- Added the feature-gated desktop panel-menu item and the built-in phone Panels
  option. Both open the selected session's overview, including empty lists.
- Added one shared phone work surface for the inset summary, full-height
  overview, and full-height detail. Back restores the originating tab/search,
  and dismissal returns focus to the Panels trigger or composer.
- Added `workload-views.test.ts`, extended the hook/chip/panel tests, and added
  the desktop and phone viewer E2E specs.

Verification:

- `pnpm exec vitest run` for the ten focused files: 10 files, 144 tests
  passed (including a deferred first-read regression through the real hook and
  store).
- `pnpm exec tsc --noEmit` (with an 8 GB Node heap): passed.
- `pnpm run i18n:check`: passed; catalogs complete and pseudo in sync.
- Scoped ESLint over every changed TS/TSX path: 0 errors.
- Desktop `chromium` E2E for the new viewer and both adapted OpenCode/process-end
  suites: 7 tests passed, including a 320px fine-pointer viewport.
- Phone `mobile-chrome` E2E for the new viewer, adapted OpenCode/process-end,
  and mobile cancel availability: 6 tests passed.
- `git diff --check`: clean.

Browser checks used real Chromium against the built backend and SPA. The new
desktop and phone specs capture overview/detail screenshots through `prCapture`.

### Review fixes

- Added session-scoped first-read readiness (`isInitialLoading`) so an empty
  initial store waits for the first fetch before choosing Active or History.
  Added a deferred real-hook/store regression for running and terminal-only
  first loads.
- Phone Background Work now requires a managed session. A restored
  `background-work` panel selection falls back to Chat for passthrough or a
  disabled feature, and the Panels menu still exposes unrelated tools.
- Back from a summary detail now requests History when the last running job has
  ended, restores focus to the inspected History row, and leaves overview-origin
  tab/search/scroll intact.
- Moved complete commands out of fixed detail headers into the single detail
  scroll region. Added a tall multiline command E2E proving the command end,
  status/actions, and output remain reachable without horizontal scrolling.
- Associated each tab with a Radix `TabsContent` panel and added role,
  `aria-controls`, `aria-labelledby`, and keyboard-navigation coverage.


