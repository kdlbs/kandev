---
status: active
system: tasks
created: 2026-08-30
owners:
  - kandev
---

# Atomic Terminal Routing Requirements

## Overview

Task completion has one idempotent destination contract across MCP moves,
explicit completion, deferred turn-end replay, workflow auto-advance, and routes
issued from merged-PR lifecycle turns.

Decision: ADR-2026-08-30-serialize-terminal-workflow-routing.

## Requirements

### REQ-TASKS-ATOMIC-TERMINAL-ROUTING-001: Terminal destination ownership

**Intent:** Once a terminal route wins, stale producers cannot move the task
backward or leave a contradictory live intent.

#### Acceptance criteria

- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.1:** An active-session move to a terminal step commits the lane, terminal task
  state, transition ledger row, and one destination-entry identity before it
  reports success.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.2:** A completion signal whose immutable turn-start step is no longer current
  fails as stale and does not create, replace, restore, transfer, or delete a
  pending move.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.3:** Automated and deferred transitions apply only when the task still occupies
  their expected source step.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.4:** Same-operation retries return the stored outcome. A different authorized
  retry against an already-terminal task is an idempotent success and creates
  neither a second transition nor a second destination-entry execution.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-001.5:** Merged-PR lifecycle prompts keep their current prompt-driven behavior. A move
  issued from the bound lifecycle turn carries the exact task/repository/PR
  cause and converges with a concurrent manual terminal route.

### REQ-TASKS-ATOMIC-TERMINAL-ROUTING-002: Exact deferred generations

**Intent:** Deferred storage never loses or overwrites a newer routing intent.

#### Acceptance criteria

- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.1:** Each pending row carries immutable row and operation identities plus the
  expected workflow step and initiating session/turn when available.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.2:** Claim and cleanup compare the exact row identity. Transition commit precedes
  exact row deletion.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.3:** Restore or transfer of an older snapshot cannot overwrite or delete a newer
  destination row. Deliberate supersession records the replaced identity.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-002.4:** Legacy rows without expected-step evidence fail closed and remain eligible for
  exact administrative cancellation or expiry.

### REQ-TASKS-ATOMIC-TERMINAL-ROUTING-003: Scope and audit

**Intent:** Routing is attributable without expanding mutation authority.

#### Acceptance criteria

- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-003.1:** Ordinary agents may terminal-route only the task bound to their server-owned
  session. Coordinator cross-task routing requires a live same-workspace grant.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-003.2:** Integration provenance is bound to the persisted event target; no caller may
  select another task or workspace through request fields.
- **AC-TASKS-ATOMIC-TERMINAL-ROUTING-003.3:** Readback can prove final lane and task state, live pending rows, routing
  operation outcome, task-step transition correlation, signal source, and
  destination-entry status.

## Compatibility and rollback

Schema evolution is additive and replay-safe on SQLite and PostgreSQL. Legacy
pending rows are never backfilled from mutable current task state. Rollout uses
one backend version; rollback keeps the additive schema and requires new-format
operations to be settled before an older binary handles the queue.

## Out of scope

- Administrative exact-row cancellation policy and UI.
- General stale-session cleanup beyond the shared pending generation API.
- A global merged-PR-to-Done automation policy.
- Mutation of historical live reproduction rows.
