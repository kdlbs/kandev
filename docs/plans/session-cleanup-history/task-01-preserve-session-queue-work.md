---
id: "01-preserve-session-queue-work"
title: "Preserve and recover session queue work"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-001
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-002
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-003
acceptance_criteria:
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-001.1
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-001.2
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-001.3
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-001.4
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-001.5
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-001.6
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-001.7
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-002.1
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-002.2
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-002.3
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-002.4
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-003.1
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-003.2
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-003.3
  - AC-TASKS-GUARDED-MESSAGE-QUEUE-003.4
system_design:
  - ../../specs/tasks/system-design/guarded-message-queue-disposition.md
---

# Task 01: Preserve and recover session queue work

## Summary

Give an in-session agent a bounded, body-free way to inspect and dispose of
exact queue entries, preserve FIFO work during session ownership changes, and
refuse session deletion while unread entries remain.

## In scope

- Content-free queue census and snapshot-claimed exact disposition.
- Session-scoped authorization, concurrency serialization, and audit evidence.
- Coalescing only identical scheduled routine wakes.
- Durable transfer receipts and exact queued-message snapshots during primary
  session replacement.
- Transactional queue checks before session deletion.
- MCP tool schemas and the internal WebSocket bridge.

## Out of scope

- Broad queue clearing or body retrieval through the census tools.
- Cross-task queue operations or deletion of reserved lifecycle deliveries.
- Changes to prompt composer admission policy.

## Acceptance

- Disposition removes only the exact observed entry snapshots and preserves
  unrelated queue rows, order, provenance, and reserved deliveries.
- Recovery transfers the original queued work once, with a replayable receipt;
  failed or stale requests leave both source work and target state intact.
- Session deletion fails while queued work remains and leaves the session and
  queue unchanged.

## Verification

Run focused Go tests for message queue census/disposition, queue recovery,
session deletion preservation, and MCP authorization. Run changed-Go lint for
the touched backend files and `git diff --check`.

## Results

The accepted candidate `6aabf4bbf4e69fcadec352c3bf9ddaa080df6d62` contains
focused repository, service, orchestrator, and MCP tests for these contracts.
Independent Review and QA passed that exact candidate. This work order records
its delivered scope; the documentation-only coverage correction does not
change or extend those test results.
