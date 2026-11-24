---
created: 2026-11-17
status: completed
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Shared workspace turn changes

## Overview

Implement the reviewed attachment with the requested correction to retry recovery.
The user requested implementation and PR delivery after the patch review.

## Scope

Validate capture admission against the session's persisted environment binding.
Stop summary retries for missing records and invalid relationships. Preserve
retry recovery for temporary storage and publication failures.
No schema, UI, public API, capture fence, or historical-data repair changes.

## Technical approach

Remove only the environment-owner equality from `validateTurnChangeSetRelationship`.
Keep the exact task/session/turn and session/environment joins.
Classify permanent errors in both lifecycle summary entry paths. Keep transient
retries without the attachment's eight-attempt cutoff and suppress background
shutdown warnings. Existing durable content recovery remains unchanged.

## Work orders

1. [Task 01](task-01-binding-and-retries.md): done. Sequential implementation and verification passed.

## Validation

Use the exact commands and regression evidence in Task 01.
