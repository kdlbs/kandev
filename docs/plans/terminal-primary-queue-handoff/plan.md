---
created: 2026-10-20
status: done
requirements:
  - REQ-TASKS-PRIMARY-SESSION-OWNERSHIP-001
system_design:
  - ../../specs/tasks/system-design/primary-session-ownership.md
legacy_specs: []
---

# Implementation Plan: Opt-in queue hand-off from a terminal primary session

## Overview

When a task's primary session fails, is cancelled or completes while messages
are still queued for it, the primary role moves on but the queue stays on a
session that can never be prompted again. Agent-to-agent messages addressed to
the task (for example from `message_task_kandev`) are then never delivered.
This adds an opt-in flag that moves that queue to the new primary.

## Scope

In scope: `features.terminalPrimaryQueueHandoff` (config, runtime flag registry,
profiles, orchestrator service config), the hand-off in `SetPrimarySession`, and
its tests.

Excluded: any change with the flag off, filtering rows by author, and moving the
queue of a live session.

## Technical approach

Reuse the queue transfer and `DrainQueuedMessageIfAutoRun`; the new decisions
are whether the previous primary is terminal, that the retired source's
Auto-run does not combine into the receiver's, and that the receiver must
still be primary inside the transfer's admission.

## Work orders

- [x] [Task 01: Hand off a terminal primary's queue behind a flag](task-01-terminal-primary-queue-handoff.md)

## Changed-path coverage

The work order owns the orchestrator, config, runtime flag and profile files,
the permanent tests, and `AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.4` and `.5`.
