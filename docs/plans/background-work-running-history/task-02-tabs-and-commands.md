---
id: "02-tabs-and-commands"
title: "Tabbed history and contained commands"
status: completed
wave: 2
depends_on:
  - "01-running-summary"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-005
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-005.3
  - AC-AGENTS-BACKGROUND-WORK-005.4
  - AC-AGENTS-BACKGROUND-WORK-005.5
  - AC-AGENTS-BACKGROUND-WORK-005.6
  - AC-AGENTS-BACKGROUND-WORK-005.7
  - AC-AGENTS-BACKGROUND-WORK-005.8
  - AC-AGENTS-BACKGROUND-WORK-005.9
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 02: Tabbed history and contained commands

## Summary

Add Active and History tabs to the shared desktop and phone overview. Keep
commands readable without horizontal scrolling and preserve open inspection.

## In scope

- Use Task 01 selectors with `@kandev/ui/tabs`. Resolve the initial tab after
  loading, then preserve selection across accepted state updates.
- Keep counts before search and apply the same search within each tab. Add
  localized tab-empty and search-empty states. Unknown remains visibly unknown.
- Constrain card/header text tracks, status, Open, toolbar, panel, and drawer
  ancestors. Truncate compact titles and wrap the complete command in details,
  including unbroken tokens. Keep the full accessible command name.
- Preserve phone list tab/search/scroll on detail Back, and desktop detail tab
  identity on repeat selection. Do not close details when state changes.
- Extend desktop/phone E2E with long commands, live transitions, no-running
  history entry, and measured overflow/touch/focus assertions.
- Adapt the existing clear-window rendered test to the new phone entry as
  needed. Preserve real-store output-clear coverage.
- Update existing inspection how-to sections in public task and agent docs.
  Reconcile affected test mappings in companion packages without rewriting
  their historical results or marking unrelated tasks complete.

## Out of scope

Provider completion delivery, data retention, controls, accounting, new flags,
and redesign of shared panel primitives or unrelated mobile navigation.

## Acceptance

1. Active and History separate every accepted workload by state. Counts, search,
   loading/default selection, empty states, and live moves follow REQ-005.
2. Compact and complete commands remain usable with status/actions contained,
   without horizontal scroll in narrow desktop panels or phone list/detail.
3. Keyboard/touch navigation, Back, focus, session identity, draft, and output
   clear behavior pass the targeted rendered and E2E checks.

## ASCII UI preview

Use [UI-02 through UI-04](plan.md#ascii-ui-preview), AC-005.3-.9.

```text
Desktop overview
Background Work
[Active (2)] [History (5)]  [Search jobs...]
command...                 [State] [Open]
one vertical list scroller

Phone full-height overview       Phone full-height detail
Background Work [Close]          [Back] Job [Close]
[Active (2)] [History (5)]        complete command wraps
[Search jobs...]                 including/unbroken/tokens
command... [State] [Open]         [Unknown]
one list scroller                one content scroller
safe area                        safe area
```

Keep headers/tabs/search fixed. Details show the complete command. Lists use
ellipsis without hiding status or Open. Empty tabs remain selected after live
changes. Unknown-only first load opens History. These are structural requirements.

## Verification

From the repository root. Run commands sequentially with installed workspace
dependencies. Task 01 creates the selector and E2E files. Extend them with RED
regressions before changing tabs or layout.

```bash
(cd apps/web && pnpm exec vitest run lib/background-work/workload-views.test.ts hooks/domains/session/use-background-work.test.ts components/task/chat/background-work/background-work.test.tsx components/task/chat/background-work/background-work-clear-window.test.tsx lib/state/dockview-background-work-panel.test.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/background-work-viewer.spec.ts tests/task/opencode-background-work.spec.ts tests/task/background-work-process-end.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-background-work-viewer.spec.ts tests/task/mobile-opencode-background-work.spec.ts tests/task/mobile-background-work-process-end.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run scoped ESLint on changed TS/TSX paths and record the exact invocation.
Capture focused desktop/phone screenshots during these E2E checks. Compare them
with UI-02/UI-03. Measure `scrollWidth <= clientWidth` on the panel, card,
drawer, and document. Include a 320px desktop panel, long unbroken command,
phone touch targets, narrow fine-pointer phone, and 767px/768px boundaries.
Keep one E2E run at a time. Report blocked browser evidence explicitly.

## Files likely touched

- `apps/web/components/task/chat/background-work/background-work-panel.tsx`.
- `apps/web/components/task/chat/background-work/background-work-chip.tsx`.
- Shared overview/phone work-surface components extracted by Task 01.
- `apps/web/components/task/chat/background-work/background-work.test.tsx`.
- `apps/web/components/task/chat/background-work/background-work-clear-window.test.tsx`.
- `apps/web/lib/background-work/workload-views.test.ts`.
- Listed background-work E2E suites and related focused helpers.
- `apps/web/src/locales/` catalogs.
- `docs/public/tasks-and-workflows.md` and `docs/public/agents-and-profiles.md`.
- This package and affected companion work-order test mappings.

## Dependencies

Task 01 supplies selectors, independent entries, and the shared phone surface.

## Risks

A clipped status or Open button can hide overflow without fixing usability.
HappyDOM cannot prove browser geometry. Do not substitute class assertions for
the required Chromium/phone checks. The owning draft pair covers broader
unfinished work and must not be promoted in full solely for this UX repair.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/background-work.md), REQ-005.
- [Shared design](../../specs/agents/system-design/background-work.md#shared-client-and-responsive-ui).
- [Clear-window regression contract](../background-work-clear-window/plan.md).

## Results

Completed 2026-11-03.

- Extracted the shared overview to `background-work-overview.tsx`. It uses
  `@kandev/ui/tabs`, shows localized Active/History counts before search,
  filters only the selected tab, resolves the initial tab after loading, and
  keeps the user's selection through live membership changes and empty lists.
- Added localized loading, per-tab empty, and no-match states. Unknown records
  keep their actual status in History and can return to Active.
- Reworked the desktop cards, summary rows, and detail headers with `min-w-0`
  tracks and nonshrinking status/Open regions. Compact titles truncate with a
  full accessible title; details show the complete command with
  `overflow-wrap:anywhere`.
- Preserved phone list tab/search/scroll state across detail Back and kept the
  independent Panels entry's draft and focus behavior. The overview and detail
  each own one vertical scroller and contain the document width at 320px.
- Updated the public inspection instructions in `tasks-and-workflows.md` and
  `agents-and-profiles.md`, and reconciled both companion work orders with the
  superseding package.

Verification:

- The Task 02 focused Vitest set, including the retained clear-window suite and
  the deferred first-read regression: 10 files, 144 tests passed.
- `pnpm exec tsc --noEmit` (with an 8 GB Node heap): passed.
- `pnpm run i18n:check`: passed; all seven catalogs complete.
- Scoped ESLint over every changed TS/TSX path: 0 errors.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `python3 scripts/list-docs.py validate`: 428 decisions and 1902 specs.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: clean.

Browser checks:

- New desktop viewer E2E (`chromium`): 4 tests passed with real backend, real
  WebSocket updates, measured document/panel overflow, a 320px fine-pointer
  viewport proving 44px chip and summary targets, and a command taller than the
  detail viewport proving the command end, status/actions, and output remain
  reachable while scrolling.
- New phone viewer E2E (`mobile-chrome`): 3 tests passed, covering the Panels
  entry, 320px containment, 44px Back target, full-height overview/detail,
  live updates, summary dismissal, focus restoration, and draft retention.
- Adapted OpenCode and process-end suites passed on both projects, and the
  phone cancel-turn suite passed with the running-only chip.
- Screenshots: `background-work-viewer-history`,
  `mobile-background-work-history`, and `mobile-background-work-overview`
  captured through the E2E `prCapture` fixture.

### Review fixes

- Added explicit `TabsContent` panels with matching values, so every selected
  tab owns a linked `role=tabpanel` with `aria-labelledby`. Added role,
  `aria-controls`, and keyboard-navigation tests.
- Moved the complete command into the shared detail scroll region on desktop
  and phone. The fixed header keeps navigation, status, and actions visible.
- Added the summary-to-detail Back fallback: when the inspected job leaves the
  running summary, Back selects the inspected record's Active or History tab
  and restores focus to its row. Overview-origin tab/search/scroll state remains
  untouched.
- Tracked a failed first read as an explicit localized error with Retry, so an
  unverified empty list is never presented as history.
- Added capability-gated Stop and Interrupt controls to the phone detail and
  row-specific accessible names for every Open action.


