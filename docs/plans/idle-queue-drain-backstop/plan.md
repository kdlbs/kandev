---
created: 2026-10-20
status: done
requirements:
  - REQ-TASKS-IDLE-QUEUE-DRAIN-001
system_design:
  - ../../specs/tasks/system-design/idle-queue-drain.md
legacy_specs: []
---

# Implementation Plan: Idle queue drain backstop

## Overview

Queue drains are event driven. A dispatch that timed out and was returned to
the queue, or a row left on a session that went idle, waits for the next event.
The existing ceiling sweep now offers those sessions to the ordinary Auto-run
drain with a per-session backoff.

## Scope

In scope: `apps/backend/internal/orchestrator/ceiling_sweep.go`, `service.go`,
the new `queue_idle_drain.go`, and the new
`apps/backend/internal/orchestrator/messagequeue/pending_sessions.go` listing,
with their tests.

Excluded: dead-letter storage, user notifications, new settings, a new
goroutine, and any change to queue targets or Auto-run policy.

## Technical approach

Reuse `DrainQueuedMessageIfAutoRun` so every existing guard decides, and add
only a listing and an in-memory backoff.

## Work orders

- [x] [Task 01: Drain queued messages left on idle sessions](task-01-idle-queue-drain.md)

## Changed-path coverage

The work order owns the orchestrator and messagequeue files above, their
permanent tests, and the new idle queue drain requirement and system design.
