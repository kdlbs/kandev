---
status: done
created: 2026-09-15
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Session Concurrency Ceiling Delivery Plan

## Overview

Add a shared instance-wide admission ceiling for agent sessions. Preserve the
existing orchestrator launch seams, durable task metadata, and executor
callbacks. Close the recovery, replay, stale-callback, and queue ownership
gaps found during review.

## Delivery flow

```text
launch seam -> admission controller -> reservation or deferred_launch CAS
  -> executor callback -> confirm/release -> sweep replay -> task update
```

The controller owns live admission state. The task repository owns the durable
retry record. The executor owns process identity. These ownership boundaries
remain separate during repair.

## Verification strategy

- Run focused orchestrator tests for cold resume, callback identity, Office
  replay, dynamic relaunch outcomes, queue retry, and deferred-record conflicts.
- Run focused task-service tests for nested ceiling prompt edits.
- Run `go test -race` for the orchestrator package because reservations and
  callback release are concurrent.
- Run the documentation coverage evaluator through the PR workflow after push.

## Work orders

- [x] [Task 01: Preserve ceiling launch ownership](task-01-session-concurrency-ceiling.md)

## Risks

Follow-up: [queued session ownership](../queued-session-ownership/plan.md) adds
passive-origin enforcement and task queue visibility. Existing manual override
semantics and the completed work order remain unchanged.

- A repository read failure before a process callback is treated as not owned,
  so a reservation can wait for the sweep instead of being released by an
  ambiguous callback.
- The startup ceiling is environment-only. Operators must restart the backend
  after changing it.
