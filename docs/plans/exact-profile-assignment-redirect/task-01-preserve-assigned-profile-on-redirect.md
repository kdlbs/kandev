---
id: "01-preserve-assigned-profile-on-redirect"
title: "Preserve exact profile on workflow launch redirect"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-EXACT-PROFILE-LAUNCH-001
acceptance_criteria:
  - AC-TASKS-EXACT-PROFILE-LAUNCH-001.1
system_design:
  - ../../specs/tasks/system-design/exact-profile-launch-assignment.md
---

# Task 01: Preserve Exact Profile on Workflow Launch Redirect

## Summary

When workflow processing redirects a launch to a different active task
session, persist the active exact assignment's profile and snapshot on that
session before binding or launching it.

## Scope

- Resolve the redirected launch profile from the active exact assignment.
- Persist a changed profile through the guarded session update before the exact
  binding or agent launch.
- Add a regression with a redirected session whose original profile differs
  from the exact assignment.

## Acceptance

- The regression proves the redirected session stores the assigned profile and
  the launch request uses that same profile.
- A guarded persistence failure prevents the launch.

## Verification

```bash
cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run '^TestStartCreatedSession_PersistsExactBindingOnWorkflowRedirect$' -count=1
```

## Results

- RED: the focused test launched `profile-other` while the exact assignment was
  `profile-exact`.
- GREEN: the focused test passed after the redirect persisted the assigned
  profile and snapshot before binding and launch.
