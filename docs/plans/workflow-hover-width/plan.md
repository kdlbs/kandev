---
created: 2026-09-13
status: complete
requirements:
  - REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001
system_design:
  - ../../specs/ui/system-design/compact-workflow-step-navigation.md
legacy_specs: []
---

# Implementation Plan: Workflow hover width

## Overview

Widen the compact workflow hover so ordinary step names remain readable. One sequential work order covers the CSS correction and rendered checks.

## Scope

Include desktop disclosure width and regression coverage. Exclude move policy, backend changes, new copy, full-stepper changes, and phone navigation changes.

## Technical approach

The screenshot showed truncated labels beside fixed-size controls. `CompactWorkflowDisclosureSurface` now uses `w-[28rem]` (448px) instead of `w-72` (288px). `StepDisclosureRow` still gives the label remaining flex space and applies `truncate`; actions do not shrink.

Replace `w-72` with `w-[28rem]` in `apps/web/components/task/workflow-step-disclosure.tsx`. Preserve the viewport cap, collision handling, and touch Drawer branch. The existing UI specification owns this responsive disclosure; tasks retain movement policy ownership.

## ASCII UI preview

UI-01: Compact stepper hover, eligible step. AC-001.11 and AC-001.6 below refer to the full acceptance IDs in the work order.

```text
Before: [o I...  icons  options  Move here]
After:  [o Implementation  icons       options  Move here]
Phone:  [Task actions] -> [Move to] -> [step choices]
Tablet: [Current step v] -> [inset drawer with step choices]
```

Keep the row order and right-hand actions. Width is 28rem, capped to the viewport; ASCII spacing is illustrative.
The existing list owns vertical scrolling. Keep safe-area padding and touch targets in the existing drawer.
Phone navigation stays as shipped; the nearest exemplar is the existing compact disclosure Drawer and phone task actions.

## Tests

Run existing workflow-stepper component tests to check interaction semantics. Do not add a unit test that only repeats a CSS class.

## E2E tests

Extend `apps/web/e2e/tests/layout/task-topbar-workflow-stepper.spec.ts` with a test named `compact disclosure keeps ordinary step names readable` for AC-001.11. Seed Backlog and Implementation with capability icons and enabled move controls. Assert label scrollWidth does not exceed clientWidth, actions remain contained, and the dialog stays within the viewport. Cover English and pt-pt action widths.

Retain existing desktop keyboard and tablet Drawer checks for AC-001.6 and AC-001.3. Run the existing phone task-drawer move scenario for AC-001.9. Capture desktop and phone screenshots during these focused checks.

## Work orders

- [x] [Task 01: Widen compact disclosure](task-01-widen-disclosure.md)

## Verification results

Implementation and rendered checks passed. The desktop Popover uses `w-[28rem]` with the existing viewport cap; the touch Drawer branch is unchanged. The regression covers Backlog and Implementation labels, capability icons, action containment, English and pt-pt, and viewport containment.

Validation passed: `pnpm exec vitest run components/task/workflow-stepper.test.tsx` (20 tests), targeted ESLint, the desktop workflow-stepper E2E (3 tests), the phone task-drawer move E2E (1 test), and `git diff --check`. The host E2E run built the backend and pseudo-locale Vite bundle successfully. Documentation validation also passed: `list-docs.py validate` (267 decisions, 895 specifications), `lint-spec-files.py --all`, and `lint-spec-files.test.py` (36 tests).

## Risks

Long custom names may still truncate. Localized action labels consume more width; rendered checks must use real icons and controls.

## Documentation impact

Internal requirements and design updated. Public instructions and terminology remain accurate because only spacing changes.
