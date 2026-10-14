---
id: "12-task-menu-recovery"
title: "Keep closed task menus closed during processing"
status: done
wave: 8
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-ACTIONS-MENU-004
acceptance_criteria:
  - AC-TASKS-TASK-ACTIONS-MENU-004.2
system_design:
  - ../../specs/tasks/system-design/task-actions-menu.md
---

# Task 12: Keep closed task menus closed during processing

## Summary

PR #4368's managed-task command test closed a stale deletion confirmation,
but the body retained `pointer-events: none`. Its retry passed; the flake gate failed.
The card forced its dropdown open whenever delete or archive processing started,
even after the user had closed that menu to enter a confirmation dialog.

## Scope and existing contract

Keep the controlled menu's open state independent of mutation progress.
Retain processing guards for a menu that the user explicitly opens.
Preserve menu entries, confirmations, request validation, retries, and timeouts.
The existing task-action design closes the menu on a terminal selection.
No new API, copy, configuration, or interaction surface is required.

## Desktop and phone interaction

Both paths use the card's visible More options control and existing confirmation.
The phone exemplar is the shipped focused Kanban column and its touch-sized card menu.
The dialog keeps its current hierarchy, scroll owner, containment, and safe-area behavior.
The mutation state is shared; the phone still uses touch activation.

```text
Desktop: card > More options > Delete > confirmation > rejection > usable card
Phone:   focused column > card menu > Delete > confirmation > rejection > usable card
```

## Files and checks

- `apps/web/components/kanban-card-actions.tsx`: remove progress-driven reopening.
- Its processing regression: a closed menu stays closed; an open busy menu stays mounted.
- Existing managed-task command E2E: stale confirmation preserves both tasks; fresh cascade deletes both.
- New phone E2E: touch the same card path, recover from rejection, and complete cascade deletion.
- Focused component tests, ten desktop repetitions, phone repetitions, typecheck, lint, and formatting.

## Evidence

Hosted run 37944513912, attempt 1, shard 11, job 113888420064 captured the pointer lock.
Ten local baseline repetitions reproduced that lock six times and passed twice.
Two other attempts failed during backend/fixture startup and are not pointer-lock evidence.
Two closed-menu component cases failed before the fix; the two open-menu cases passed.
All four new cases and 39 existing menu/dialog cases pass after the fix.
After a fresh managed build, all ten desktop repetitions passed without retries.
All three phone repetitions passed without retries, including stale-preview recovery,
touch activation, cascade deletion, and document-overflow checks.
Typecheck, focused ESLint, formatting, catalog validation, and specification lint pass.
Hosted verification remains pending the remediation push.
