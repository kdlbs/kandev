---
created: 2026-10-27
status: done
requirements:
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
system_design:
  - ../../specs/ui/system-design/sidebar-task-row-presentation.md
legacy_specs: []
---

# Implementation plan: Hide sidebar change-request status

## Overview

Selecting Nothing for a saved view's right-side task-row content still renders
change-request indicators beside the title because the title fallback only
excludes the change-request-status trailing selection. Suppress the fallback
for Nothing too, preserving other choices and separately configured details.

## Scope and technical approach

Update `TaskItemContent` in `apps/web/components/task/task-item.tsx` and verify
GitHub, GitLab, and fallback status, other title badges, number details, menu,
and switching between selections. Existing saved-view persistence is unchanged.
The desktop filter popover and mobile inset drawer share the row and view state.

## ASCII UI preview

UI-01: Task row with Right side set to Nothing, desktop and phone
(`AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.17`). The task title remains the primary
navigation target; the phone keeps its visible touch action. Spacing is illustrative.

```text
Before: [state] Task title [PR status]  [actions]
After:  [state] Task title              [actions]
        [optional PR number and other details]
```

## Tests

A focused `task-item-hidden-status.test.tsx` regression exercises the real row through saved
presentation changes, checks both PR and MR, and retains number and menu.
Desktop and mobile sidebar filter Playwright tests prove selection and preview.

## Work orders

- [x] [Task 01: Suppress hidden status](task-01-hide-status.md)

## Verification results

Component regression: passed (1). Existing task-item suite: passed (49).
Desktop and mobile managed Playwright scenarios: passed (1 each). The existing
desktop saved-view scenario passed with retries disabled after its persistence
assertion was changed to wait for the confirmed save.
Specification validation and lint: passed. TypeScript check with an 8 GiB Node heap: passed.
Public documentation validation:
63 tests and 54 pages passed. Focused frontend lint: passed.

## Risks

Provider status icons must remain visible for existing Git changes and relative
time choices and move to the trailing position for change-request status.
