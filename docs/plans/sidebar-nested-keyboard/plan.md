---
created: 2026-10-28
status: implemented
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-003
system_design:
  - ../../specs/tasks/system-design/task-keyboard-actions.md
legacy_specs: []
---

# Implementation Plan: Sidebar nested-control keyboard ownership

## Overview

Restore native keyboard activation of the sidebar task action button while
preserving task-row navigation and selection. One sequential work order first
establishes an independently authored rendered failure and control, then applies
and verifies the smallest row-boundary correction. Implementation was performed after the separate implementation instruction and exclusive
local heavy-resource grant.

## Scope

Own `TaskItem` keyboard handling, affected rendered regressions, desktop and
phone browser evidence, and this task's owning documents. Exclude menu redesign,
pointer gestures, unrelated menus, layout, copy, list loading, state recovery,
and shared confirmation-component changes.

## Technical approach

The row receives bubbled Enter/Space from `TaskMenuButton`, calls
`preventDefault`, and dispatches selection before native button activation.
Guard nested native button origins at `handleTaskItemKeyDown`, before the
existing default prevention. Include descendants of the button, bound to the
current row; preserve row and noninteractive title origins. Keep the existing
`defaultPrevented`, repeat, modifiers and pending-removal behavior for row-owned
events. `dispatchSidebarRowClick` continues to own navigation and multi-select.
The same boundary naturally covers the actual coarse desktop inline archive
confirmation. Do not add handlers to the confirmation or menu primitives.

The phone `SessionTaskSwitcherSheet` -> `MobileTaskList` -> `TaskSwitcher` uses
the same row and action button; it supplies its own view state and task actions
without desktop selection handlers. The phone picker is an inset navigation
surface with a single task-list scroll owner; the visible overflow opens the
existing safe-area-aware menu sheet with touch-sized actions. The nearest
shipped exemplar is `mobile-pr-unlink-sidebar-menu.spec.ts`; the task title
opens the picker, and selecting a task remains its primary navigation action.
No geometry, scroll ownership, safe-area behavior, or touch target changes.
Non-phone coarse pointers can use inline simple archive confirmation; phones
use its dialog, and fine pointers use its popover. Focus return and dismissal
remain owned by existing surfaces.

## Existing surfaces

On desktop, the task row retains its trailing Task actions button and existing
menu. Currently, Enter/Space on that focused button activates the row and
cancels the button's native click. After the fix, it opens the menu without
changing the current task or selection; a row-focused key still activates the
row. On phone, the task title opens the existing picker, whose fixed header and
scrolling task list lead to the visible action and safe-area-aware action sheet.
On coarse non-phone layouts, a simple resolved archive confirmation presents
its existing Cancel and Archive buttons inside the row. Their keyboard actions
remain owned by those buttons. These surfaces and their existing copy do not
change. They map to AC-003.1 through .3 and the rendered checks below.

## Tests

Create `components/task/task-item-keyboard-ownership.test.tsx` using the real
`TaskItem` and real `TaskMenuButton`, `StateProvider` and `TooltipProvider`,
without product component mocks. Before the fix, assert for both keys that the
button-origin keydown does not select or prevent default: both must fail for
the observed reason. An explicit actual button click must still produce one
bubbling contextmenu and no row selection; row Enter is a passing control.
HappyDOM does not prove native keyboard-to-click synthesis. Also cover row
Enter/Space, a noninteractive title target, modifier forwarding, repeated and
already-prevented row events as existing behavior, active/multi-selected and
pending-removal states, and subtask toggle. Test the actual coarse non-phone
archive adapter and rendered confirmation buttons without inventing wrappers;
confirm Cancel and enabled Archive perform their real existing callbacks once
and do not select the row. Use actual adapter setup and record any fixture
limitation rather than substituting a mocked confirmation.

## E2E tests

Add `e2e/tests/task/sidebar-action-keyboard.spec.ts` and
`e2e/tests/task/sidebar-inline-archive-keyboard.spec.ts` in Chromium, and
`e2e/tests/task/mobile-sidebar-action-keyboard.spec.ts` in mobile-chrome. Seed
real tasks through existing isolated fixtures. From the visible row/picker entry,
reach the real action button by keyboard, then separately press Enter and Space;
assert the actual task menu opens, the route/current task and selection do not
change, and dismissal works. Retain a row-key navigation control; on phone also
use the real touch action as parity evidence. Do not substitute `click` or a
synthetic event for keyboard activation. Test one worker, zero retries, and
verify discovered and executed counts. The first GREEN managed run builds the
fixed production assets. A later `--no-build` run is allowed only after verifying
that production asset inputs and fingerprints are unchanged.

## Work orders

- [x] [Task 01: Restore row keyboard ownership](task-01-row-keyboard.md)

## Verification results

The real rendered HappyDOM regression failed before the fix for Enter and Space
(2 failed, 6 controls passed) and passed afterward (9 tests). Desktop Chromium
passed 2 tests, including the actual coarse inline archive path; mobile Chromium
passed 1 test. Both used one worker and zero retries, with real keyboard input.
The initial desktop run reached both menus but its later row-focus control failed;
the corrected focus sequence passed. The later runs used unchanged verified
production inputs and built-asset fingerprints. Scoped ESLint, 6144 MiB
TypeScript check, i18n check and ratchet passed. Public documentation needs no
change for this restored interaction.

## Documentation impact

`docs/public/tasks-and-workflows.md` is a task-oriented how-to and already
states that the task actions menu remains available; `docs/public/use-kandev.md`
describes the separate command-palette keyboard path. This repair restores an
existing native control, adds no command, terminology, layout, or procedure, and
does not require public documentation or screenshot changes. Reassess if the
implementation changes that premise. The owning requirements and design above
record the interaction contract.

## Risks

Native keyboard synthesis differs from HappyDOM; Chromium evidence is required.
Selection modifiers and pending-removal ordering are easy to regress. The
coarse archive path depends on resolved classification and pointer mode; the
test must prove it mounted the real inline surface. A phone touch-only test
cannot establish attached-keyboard behavior.
