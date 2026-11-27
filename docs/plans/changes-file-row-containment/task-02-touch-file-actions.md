---
id: "02-touch-file-actions"
title: "Keep touch file actions available"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-002
acceptance_criteria:
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.2
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-002.5
system_design:
  - ../../specs/ui/system-design/changes-file-row-containment.md
---

# Task 02: Verify touch file actions and confirmation

## Scope

Keep the shared action menu and its existing touch, keyboard, and mouse behavior.
Wait for finite menu animations before browser measurements or action selection.
Open single-file confirmation after the menu focus scope closes.
Remove the old inline tablet confirmation so the action trigger remains mounted.
Use the compact confirmation Drawer for coarse-pointer file actions at every width.
Keep the fine-pointer anchored popover and bulk deletion behavior.

## Verification

- Confirmation unit tests cover opt-in tablet presentation and presentation changes.
- Phone and 820px touch browser cases cover cancellation, focus return, deletion,
  unchanged row geometry, and reachable controls.
- The mobile symlink case covers menu visibility and successful Edit selection.

## Results

The imported custom pointer-up toggle was removed during consolidation review.
Its new unit case passed against unchanged main, without proving a production defect.
Two confirmation unit cases failed against main and passed with the adapter fix.
The imported tablet path grew a 44px row to 194px and rendered duplicate controls.
A regression test proves that the corrected row keeps its action trigger.
Browser results are recorded in the replacement PR validation section.
