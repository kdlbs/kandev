---
created: 2026-10-25
updated: 2026-10-25
status: done
requirements:
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-001
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-002
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-003
  - REQ-TASKS-TERMINAL-SESSION-HISTORY-001
system_design:
  - ../../specs/tasks/system-design/guarded-message-queue-disposition.md
  - ../../specs/tasks/system-design/terminal-session-history.md
legacy_specs: []
---

# Implementation plan: Safe session cleanup and history

## Overview

Preserve queued work across supported session ownership changes and make
terminal helper cleanup retain useful conversation history. Queue safety and
archived-history presentation are separate vertical results in the same
session-cleanup delivery.

## Scope

In scope: content-free queue census and exact disposition, scheduled routine
wake coalescing, durable queue recovery and session-deletion guards, scoped
terminal-helper cleanup receipts, archived transcript retention, and explicit
history access on desktop, kanban preview, and mobile.

Out of scope: stopping live sessions, restoring archived sessions to active
state, and deleting task-owned workspace resources.

## Work orders

- [x] [Task 01: Preserve and recover session queue work](task-01-preserve-session-queue-work.md)
- [x] [Task 02: Archive and reopen helper history](task-02-archive-helper-history.md)

## UI preview

The required interaction is an explicit history control. Archived rows appear
only after the user enables history; selecting one opens its existing
conversation. Labels and exact spacing are illustrative.

```text
UI-01: Desktop task session tabs, history hidden
[Active session] [Active helper] [History icon]

UI-02: Desktop task session tabs, history shown
[Active session] [Archived helper] [Hide history]

UI-03: Mobile task sessions, history hidden
Sessions
[Active session]
[Active helper]
[Show history]

UI-04: Mobile task sessions, history shown
Sessions
[Active session]
[Archived helper]
[Hide history]
```

The control remains localized and accessible; mobile uses a full-width touch
target. Browser coverage owns desktop transcript reopening, kanban preview
visibility, and mobile history visibility.

## Verification

The queue work order owns Go tests for census, exact disposition, recovery, and
session deletion guards. The history work order owns cleanup and summary
projection tests plus desktop, kanban preview, and mobile browser journeys.
Specification consistency is checked with `python3 scripts/list-docs.py validate`
and `python3 scripts/lint-spec-files.py --all`.

## Results

Implementation and its focused backend and browser evidence are recorded in
the work orders. Documentation coverage initially failed because this
delivery package was absent from the changed PR paths. This package supplies
the linked requirements, designs, and work orders for the already accepted
candidate; it does not claim new runtime evidence.
