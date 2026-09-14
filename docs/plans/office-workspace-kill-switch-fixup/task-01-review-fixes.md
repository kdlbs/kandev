---
id: "01-review-fixes"
title: "Apply workspace pause review fixes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-KILL-SWITCH-002
  - REQ-OFFICE-KILL-SWITCH-003
  - REQ-OFFICE-KILL-SWITCH-005
  - REQ-OFFICE-KILL-SWITCH-006
acceptance_criteria:
  - AC-OFFICE-KILL-SWITCH-003.3
  - AC-OFFICE-KILL-SWITCH-003.6
  - AC-OFFICE-KILL-SWITCH-003.7
  - AC-OFFICE-KILL-SWITCH-005.6
  - AC-OFFICE-KILL-SWITCH-006.6
  - AC-OFFICE-KILL-SWITCH-006.8
  - AC-OFFICE-KILL-SWITCH-006.9
  - AC-OFFICE-KILL-SWITCH-006.11
system_design:
  - ../../specs/office/system-design/workspace-kill-switch-01.md
  - ../../specs/office/system-design/workspace-kill-switch-02.md
---

# Task 01: Apply workspace pause review fixes

## Scope

- Require workspace management authorization for pause and resume mutations.
- Keep active Office task sessions discoverable for a repeated halt sweep.
- Return sweep failures through the API and show a localized retry action on
  desktop and mobile Office surfaces.
- Reject malformed JSON request bodies and keep the empty resume-body contract.
- Update the internal design record, copy, and focused regression tests.

## Verification

- Focused backend Go tests for the pause handlers, sweep, scope middleware, and
  SQLite/PostgreSQL repository paths.
- Focused frontend API and hook tests, i18n checks, typecheck, and spec lint.
