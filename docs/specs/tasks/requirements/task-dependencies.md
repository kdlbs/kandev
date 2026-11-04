---
status: active
system: tasks
created: 2026-08-09
owners:
  - kandev
---
# Task Dependencies and Auto-Start Chains Requirements

## Overview

This requirement record defines the observable task-system contract for the companion system design.

## Requirements

### REQ-TASKS-TASK-DEPENDENCIES-001: Task Dependencies and Auto-Start Chains

**Intent:** Preserve the observable behavior of the migrated capability.

#### Acceptance criteria

- **AC-TASKS-TASK-DEPENDENCIES-001.1:** When a consumer uses this capability, the system shall satisfy the behavior described by the companion system design.
- **AC-TASKS-TASK-DEPENDENCIES-001.2:** When the system evaluates a dependency,
  it shall use the predecessor's persisted task state, even after archival.
  `COMPLETED` resolves. `FAILED` or `CANCELLED` fails. Unfinished work stays
  pending. Archiving shall not remove the edge.
