---
created: 2026-09-30
status: complete
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
legacy_specs: []
---

# Implementation Plan: PR task summary scrolling

## Overview

A task with five PRs can show a summary taller than the viewport.
The desktop tooltip has no height limit or scroll owner.
It also ignores pointer events and closes when the pointer leaves its trigger.
The follow-up review also identified tooltip description and keyboard-focus
visibility gaps. One sequential work order corrects containment, input behavior,
and accessibility.

UI owns this repair because the existing shared task-summary contract owns disclosure presentation.
Provider state, associations, and hydration remain under their existing owners.

## Evidence and intent

The supplied screenshot shows clipped PR entries above the viewport.
`PRTaskIconTooltip` sets width and padding only.
The Tooltip primitive sets `pointer-events-none`.
`useTaskIconTooltipState` immediately clears hover on trigger leave.
These source facts explain both clipping and the missing usable scroll path.
No local browser reproduction ran during this design turn.

The smallest reproduction is five associated PRs with wrapped titles in a short desktop viewport.
Hover the sidebar PR icon, then attempt to reach the last entry with the mouse wheel.
The regression must fail on actual containment or scrolling before the correction.

## Scope

In scope: the GitHub task-icon tooltip shell, hover/focus continuity, and desktop/phone regression coverage.

Out of scope: provider status logic, PR associations, APIs, polling, global Tooltip defaults, and unrelated provider disclosures.
No new product copy, runtime flag, or ADR is necessary.
This design package changes internal documentation only.

## Technical approach

- Bound the outer tooltip by Radix available height and dynamic viewport height.
- Place summaries and automation details inside one keyboard-focusable scrolling body.
- Name the scroll body as a region, preserve rendered PR details in the Tooltip description, and show a visible keyboard-focus indicator.
- Enable pointer input only on this tooltip. Keep the arrow outside the scrolling body.
- Add an opt-in hoverable mode to `useTaskIconTooltipState` through its existing provider-neutral alias.
- Reuse `useHoverPopover` for trigger/content presence and a short gap delay.
- Preserve immediate trigger disclosure, hydration deduplication, focus-visible filtering, and Escape behavior.
- Keep other callers on their existing default behavior.

The original summary, row-presentation, and hover-hydration packages are complete.
This package extends them without reopening their completed work orders.

## Mobile design contract

Entry: open the phone task picker and tap the existing PR indicator.
The nearest exemplar is `PRTaskIconDrawer`, already covered by mobile sidebar automation tests.
The drawer suits a temporary read of status summaries and preserves task navigation.
Its fixed header precedes PR entries and automation detail.
Its body remains the single scroll owner, with dynamic height and shared safe-area handling.
The existing 44-pixel icon target, dismissal, and focus return remain in place.
Desktop and phone reuse the same summary data and hydration.

## ASCII UI preview

UI-01: Desktop sidebar PR hover, five linked PRs.

```text
Before:                       After, within viewport:
[ PR 1 above screen ]         +------------------------+
| PR 2 title / status |       | PR 1 title / status    |
| PR 3 title / status |       | PR 2 title / status  # |
| PR 4 title / status |       | ...                  # |
| PR 5 title / status |       | PR 5 after scroll     |
+--------------------+       | Automation after scroll|
                             +-----------v------------+
Sidebar: Task [PR 5]          Sidebar: Task [PR 5]
```

The outer shell stays inside the viewport. The `#` marks the internal scrollbar.
The same body supports pointer and keyboard scrolling.
A short summary keeps its natural height without an active scrollbar.

UI-02: Phone task picker, PR indicator tapped.

```text
+----------------------------+
| Task picker                |
| Task title          [PR 5] |
|                            |
| +------------------------+ |
| | 5 pull requests        | | fixed header
| | PR 1 title / status  # | |
| | ...                  # | | one scroll body
| | PR 5 after scroll      | |
| | Automation            | |
| +------------------------+ | safe-area clearance
+----------------------------+
```

## Tests

Extend `use-task-icon-tooltip-state.test.ts` for opt-in gap continuity,
content focus, Escape, unmount cleanup, and unchanged default mode.
Extend `pr-task-icon.render.test.tsx` for focus transfer and hydration continuity.
jsdom class assertions alone do not prove scroll behavior.

## Component tests

- `pr-task-icon.render.test.tsx`: verify that the Tooltip description retains PR details and its named scroll region is discoverable (AC 001.24).

## E2E tests

- `pr-sidebar-hover-hydration.spec.ts`, chromium: add five-PR overflow, wheel, keyboard, and short-summary cases (AC 001.21 and 001.22).
- `pr-sidebar-hover-hydration.spec.ts`, chromium: verify the keyboard focus indicator on the scroll region (AC 001.25).
- `mobile-pr-sidebar-automation-indicators.spec.ts`, mobile-chrome: add five-PR drawer reachability and containment (AC 001.23).
- `pr-status-badge.spec.ts`: keep existing automation and summary selectors aligned with the scroll-body wrapper.
- Measure tooltip bounds at a short viewport and at 767px/768px fine-pointer widths.
- Require actual positive scroll movement, final-entry containment, and zero document horizontal overflow.
- Capture the desktop overflow panel and phone drawer after scrolling.

## Work orders

- [x] [Task 01: Make long PR summaries scrollable](task-01-scroll-summary.md)

## Verification results

Implementation checks passed. Exact commands and results are in Task 01.

Design checks passed on 2026-09-30:

- Specification catalog validation: 337 decisions and 1271 specifications.
- Specification linter tests: 36 passed.
- Full specification lint and `git diff --check`: passed.
- Local PR-documentation preflight: the planned tooltip change has valid work-order coverage.

The coverage preflight used the planned runtime path as a simulated changed file.
It did not change that source file or contact GitHub.

## Risks

- CSS overflow alone leaves the pointer path unusable.
- An outer overflow rule can clip the tooltip arrow.
- A shared hook change can alter unrelated indicators unless the mode is opt-in.
- Focus transfer must preserve Escape dismissal and avoid duplicate hydration.
