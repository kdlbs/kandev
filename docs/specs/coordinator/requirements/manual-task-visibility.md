---
id: coordinator-manual-task-visibility
title: Manual session task lifecycle visibility
status: active
system: coordinator
owners:
  - kandev
created: 2026-10-25
last_updated: 2026-10-25
---

# Manual session task lifecycle visibility Requirements

## Overview

An approved manual Coordinator session needs a compact disposition for a task
that may have left the board. This capability is separate from the managed
coordinator copilot and does not change the manual task's origin or lifecycle.

## Requirements

### REQ-COORDINATOR-MANUAL-VISIBILITY-001: Session-bound lifecycle read

**Intent:** An authorized manual session can determine a task's lifecycle
disposition without gaining general task relations or content access.

#### Acceptance criteria

- **AC-COORDINATOR-MANUAL-VISIBILITY-001.1:** When a trusted backend binds a
  versioned grant to one manual task session and its workspace, that session
  shall be able to request a compact lifecycle visibility result for a task in
  the same workspace. The grant shall survive session persistence and restart.
- **AC-COORDINATOR-MANUAL-VISIBILITY-001.2:** A missing, revoked, malformed, or
  task-, session-, or workspace-mismatched grant shall deny the read. A caller
  supplied task title, profile, or scope shall not grant access.
- **AC-COORDINATOR-MANUAL-VISIBILITY-001.3:** The result shall contain only the
  target's compact identity, workspace, workflow position, state, disposition,
  change time, revision, and provenance. It shall not expose conversations,
  task relations, plans, documents, or write capability.
- **AC-COORDINATOR-MANUAL-VISIBILITY-001.4:** When a task is archived, moved,
  promoted, restored, or deleted, the lifecycle disposition shall reflect
  committed evidence. Refused or rolled-back operations and replay shall not
  advance that evidence. A deleted task's receipt shall remain readable by an
  authorized same-workspace session.
- **AC-COORDINATOR-MANUAL-VISIBILITY-001.5:** A foreign-workspace target,
  unavailable target, or target without durable lifecycle evidence shall not
  disclose task data. The backend shall resolve the caller and target again at
  dispatch rather than trust MCP payload identity fields.

## Exclusions

- This requirement does not activate a grant for every manual task or change
  the managed coordinator copilot's separate tool surface.
- It does not authorize relation inspection, plugin access, task content, or
  a write operation.
