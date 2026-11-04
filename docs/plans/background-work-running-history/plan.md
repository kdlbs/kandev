---
created: 2026-11-03
status: completed
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-005
system_design:
  - ../../specs/agents/system-design/background-work.md
legacy_specs: []
---

# Implementation Plan: Running background work and history

## Overview

Make the composer chip describe running work only. Separate current work from
history in the full overview and contain long commands. Two sequential work
orders deliver entry/filtering first, then tabs and command layout.

Agents owns workload observations and the existing shared inspection contract.
Update that requirement/design pair rather than creating a second UI contract.
The user explicitly requested a running-only popover, history in the panel,
tabs, and readable long commands. Active is the panel label because waiting
work still needs a current-work destination. History includes unknown records
with their actual status. It does not imply completion.

## Scope

### In scope

- Running-only chip count and summary, with no composer history chip.
- Independent desktop panel-menu and phone Panels entries.
- Active and History tabs, search, empty states, and live membership updates.
- Truncated compact commands, wrapped complete commands in details, and
  contained status/actions at narrow widths.
- Localized copy, keyboard/focus behavior, and phone navigation/geometry proof.

### Out of scope

- Provider completion delivery, ACP categorization, backend accounting,
  lifecycle settlement, new APIs, persistence, flags, and job controls.
- Prompt admission, steering, output retention, and redesign of other panels.

## Technical approach

Add tested view selectors in `apps/web/lib/background-work/workload-views.ts`.
Use the accepted raw workload list from `useBackgroundWork` for running-only
summary membership, Active membership (`running`, `waiting`), and History
membership (terminal and `unknown`). Audit the existing hook count consumers
before changing any public return meaning. Do not reuse chip totals in activity
or admission decisions.

Filter `BackgroundWorkChip` before counting and rendering. Remove its history
variant and waiting/unknown badges. Preserve loaded detail data independently
of the running summary list. Handle removal of the last running job without
leaving an open orphaned summary or losing composer focus.

Add a feature-gated managed-session Background Work item in
`dockview-add-panel-items.tsx`. Open the existing overview action with an
explicit session ID. Extend the built-in actions in `PluginPanelPicker` and
`SessionMobileBottomNav` so phone inspection remains available without plugins.
Use a reusable phone work surface opened by both the chip and Panels entry.
The picker closes before that surface opens. Preserve explicit session identity
through overview-to-detail navigation and existing desktop tab activation.

Extract the overview from `background-work-panel.tsx` if needed so desktop and
phone share tabs, counts, search, and list data. Keep navigation callbacks
specific to each surface. Use `@kandev/ui/tabs`; default after the first fetch
settles, then preserve the user's selection. Counts precede search. Replace
the misleading overview output-search label with a job-search label.

Constrain card/header text tracks and ancestors with `min-w-0` and available
width. Compact commands truncate. Status and Open remain visible. Details show
the full command with wrapping that handles unbroken tokens. Measure panel,
card, drawer, and document overflow instead of relying on CSS class assertions.
Keep output clear/search behavior covered by its existing focused regression.

### Provider compatibility

| Provider or shape | Inspection behavior | Evidence and fallback |
| --- | --- | --- |
| Codex normalized shell/subagent | Running in chip; waiting in Active; outcomes in History | Shared selector/rendered tests; existing capabilities still control actions |
| Legacy OpenCode detached shell | Confirmed running launch can appear; unknown after settlement appears only in History | Existing OpenCode ACP fixture and updated desktop/phone E2E |
| Other ACP observations | Same normalized state rules | No provider-name switches; missing activity remains unknown |
| Feature disabled or passthrough | No new entry or chip | Existing gates and explicit negative rendered tests |

No new live-provider support is claimed. Deterministic fixtures prove UI behavior.

## ASCII UI preview

Structural requirements: running-only summary, distinct overview tabs, visible
status/actions, complete wrapped command in detail, and one vertical scroller.
Copy and spacing are illustrative. All rendered copy uses translations.

UI-01: Desktop composer summary. Entry: running chip. AC-005.1, .7, .8.
The current popover mixes ended/unknown rows under an active count. Replace it:

```text
Before                         After
[5 Background Jobs]            [2 Background Jobs]
  0 running / 5 unknown          Running work (2)
  Ended command...               build --watch... [Running] [Open]
  Unknown command...             checks --watch... [Running] [Open]
                                 [View all work]
```

No running jobs: no composer chip. Desktop `+ > Background Work` opens the
overview even when all records are unknown or terminal. No history popover.

UI-02: Desktop overview and detail. Entry: View all or panel menu.
AC-005.3, .6, .7, .9.

```text
+--------------------------------------------------------+
| Background Work                                        |
| [Active (2)] [History (5)]       [Search jobs...]         |
|--------------------------------------------------------|
| build --watch...              [Running] [Open]           |
| long/unbroken/command/...     [Waiting] [Open]           |
|                 one vertical list scroller              |
+--------------------------------------------------------+
Detail: full command wraps onto additional lines.
Status and actions occupy their own visible region.
```

UI-03: Phone, same state and outcomes. Entry: running chip opens an inset
summary. Panels > Background Work or View all opens the full-height work surface.
AC-005.3, .4, .6-.9. Reuse `MobilePickerSheet` anatomy and the shipped background
detail drawer. This depth of inspection needs a full-height surface.

```text
Inset summary                 Full-height overview
+-------------------------+   +---------------------------+
| Running work (2) [Close] |   | Background Work    [Close] |
| build... [Running] [Open]|   | [Active (2)] [History (5)]|
| [View all work]          |   | [Search jobs...]          |
+-------------------------+   |---------------------------|
                              | command... [State] [Open]|
                              | one list scroll region    |
                              |                           |
                              +------- safe area ---------+
Full-height detail
+---------------------------+
| [Back] Job         [Close] |
| complete command that     |
| wraps including long/path |
| [Unknown]                 |
|---------------------------|
| one content scroll region |
+------- safe area ---------+
```

Tabs/search/header remain fixed. Overview/detail are successive screens, not
stacked overlays. Back restores the tab, search, and scroll position. Close
returns focus to the originating trigger, or the composer if the chip vanished.
Use dynamic viewport height, safe-area clearance, and 44px phone/coarse-pointer
actions, including a narrow fine-pointer phone. Keep ordinary desktop controls
28px. Do not overwrite saved desktop layouts or composer drafts.

UI-04: Loading and empty states, both viewports. AC-005.4, .6, .8, .9.

```text
Loading: header + loading indicator; initial tab not resolved yet.
Active empty: [Active (0)] [History (5)] -> No active background work.
History empty: [Active (2)] [History (0)] -> No background work history.
Search empty: selected tab retained -> No matching background work.
Unknown only: chip absent; independent entry initially opens History.
```

Keep an already selected tab when its last item leaves. Close only a running
summary whose list empties. An open overview or detail remains inspectable.

## Tests

- `workload-views.test.ts`: every RunState, mixed kinds, unique accepted rows,
  empty and unknown-only lists. AC-005.1, .6.
- `use-background-work.test.ts` and `background-work.test.tsx`: chip count/rows,
  independent entry, initial loading selection, tabs/search, unknown status,
  live transitions, session identity, and focus. AC-005.1, .3, .6, .8, .9.
- `dockview-add-panel-items.test.tsx`, mobile picker/bottom-nav tests, and
  `dockview-background-work-panel.test.ts`: explicit session, repeated activation,
  and gates. AC-005.3, .9.
- Retain `background-work-clear-window.test.tsx` and its real-store assertions
  after changing the phone entry path. No output-clear redesign. AC-005.8.
- Repository locale checks cover all new tab/search/empty/accessibility copy.
  AC-005.5. Generate Traditional Chinese through `cd apps/web && pnpm run i18n:zh-hant`.

## E2E tests

Add `tests/task/background-work-viewer.spec.ts` (chromium) and
`tests/task/mobile-background-work-viewer.spec.ts` (mobile-chrome), using the
existing `recordBackgroundWorkload` fixture API and actual UI entries. Prove:

1. A mixed running/waiting/unknown/ended list shows only running jobs in the
   chip summary. Unknown-only and waiting-only sessions have no chip.
2. Independent entry reaches history without a chip. Tab counts, search,
   real-time state changes, open details, and draft retention work.
3. Long commands, including an unbroken token, do not create horizontal scroll
   in a narrow desktop panel, phone list, or detail. Status/Open stay contained.
4. Phone picker and full-height navigation, Back, close/focus fallback, 44px
   actions, one scroll owner, and viewport/safe-area containment work. Include
   widths just below/above 768px and a narrow fine-pointer viewport.

Update both existing OpenCode background-work specs to inspect unknown records
through the independent History entry. Update process-end helper assertions to
reopen history through the panel entry rather than a history chip. Audit the
mobile cancel-turn suite for running-chip assumptions. Run affected suites
sequentially with the guarded runner, not overlapping full suites.

## Work orders

- [x] [01: Running summary and independent inspection](task-01-running-summary.md)
- [x] [02: Tabbed history and contained commands](task-02-tabs-and-commands.md)

Executed 01, then 02 sequentially in one conversation.

## Verification results

Implementation and product checks completed 2026-11-03.

- Focused Vitest set (selector, hook, chip/panel, initial-read regression,
  clear-window, desktop menu, phone picker/bottom nav, panel actions): 10 files,
  144 tests passed.
- `pnpm exec tsc --noEmit` with an 8 GB Node heap passed; `pnpm run
  i18n:check` passed with all seven catalogs complete.
- Scoped ESLint over every changed TS/TSX path reported 0 errors.
- Desktop `chromium`: new viewer spec (4 tests, including a 320px
  fine-pointer viewport and a command taller than the detail viewport), adapted
  OpenCode spec (2 tests), and process-end spec (1 test) passed.
- Phone `mobile-chrome`: new viewer spec (3 tests), adapted OpenCode spec (1
  test), process-end spec (1 test), and mobile cancel availability (1 test)
  passed.
- Public docs, spec catalog, specification lint, and `git diff --check` passed
  with 63 documentation tests, 54 pages, 428 decisions, and 1902 specs.
- The browser runs use a real backend and WebSocket updates. The new specs
  capture `background-work-viewer-history`, `mobile-background-work-history`,
  and `mobile-background-work-overview` screenshots through `prCapture`.

Review remediation is recorded in both work orders: session-scoped first-read
readiness, managed-only phone availability with stale-selection fallback,
summary-detail Back to History with focus, bounded detail commands, and linked
tab panels with keyboard/ARIA coverage.



## Related delivery records

The existing [shared UI work order](../agent-background-work/task-05-shared-ui.md)
and [OpenCode viewer work order](../opencode-background-continuity/task-05-shared-viewer.md)
record prior delivery. Their all-state summary/history-chip assumptions are
superseded by this package. Keep their past results and statuses intact.
The [clear-window package](../background-work-clear-window/plan.md) remains the
authority for output-clear behavior; adapt its affected phone entry tests here.

Public-doc changes belong to implementation: update the inspection sections in
`docs/public/tasks-and-workflows.md` and `docs/public/agents-and-profiles.md`.
These are how-to instructions. Design-only edits do not change published UX.

## Risks

- Hiding the chip can strand history unless both independent entries ship with it.
- Unknown is not terminal. History placement must never rewrite lifecycle data.
- Filtering the running list must not remove an open detail or change admission.
- Focus return cannot target a chip that disappeared during a live update.
- Shared design contains older proposed infrastructure. Reuse current source
  seams and avoid expanding this presentation change into that pending work.
