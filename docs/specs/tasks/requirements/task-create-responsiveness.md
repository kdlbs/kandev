---
status: active
system: tasks
created: 2026-11-03
owners:
  - kandev
---

# Task creation responsiveness requirements

## Overview

Users create tasks while other agents produce output. Task creation must avoid
unnecessary persistence work that delays creation and unrelated edits.
Tasks owns this contract because it owns creation acceptance and initial sessions.
Platform retains ownership of shared database admission and durable delivery.

## Terminology

- **Creation acceptance:** The successful create response after required creation
  work and external-ID settlement. Agent startup can continue afterward.
- **Reference workload:** An existing local repository, an unlocked workflow,
  a fixed agent profile, and a nonempty initial prompt. Eight other sessions
  each offer 100 durable text events per second. The same comparison also
  measures zero and four sessions.
- **Unrelated edit:** A manual color change on another task through user settings.

## Requirements

### REQ-TASKS-CREATE-RESPONSIVENESS-001: Creation under concurrent activity

**Intent:** Reduce creation latency while preserving task identity, initial
submission recovery, and unrelated edits.

#### Acceptance criteria

- **AC-TASKS-CREATE-RESPONSIVENESS-001.1:** Under the reference workload, the
  full immediate-start request p95 shall decrease by at least 25% against the
  original implementation on the same host. Both measurements shall use identical
  fixtures, request counts, and offered load. Each load point shall contain at
  least 100 completed create requests. The comparison shall include agent-start
  and create-without-agent requests separately. No-agent p95 shall not regress.
  A task-row-only measurement shall not establish this result.
- **AC-TASKS-CREATE-RESPONSIVENESS-001.2:** During that workload, every
  unrelated edit shall succeed and survive reload. Its p95 shall not exceed
  the original implementation's p95. Durable delivery shall sustain at least
  90% of offered events. The comparison shall report failures and actual rates.
- **AC-TASKS-CREATE-RESPONSIVENESS-001.3:** When creation accepts an immediate
  agent start, its session identity, initial preview, and recoverable submission
  shall already persist. Duplicate requests shall retain their existing
  side-effect-free result. Workspace launch shall retain its asynchronous start.
- **AC-TASKS-CREATE-RESPONSIVENESS-001.4:** When preparation creates a session,
  initial primary selection shall preserve an existing primary. Concurrent
  preparations shall establish exactly one primary for a task with prepared
  sessions. Explicit later promotion shall retain its existing behavior.
- **AC-TASKS-CREATE-RESPONSIVENESS-001.5:** When successful creation repeats
  identical last-used choices, it shall preserve the current settings revision.
  Changed choices shall remain durable and portable. Concurrent color changes
  and choices for other workspaces shall survive.
- **AC-TASKS-CREATE-RESPONSIVENESS-001.6:** Desktop and phone creation shall
  retain their loading, dismissal, navigation, and recovery behavior. A failed
  or uncertain request shall not appear successful merely to shorten loading.

## Compatibility and exclusions

The [initial submission](prompt-attachments.md),
[workflow memory](task-create-workflow-memory.md), and
[external identity](external-id-idempotency.md) contracts remain applicable.
Creation does not bypass authorization, WIP admission, attachment claims,
workspace binding, cleanup barriers, or durable-delivery acknowledgment gates.

This requirement does not promise bounded latency under arbitrary overload or
an independent SQLite writer that holds a lock indefinitely.
Database write priority, additional writer pools, general HTTP retries, new
UI controls, and the aggregate creation migration are outside this package.

## Design and delivery

- [System design](../system-design/task-create-responsiveness.md)
- [Implementation package](../../../plans/task-create-responsiveness/plan.md)
