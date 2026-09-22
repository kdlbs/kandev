---
status: draft
system: tasks
created: 2026-09-22
owners:
  - kandev
---

# MCP task move results

## Overview

Agents need to distinguish a completed destination request from a move that waits for turn end.
The task system owns this result because it owns workflow placement and deferred moves.

The existing [ordering requirement](kanban-task-reordering.md),
AC-TASKS-KANBAN-TASK-REORDERING-001.28, already preserves position for moves to the current step.
This document defines the MCP result for that case. It does not replace the ordering contract.

## Requirements

### REQ-TASKS-MCP-MOVE-RESULTS-001: Completed destination requests

**Intent:** An agent can recognize success without submitting the same destination request again.

#### Acceptance criteria

- **AC-TASKS-MCP-MOVE-RESULTS-001.1:** When a valid request names the current workflow and step without entry options, the system shall return `disposition: "applied"`.
  This result shall apply with no session, an idle session, or any running or starting session.
- **AC-TASKS-MCP-MOVE-RESULTS-001.2:** The result shall contain the stored task, including its actual position.
  The system shall not create a move identifier or report a new workflow entry for this no-op.
- **AC-TASKS-MCP-MOVE-RESULTS-001.3:** The no-op shall leave task state, position, metadata, session state, prompts, and pending moves unchanged.
  It shall not run step actions or create transition history. Repeated requests shall have the same behavior.
- **AC-TASKS-MCP-MOVE-RESULTS-001.4:** The no-op shall retain task-write authorization, archive restrictions, and workflow and step validation.
  Invalid requests shall fail without side effects. Non-empty normalized entry options shall remain invalid without a step change.
- **AC-TASKS-MCP-MOVE-RESULTS-001.5:** A valid request to another step shall retain the existing immediate or deferred behavior.
  Both task-mode and configuration-mode tool descriptions shall explain that `applied` also means the task already occupies the requested destination.
  The descriptions shall state that callers need no retry for that result.

## Compatibility and exclusions

The result reuses the existing `applied` value and response envelope. It adds no disposition value or request field.
Empty entry options retain their existing normalization. The legacy `prompt` remains an alias for entry instructions.

Existing queued work remains independently eligible. Success describes the destination at the time of the task read, not a reservation against later moves.

Historical pending-row repair, move cancellation, position reordering, HTTP behavior, and workflow recovery are outside this capability.

## Implementation plans

- [Same-step MCP move fix](../../../plans/mcp-same-step-move/plan.md)
