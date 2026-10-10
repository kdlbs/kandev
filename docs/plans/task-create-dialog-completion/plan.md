---
created: 2026-10-03
status: complete
requirements:
  - REQ-PLUGINS-VOICE-EXTRACTION-HOST-001
system_design:
  - ../../specs/plugins/system-design/task-create-dialog-completion.md
legacy_specs: []
---

# Implementation plan: Task-create dialog completion

## Overview

Provide a typed, create-dialog-scoped completion callback to plugin contributions,
so a plugin can act on the task created by its own dialog without correlating
unrelated global task events. The host isolates handler failures after creation.

## Scope

- [Requirement](../../specs/plugins/requirements/voice-extraction-host.md), acceptance criterion AC-PLUGINS-VOICE-EXTRACTION-HOST-001.9.
- [System design](../../specs/plugins/system-design/task-create-dialog-completion.md).
- Create-mode task-create plugin contributions only; edit, new-session, and unrelated task lifecycle events remain outside this contract.

## Work order

- [Task 01: Add the dialog-scoped task-created callback](task-01-dialog-scoped-task-created-callback.md).
