---
created: 2026-10-09
status: completed
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
  - REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
  - ../../specs/integrations/system-design/github-pr-unlink-menus.md
legacy_specs: []
---

# Implementation Plan: PR popover refinements

## Overview

Require a short hover before a GitHub task PR summary opens.
Two sequential work orders cover hover timing, automation filtering, and inline tab unlink controls.
UI owns this reusable disclosure contract. Provider state and synchronization keep their existing owners.
Integrations owns the existing unlink contract and its tab control.

## Evidence and intent

The screenshot shows a PR summary over sidebar rows during pointer navigation.
`PRTaskIconView` enables the shared hook's hoverable mode.
`useTaskIconTooltipState` passes zero as the open delay and explicitly forces the helper open on pointer entry.
Its `onOpen` callback also starts hydration immediately.
This source trace explains the report. No browser reproduction ran in this design turn.

The proposed routine default is 500 ms of continuous pointer presence.
Movement within the small icon does not restart the interval.
This choice suppresses brief crossings while allowing deliberate access.
Visible keyboard focus and explicit touch activation remain immediate.

## Scope

In scope: GitHub task PR icons in the sidebar and their shared Kanban/list consumers,
plus the tab unlink controls in the multi-PR information popover and drawer.
The criteria are AC-UI-PR-TASK-STATUS-SUMMARY-001.26 through 001.29
and AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.8 through 001.10.

Out of scope: global tooltip timing, GitLab/plugin indicators, provider requests,
status colors, unrelated layout changes, pointer-speed detection, and user settings.
No ADR, runtime flag, new copy, or translation change is necessary.
Internal requirements, design, and delivery records document these UI refinements.
Public documentation remains unchanged because commands, settings, and user-facing terminology do not change.

## Technical approach

Add an optional delay to `useTaskIconTooltipState` and opt in from `PRTaskIconView`.
Reuse `useHoverPopover` for pending-open cancellation and the existing close grace period.
Separate delayed mouse entry from immediate visible keyboard focus.
Run hydration on an actual opening, using the latest callback once per disclosure.
Keep default callers and the existing drawer path intact.

The Automation section lists only open PRs with auto-fix or auto-merge enabled.
It is omitted when no open PR has either option enabled, and retains its loading
message while settings load. Fetch missing settings on disclosure even when the
full PR records are already cached.

The completed summary-scrolling and hydration packages remain historical implementation records.
This package supersedes their immediate mouse-opening behavior only.

For the second screenshot, `PRTab` places its active border on the label button.
Its always-visible unlink button is a sibling outside that border.
Task 02 moves the outline to their common wrapper and reveals the close control on hover or focus.
Both buttons remain semantic siblings, with reserved close-control space.
The existing action unlinks the task association. It does not close the remote PR.
The completed unlink-menu package remains the association contract's prior delivery record.

## Mobile design contract

Entry: the existing PR icon in the phone task picker opens `PRTaskIconDrawer` by tap.
That shipped drawer is the nearest exemplar and remains the temporary status surface.
Its fixed header precedes one scroll body with PR entries and automation details.
The drawer retains its 80dvh limit, safe-area handling, touch target, dismissal, and focus return.
Desktop and phone share the same summary and hydration data.
The row body retains task navigation.

The multi-PR information surface uses the existing `PRStatusChipDrawer` on phones.
Its tab row has a local horizontal scroller above the selected PR details.
Close controls remain visible and touch-sized on phones and coarse pointers.
The selected tab and unlink state remain shared with the desktop popover.

## ASCII UI preview

UI-01: Desktop sidebar, mouse entry onto a PR icon.

```text
Before: Task [PR] -> immediate summary
After:  Task [PR] -> 0..499 ms: sidebar stays clear
                    500 ms: [PR title / CI / Merge]
                    early exit: cancel opening
Keyboard focus:   -> immediate summary
```

UI-02: Phone task picker, explicit PR tap.

```text
Task title                         [PR]
                                     | tap
       +-----------------------------+
       | Pull requests               | fixed header
       | PR title / CI / Merge       | one scroll body
       | Automation                  |
       +-----------------------------+ safe-area clearance
```

These views require the timing and input behavior in AC 001.26 through 001.28.
Labels are illustrative. Existing localized content and geometry stay in place.

UI-03: Multi-PR information tabs, desktop fine pointer.

```text
Before: [ repo #3660 ] x   [ plugin #10 ] x
Idle:   [ repo #3660   ]   [ plugin #10   ]
Hover:  [ repo #3660 x ]   [ plugin #10   ]
Focus:  [ repo #3660 x ]   [ plugin #10   ]
Busy:   [ repo #3660 o ]   [ plugin #10   ]
```

The outline includes both controls. The reserved trailing space keeps widths stable.
The same reveal rule applies to active and inactive tabs. `o` represents pending feedback.

UI-04: Phone PR-status drawer.

```text
+---------------------------------------+
| Pull requests                         | fixed drawer header
| [ repo #3660  x ] [ plugin #10  x ] -> | tab-row scrolling
| Selected PR details                   | existing body scroll
+---------------------------------------+ safe-area clearance
```

UI-03 and UI-04 cover unlink criteria 001.8 through 001.10.
Close targets remain visible and at least 44 by 44 pixels on phones/coarse pointers.
Control containment, stable width, and input access are required. Labels are illustrative.

## Tests

- `use-task-icon-tooltip-state.test.ts`: add `delays opted-in pointer disclosure until 500 ms` as the initial failing regression.
- Cover early exit, repeated entry, independent instances, keyboard focus during a pending timer, Escape, and unmount.
- Preserve zero-delay default behavior and open-content pointer/focus continuity.
- `pr-task-icon.render.test.tsx`: prove that brief hover neither opens nor hydrates, and deliberate opening hydrates once.
- Update GitHub icon tests that assume immediate mouse disclosure. Run the focused hook, automation, render, hydration, and unlink suites listed in the work orders.
- `pr-ci-popover.automation.test.tsx`: retain unlink identity, pending, failure, adjacent focus, and collapse coverage.

## E2E tests

- Chromium, `pr-sidebar-hover-hydration.spec.ts`: add `ignores brief crossings of sidebar PR icons` (AC 001.26 and 001.28).
- Cross at least two seeded icons, then remain on one. Assert no intermediate disclosure and subsequent readable details.
- Use controlled browser time for temporal assertions. Avoid wall-clock sleeps and screenshot-only evidence.
- Retain the existing long-summary keyboard, pointer transfer, scrolling, and Escape scenario (AC 001.27).
- Mobile Chrome, `mobile-pr-sidebar-automation-indicators.spec.ts`: retain tap-to-drawer and long-summary reachability (AC 001.27).
- Save desktop and phone captures during these focused checks.
- Chromium, `pr-multi-popover.spec.ts`: measure tab/close containment, hover/focus visibility, stable width, and unlink behavior.
- Mobile Chrome, `mobile-pr-ci-chip.spec.ts`: extend the existing multi-PR unlink scenario with shared-outline containment and target width.
- Exercise the tab control at phone fine-pointer width and desktop coarse-pointer width as well as normal desktop and phone inputs.

## Work orders

- [x] [Task 01: Delay task PR hover disclosure](task-01-delay-pr-hover.md)
- [x] [Task 02: Place unlink controls inside PR tabs](task-02-inline-pr-tab-close.md)

## Verification results

Design checks passed on 2026-10-09:

- Catalog validation: 368 decisions and 1496 specifications.
- Specification linter tests: 36 passed.
- Full specification lint and `git diff --check`: passed.
- Local PR-documentation coverage preflight: covered, with no errors.

The preflight supplied the planned hook path as a simulated runtime change.
It read local documents without contacting GitHub.
The expanded package passed the same checks on 2026-10-09, including both work orders in the local coverage preflight.

Implementation and verification passed on 2026-10-09:

- Task 01 delays pointer disclosure for 500 ms, preserves immediate keyboard and touch entry, hydrates on actual opening, scopes cached PRs to the current workspace generation, and filters Automation to open PRs with an enabled option.
- Task 02 places unlink controls inside the shared tab outline, reserves their space, reveals them on desktop hover/focus, keeps pending feedback visible, and provides visible 48 by 48 pixel controls on phone/coarse-pointer inputs.
- Focused Vitest coverage: 9 files and 135 tests passed. `pnpm run typecheck`, targeted ESLint, `pnpm run i18n:ratchet`, and `pnpm --filter @kandev/web build:vite` passed.
- Desktop PR E2E: 14 tests passed. Mobile PR E2E: 12 tests passed. The long-summary hydration case passed again after its evidence assertions were extracted.
- Catalog validation, full specification lint, and `git diff --check` passed. Desktop and phone captures were produced by the focused browser checks.

PR fixup verification on 2026-10-09:

- Restored the Automation loading message for missing settings, including cached PR records; hide the section after disabled settings resolve.
- Escape cancels a pending mouse opening without fetching details. Later keyboard focus still opens immediately. Pending listeners are removed on exit and unmount.
- Browser hydration evidence polls the store after responses instead of reading it once.
- Focused Vitest: 10 files, 140 tests passed. Typecheck, targeted ESLint with zero warnings, and i18n ratchet passed.
- Fresh managed Chromium sidebar checks: 5 passed. Mobile drawer checks: 4 passed, including deferred settings loading and disabled-section omission.
- Catalog validation, full specification lint, and `git diff --check` passed.
- These fixes enforce the existing disclosure contract; no new requirement or public-documentation change is needed. Phone composition remains the shipped drawer with shared status data.
- Remote CI and review verification remain pending until the remediation is pushed and the latest head reaches terminal checks. The walkthrough generator currently fails because its trusted base workflow names a model unavailable to OpenCode.

Implementation details and per-work-order results are recorded in the task files.

## Risks

- Changing only the delay value leaves the forced opening in place.
- A callback effect can duplicate hydration after rerenders unless it detects opening transitions.
- Shared default changes can affect non-PR indicators.
- Real-time timing assertions can be flaky. Controlled timers prove the exact boundary.
- Nested buttons break keyboard semantics. Keep the visual tab wrapper separate from both buttons.
- Opacity alone can leave an invisible pointer target. Pair visibility with pointer and focus behavior.
