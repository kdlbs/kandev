---
status: active
system: tasks
created: 2026-10-18
owners:
  - kandev
---

# Session Agent Error Dismissal Requirements

## Overview

Users can acknowledge the previous agent error retained by a task session.
Tasks owns this acknowledgment because it owns the session record and its
durable error metadata. The acknowledgment must work with either supported
database and must not suppress a different error that arrived meanwhile.

This contract is distinct from [Git push message dismissal](git-push-error-dismissal.md),
[launch recovery](task-launch-failure-recovery.md), and the shared
[database portability contract](../../platform/requirements/postgres-domain-store-parity.md).
It documents the existing session acknowledgment outcome without introducing
a recovery action or changing the dismissal interface.

## Terminology

- **Current error:** The session's retained agent error identified by its
  message and occurrence time. The existing dismissal request can additionally
  carry the error's stamp.
- **Dismissal:** A durable acknowledgment of that error. It does not prove that
  the agent recovered or change execution or workflow state.

## Requirements

### REQ-TASKS-AGENT-ERROR-DISMISSAL-001: Acknowledge the current session error

**Intent:** A user can hide a retained previous-error notice consistently on
SQLite and PostgreSQL without acknowledging another failure.

#### Acceptance criteria

- **AC-TASKS-AGENT-ERROR-DISMISSAL-001.1:** When an authorized caller dismisses
  the matching current error, either supported database shall persist the
  acknowledgment. A subsequent session read shall report that error as
  dismissed and retain its modeled diagnostics and unrelated session metadata.
- **AC-TASKS-AGENT-ERROR-DISMISSAL-001.2:** When a different message or a
  different occurrence replaces the observed error before persistence, dismissal
  of the observed error shall leave the replacement and session update time
  unchanged. A missing session or absent error shall not acquire an acknowledgment.
- **AC-TASKS-AGENT-ERROR-DISMISSAL-001.3:** When valid timestamps represent the
  same occurrence using equivalent fractional-second text or explicit time-zone
  offsets, dismissal shall accept the equivalent representation on both
  databases. Distinct occurrences separated by one millisecond or more shall
  remain distinguishable, including within the same second.
- **AC-TASKS-AGENT-ERROR-DISMISSAL-001.4:** Dismissal shall retain the existing
  session authorization and request-stamp checks. An unauthorized request or a
  nonmatching supplied stamp shall not change the error or publish a dismissal.
- **AC-TASKS-AGENT-ERROR-DISMISSAL-001.5:** On desktop and phone, the existing
  previous-error notice shall hide through its existing successful dismissal
  response path and remain dismissed when the persisted session is read again.
  A persistence failure shall remain a failure and shall not acknowledge the
  error. Dismissal shall retain the existing response, notification, and
  execution-state semantics.

## Out of scope

- Agent recovery, launch, restart, workflow transitions, or ACP behavior.
- Git push message dismissal and other error-dismissal endpoints.
- New UI, copy, error toasts, optimistic hiding, flags, schema, or configuration.
- A new identity scheme, first-writer timestamp policy, or stricter stamp guard
  inside persistence. Existing message/time comparison precision is retained.
- Porting unrelated database operations or repairing malformed stored metadata.

## System design

- [Session agent error dismissal](../system-design/agent-error-dismissal.md)

## Implementation Plans

- [PostgreSQL agent-error dismissal](../../../plans/postgres-agent-error-dismissal/plan.md)
