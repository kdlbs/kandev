---
created: 2026-08-26
status: implemented
requirements:
  - REQ-UI-RESPONSIVE-PLAN-FORMATTING-001
system_design:
  - ../../specs/ui/system-design/responsive-plan-formatting.md
legacy_specs: []
---

# Implementation Plan: Dock Mobile Plan Formatting Controls

## Overview

Replace the Plan editor's selection-anchored formatting bubble on phone and
touch-tablet layouts with a compact fixed strip above the software keyboard,
shown only for a focused non-whitespace text selection. Preserve the current
desktop bubble, formatting commands, Plan comment handoff, and plan data flow.
Implement the correction as one vertical TDD work order so both the component
regression and mobile browser outcome fail before production code changes and
pass together afterward.

The confirmed root cause is `PlanBubbleMenu`: every non-empty selection mounts
Tiptap's selection-anchored `BubbleMenu` with top placement on every viewport.
Android and iOS independently place native text-selection actions around the
same selection, outside Kandev's DOM stacking order. Changing placement,
flipping, or `z-index` cannot reliably separate them.

## Scope

### In scope

- Preserve the current selection bubble on compact and full desktop layouts.
- Render a selection-driven docked formatting strip in phone and touch-tablet
  Plan layouts, with a compact 48-pixel bar and 32-pixel visual action surfaces
  around 44-pixel touch targets.
- Share active-mark state, command handlers, link mode, and Plan comment
  selection behavior across both presentations.
- Reuse the terminal keybar's visual-viewport positioning behavior through a
  shared pure helper.
- Reserve editor space, clear mobile task navigation and safe areas, provide
  44-pixel touch controls, and contain horizontal overflow within the strip.
- Add localized toolbar accessibility copy, focused component coverage, and a
  production-build `mobile-chrome` Plan formatting scenario.

### Out of scope

- Suppressing or customizing native mobile text-selection actions.
- Backend, API, database, plan autosave, revision, or WebSocket changes.
- New formatting commands or a redesign of the Plan comment composer.
- Changes to public plugin editor props or plugin-specific comment support.
- A generic rewrite of Tiptap menus, drawers, or mobile task navigation.

## Technical approach

### Responsive formatting surface

- Extend `TaskPlanPanel` and `TipTapPlanEditor` with an internal presentation
  input selected from `useResponsiveBreakpoint` and the mobile layout's existing
  bottom-navigation height.
- Refactor `PlanBubbleMenu` so its controls and reactive Tiptap state are shared.
  Keep BubbleMenu selection anchoring only for desktop; render the docked
  surface for phone and tablet.
- Subscribe separately to editor focus/blur and Tiptap transactions. Show the
  mobile surface only for a focused non-whitespace text selection, keep it
  mounted while its link input has focus, and hide it in code blocks.
- Preserve editor selection with pointer-down and mouse-down prevention before
  running existing command chains.

### Viewport geometry and scroll ownership

- Extract the terminal keybar's pure open/closed keyboard position resolver
  beside `useVisualViewportOffset`; parameterize bar height and base bottom
  offset.
- Keep `MobileTerminalKeybar` on the extracted helper as a behavior-preserving
  regression consumer.
- Position the open-keyboard Plan strip from `viewportBottom - barHeight` and
  the closed-keyboard strip above task navigation plus the safe area.
- Add scoped Plan-editor bottom space while the strip is visible. The Plan
  editor remains the single vertical scroller; only the toolbar row scrolls
  horizontally.

### Accessibility and localization

- Reuse existing localized button labels and add one localized toolbar name in
  `editors.json` for English, Portuguese, Simplified Chinese, and both
  Traditional Chinese catalogs. The scoped Traditional Chinese converter was
  used because the full catalog command is currently blocked by unrelated
  pre-existing residual keys in `agents.json`.
- Expose toolbar semantics, action names, pressed states, compact 48/32-pixel
  visual geometry, and minimum 44-pixel touch geometry.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.1` | `plan-bubble-menu.test.tsx` proves the fine-pointer selection bubble and code-block exclusion remain. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.2` | Component and mobile browser RED/GREEN coverage proves a focused non-whitespace selection renders the docked strip while a caret or whitespace-only selection keeps it hidden. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.3` | Component tests cover shared actions and active states on the eligible-selection surface; the hidden empty-selection state prevents selection-only actions from being presented. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.4` | Source regression asserts no selection/callout suppression; physical Android/iOS acceptance covers native chrome. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.5` | Shared helper/unit tests and mobile browser viewport resize/scroll assertions cover keyboard-edge tracking. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.6` | Component positioning/padding coverage and mobile task-nav geometry assertions cover clearance and final-line reachability. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.7` | Component accessibility assertions and mobile bounding-box/overflow checks cover selection-driven visibility, names, states, 44-pixel touch size, compact 48/32-pixel sizing, and containment. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.8` | Component pointer-event coverage and mobile Bold flow prove focus, selection, one command, and continued editing. |

The component regression covers mobile Plan formatting outside the selection
geometry. It fails before implementation because the current component always
mounts a Tiptap BubbleMenu. The browser regression is `docks above the keyboard
and preserves the selection for Bold`. It fails before implementation because
no docked toolbar exists.

## E2E tests

Add `apps/web/e2e/tests/task/mobile-plan-formatting-toolbar.spec.ts` for the
`mobile-chrome` Pixel 5 project:

1. Seed Plan content through the existing mock MCP plan-writing pattern and
   open the mobile Plan tab.
2. Focus the real ProseMirror editor and assert that a caret does not mount the
   dock. Select a known phrase and assert the compact docked toolbar replaces
   the selection bubble.
3. Simulate visual-viewport keyboard resize and scroll using the established
   terminal-keybar event pattern. Assert the toolbar follows the visible bottom
   edge and clears mobile navigation.
4. Assert the 48-pixel toolbar, 32-pixel visual action surfaces, 44-pixel
   action targets, internal horizontal containment, and no document-level
   horizontal overflow.
5. Tap Bold and assert the known phrase is formatted and the editor remains
   ready for input. Component coverage verifies that the command runs once.

The test does not claim to display native Android or iOS menus. Record manual
device evidence separately when those devices are available.

## Work orders

- [x] [Task 01: Dock responsive Plan formatting controls](task-01-dock-responsive-plan-formatting.md)

## Verification results

Implemented in Task 01. Focused unit coverage, production-build mobile E2E,
type checking, changed-file lint, i18n validation, spec lint, and whitespace
checks pass. Review follow-up also covers keyboard occlusion through the final
editor line, constrains the tablet dock to the Plan pane, hides the dock for
caret and whitespace-only selections, and keeps its compact visual controls
inside accessible touch targets. Fresh desktop and mobile captures were
produced for the PR. Native Android Chrome and iOS Safari checks were not
available; Playwright mobile emulation covers the automated geometry and
selection contract.

## Risks

- Tiptap's React editor-state selector observes transactions but not focus
  events. Relying on it alone would leave selection-and-focus visibility stale.
- Toolbar taps can collapse a mobile selection unless both pointer-down and
  mouse-down focus transfer are prevented before the command runs.
- A fixed bar that ignores the mobile task-navigation offset can replace the
  native-menu collision with a bottom-navigation collision.
- Extracting terminal keybar positioning can regress terminal input geometry;
  its existing unit and mobile E2E coverage must remain unchanged.
- Playwright device emulation cannot render operating-system text-selection
  chrome. The automated contract must stay limited to Kandev geometry,
  selection preservation, and formatting outcome.

## Public documentation

None. This repair changes no public command, configuration, API, workflow, or
plugin prop contract.

## Decisions

No ADR is required. The package applies an established mobile viewport pattern
to an existing UI surface and does not establish a new ownership or public
contract boundary.
