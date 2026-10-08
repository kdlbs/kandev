---
id: "01-dialog-scoped-task-created-callback"
title: "Add the dialog-scoped task-created callback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-VOICE-EXTRACTION-HOST-001
acceptance_criteria:
  - AC-PLUGINS-VOICE-EXTRACTION-HOST-001.9
system_design:
  - ../../specs/plugins/system-design/task-create-dialog-completion.md
---

# Task 01: Add the dialog-scoped task-created callback

## Summary

Provide a task-created callback to create-mode task-create plugin contributions. Keep notification tied to the owning dialog's successful create result and pass the created task identity.

## In scope

- The dialog-owned handler registry and scoped context.
- Registry ownership for each open create cycle, including revocation when the dialog closes or changes modes.
- The public SDK and composer-slot callback types, with capability injection only for create-mode task-create contributions.
- Isolated sync/async handler failure logging and callback cleanup on unmount.
- Regression coverage for successful create, failure, dialog isolation, and non-create surfaces.

## Out of scope

- Global task lifecycle event changes.
- Edit and new-session completion callbacks.
- Plugin-specific tag persistence or task update behavior.

## Acceptance

1. The callback receives the exact task identity created by its owning dialog, once after successful creation.
2. Failed or canceled creation and unrelated task creation do not notify the callback.
3. Unmounting a contribution unregisters its callback; edit and new-session slots do not receive the create-only capability.
4. The public SDK types synchronous and asynchronous handlers; each failure is logged without blocking other handlers or changing task creation.
5. A late create response from a closed or replaced dialog cycle cannot notify a contribution from a later cycle.

## Review remediation results

The follow-up also binds the registry to one open create cycle, so a late result
cannot reach callbacks registered after close or a mode change. Verification:

- Targeted task-create dialog and plugin SDK tests — passed (83 tests across 5 files).
- Web typecheck — passed.
- ESLint on changed web TypeScript files — passed.
- SDK typecheck, including the typed composer-slot consumer fixture — passed.
- SDK runtime tests — passed (2 tests).
- Prettier checks for changed TypeScript files — passed.
- Specification validation and lint — passed.
- Public documentation tests and validation — passed (62 tests; 47 published pages).
- `git diff --check` — passed.
