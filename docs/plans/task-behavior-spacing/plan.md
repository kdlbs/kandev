---
created: 2026-10-19
status: implemented
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-002
  - REQ-UI-SETTINGS-COMPOSITION-004
system_design:
  - ../../specs/ui/system-design/settings-composition.md
legacy_specs: []
---

# Implementation Plan: Task behavior spacing

## Overview

Remove duplicate group padding and keep Task behavior controls close to their labels.
One work order implements and checks the complete layout correction.
UI owns this change because settings composition is a reusable presentation contract.

## Evidence and assumptions

The supplied screenshot shows full-width groups, distant controls, and a tall profile row.
Source inspection establishes three causes:

1. `SettingsGroupContent` inherits Card `py-4`, then adds inner `py-2` and row edge padding.
   The first and last content edges therefore receive 32px instead of the intended 16px, excluding borders.
2. `TaskBehaviorSettings` has no width bound. The header and rows distribute controls across all available space.
3. The profile radio group stacks two cards inside a centered, right-aligned row control wrapper.
   Its height determines the entire row height and leaves the label isolated on the left.

Reproduction: open `/settings/preferences/task-behavior` on a wide desktop viewport with Tasks selected.
The screenshot and source establish the cause. No browser reproduction ran during this design turn.
The 64rem width and two-column desktop profile layout are proposed reversible design choices.
No setting value, label, save behavior, or navigation change is required.

## Scope

### In scope

- One padding owner for framed settings groups.
- A left-aligned, bounded Task behavior page, including its header and all panels.
- Profile choices below their label, with desktop columns and stacked phone choices.
- Focused rendered checks for geometry, wrapping, touch, and existing settings interactions.

### Out of scope

- Global Card styling, global settings width changes, new controls, and backend changes.
- New copy, translation changes, preference defaults, or save coordination changes.

## Technical approach

Apply the spacing details in the [system design](../../specs/ui/system-design/settings-composition.md).
Modify only `settings-group.tsx`, `task-behavior-settings.tsx`, and the profile setting component in production.
Use existing row class overrides instead of adding a general layout API.
Do not change `settings-tabs.tsx`, `SettingsPageHeader`, or `@kandev/ui` Card.
The shared group fix affects other framed groups, so include Appearance and Terminal and Editors in rendered checks.
The [original composition package](../settings-composition/plan.md) remains historical delivery evidence.
This package owns the new spacing assertions and does not reopen its completed work orders.

## Mobile design contract

Enter from the existing phone Settings index and Task Behavior link.
Reuse Data & Logs header tabs and the existing settings page shell for navigation and scroll ownership.
Keep title, tabs, group heading, setting explanation, and control in reading order.
Profile selection remains inline because it is a persistent preference with two visible choices.
Retain the existing info drawer for temporary explanations and the existing Save/Reset surface for pending changes.
The page remains the single vertical scroll owner. Preserve safe-area clearance and do not add a nested scrolling panel.
Below 768px, choices stack and controls retain phone targets of at least 44px, including narrow fine-pointer devices.
At wider coarse-pointer widths, retain the same minimum touch targets.
Share all draft, selection, and save state across viewport changes.

## UI preview

These structural previews map to AC-UI-SETTINGS-COMPOSITION-001.6, 002.7, 002.8, and 004.1 through 004.5.
Labels below identify existing translated controls. They are not replacement product copy.

| View | Proposed composition |
| --- | --- |
| UI-01: Desktop Tasks | Title and tabs share a bounded header. Creating/opening rows use consistent insets. The profile label spans the row above two equal option cards. Archiving uses the same group inset. |
| UI-02: Phone Tasks | Title, tab strip, and groups follow vertically. The profile label precedes two full-width stacked choices. The existing Save/Reset surface appears only for drafts. |

The 64rem cap, 16px group edge inset, and 8px choice gap are design values at the standard root font.
Text and selection cards grow when translations wrap. No fixed row height is required.
Inactive tabs, validation errors, and loading states retain their existing behavior.

## Tests

Run existing group, Task behavior, and profile component suites to protect state and semantics.
Do not add jsdom assertions that merely mirror CSS class names.

## E2E tests

Extend existing files rather than create a separate geometry framework:

| File under `apps/web/e2e/tests/` | Project and evidence |
| --- | --- |
| `settings/settings-composition.spec.ts` | chromium: computed group padding and rendered first/last insets on Task behavior and neighboring framed groups. AC-001.6. |
| `settings/task-behavior-concise-help.spec.ts` | chromium: 1920px page cap, all three tabs, profile alignment and equal columns at 768px, stacking at 767px, keyboard selection, drafts. AC-002.7/002.8, AC-004.2/004.5. |
| `settings/mobile-settings-composition.spec.ts` | mobile-chrome: existing shared-group and switch-edge activation checks. AC-004.1/004.4. |
| `settings/mobile-task-behavior-concise-help.spec.ts` | mobile-chrome: canonical phone, 767/768px boundary, 900px coarse-pointer, pseudo-locale containment, info focus return. AC-004.1 through 004.5. |
| `task/mobile-mcp-task-agent-profile-default.spec.ts` | mobile-chrome: profile selection, containment, target size, save/reload. AC-002.8, AC-004.3 through 004.5. |

All abbreviated AC suffixes above use the `AC-UI-SETTINGS-COMPOSITION-` prefix.
Use computed styles and bounding boxes with a 1px tolerance. Measure content edges excluding borders.
Capture desktop and phone screenshots during implementation and compare with UI-01/UI-02.

## Work orders

- [x] [Task 01: Correct settings spacing](task-01-correct-settings-spacing.md)

## Verification results

Design validation passed on 2026-10-19:

- `python3 scripts/list-docs.py validate`: 384 decisions and 1616 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `git diff --check`: passed.

Implementation validation passed on 2026-10-19:

- Pre-fix browser run: four expected geometry failures, with the existing phone stacking case passing.
- Focused Vitest command: three suites and 14 tests passed.
- Scoped ESLint command: passed.
- Desktop E2E command: 17 tests passed, including padding, all three tabs, keyboard profile selection, and Save/Reset.
- Mobile E2E command: 12 tests passed, plus two temporary screenshot capture cases.
- Local PR-documentation `validateCoverage` preflight: covered, with no errors.

E2E used `--host` with `KANDEV_SERVER_HOST=127.0.0.1` and the installed Nix Chromium library paths in `LD_LIBRARY_PATH`.
The first final run rebuilt the production assets. Subsequent runs used `--no-build` with those same unchanged assets.
One initial geometry assertion excluded form-only groups. The final test checks all framed groups.
Keyboard checks hold the arrow key until focus and selection settle, then release it.
These test corrections required no further production edits.

Screenshots use disposable E2E data from this branch, not the user's live instance.
Visual inspection passed for the 1024x612 desktop content capture and both 393x727 phone viewport captures.
Files are retained under `apps/web/.pr-assets/task-spacing/`: `desktop.png`, `phone.png`, and `phone-bottom.png`.
The final two capture cases passed. The temporary capture spec was removed, and no test backend remains running.
Public documentation needs no text change because labels, navigation, and settings behavior retain their existing meaning.

## Risks

- Shared group padding affects multiple settings pages. Check representative single-row and multiple-row groups.
- Runtime controls and long translations must remain usable within the page bound.
- Incorrect responsive overrides can leave the profile wrapper capped at half width or lose touch targets.

### PR review follow-up

Both Claude suggestions were applied: the private radio group now passes classes directly, and its row caller declares `grid` explicitly.
The existing rendered layout is unchanged. Focused verification passed: 4 component tests, scoped ESLint, a fresh Vite build, 3 desktop profile cases, and 5 phone/touch cases.
See the work order for commands. Remote CI and review completion are tracked on the PR.
