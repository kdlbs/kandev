---
created: 2026-09-25
status: in_progress
requirements:
  - REQ-TASKS-EXACT-PROFILE-LAUNCH-001
system_design:
  - ../../specs/tasks/system-design/exact-profile-launch-assignment.md
---

# Exact Profile Assignment Redirect

## Outcome

Keep the newly active task session aligned with the exact profile assignment
when a workflow transition redirects a launch from a completed session.

## Work order

- [Task 01: Preserve assigned profile on launch redirect](task-01-preserve-assigned-profile-on-redirect.md)

## Verification

The redirected-session regression must prove that a session initially carrying
a different profile persists and launches with the assignment's profile. Run
the focused orchestrator test with the race detector.
