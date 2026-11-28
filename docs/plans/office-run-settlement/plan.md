---
created: 2026-10-20
status: done
requirements:
  - REQ-OFFICE-RUN-SETTLE-001
  - REQ-OFFICE-RUN-SETTLE-002
  - REQ-OFFICE-RUN-SETTLE-003
system_design:
  - ../../specs/office/system-design/run-settlement-integrity.md
legacy_specs: []
---

# Implementation Plan: Office Run Settlement Integrity

## Overview

Three production defects left Office runs claimed, hidden, or duplicated. This
plan settles parked per-launch sessions, resolves inherited workspaces for
parents run only by Office, surfaces unsafe workspace reuse, and adds an opt-in
guard that waits for the failed session's verdict before an agent-error
replacement run is created.

## Root cause

- Run settlement depended on the Office `IDLE` teardown (`AgentStopped`) or a
  process exit (`AgentCompleted`). A Routine-workflow task is not an Office task
  (`Task.IsFromOffice` is false), so its sessions follow the per-launch model
  and park at `WAITING_FOR_INPUT` with the process alive: neither event fires.
- Inheritance read only the parent's primary session. Office sessions are never
  primary, so a parent run only by Office looked unresolvable. Unsafe workspace
  reuse then took the generic 2m-2h retry backoff.
- `handleAgentFailed` made the run terminal and dispatched `on_agent_error`
  without waiting for the orchestrator's session verdict, while dynamic routing
  launched a successor in the same session.

## Scope

### In scope

- `REQ-OFFICE-RUN-SETTLE-001`: settle per-launch runs at a clean park and
  Office runs after their completing turn confirms runtime settlement. Keep
  successor ownership and completed business results intact.
- `REQ-OFFICE-RUN-SETTLE-002`: inheritance fallbacks and immediate escalation of
  unsafe workspace reuse.
- `REQ-OFFICE-RUN-SETTLE-003`: the default-off session verdict guard.

### Out of scope

- Office task classification, primary promotion of Office sessions, dynamic
  routing's failover decision, and restart persistence of deferred failures.

## Work orders

- [Task 01: Settle parked launched sessions](task-01-parked-session-settlement.md)
- [Task 02: Resolve primaryless inherited workspaces](task-02-primaryless-inheritance.md)
- [Task 03: Agent-error session verdict guard](task-03-agent-error-session-verdict.md)

## Risks

- Early settlement of a canonical Office session can race delayed teardown
  and configure a process still owned by the previous turn. Office now waits
  for completion confirmation while Routine/per-launch park behavior remains.
  Failed stop is not success; immutable turn timing fences later claims.
- A pending inheritance (live parent, no environment yet) keeps the retry
  backoff and escalates only after `MaxRetryCount`, as before; its first
  deferral is now recorded on the run and in activity.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/office/service/ -run 'Parked|AgentErrorSessionVerdict|HandleRunFailure|AgentCompleted|AgentFailed'
go test -count=1 ./internal/orchestrator/ -run 'Inherit|SharedGroup'
go test -count=1 ./internal/runtimeflags/ ./internal/profiles/
```
