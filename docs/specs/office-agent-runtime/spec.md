---
status: draft
created: 2026-05-09
owner: cfl
---

# Office Agent Runtime

## Why

Office agents need stable operating primitives instead of relying on UI-specific
flows or broad service access. Users need autonomous runs that are inspectable,
scoped, and recoverable: every agent action should be tied to a run, bounded by
capabilities, and visible in Office activity.

## What

- Each scheduler-launched agent turn has a runtime context with workspace,
  agent, task, run, session, wakeup reason, and capability scope.
- Agents mutate Office through a narrow action surface: post comment, update
  task status, create subtask, request approval, read/write memory, and inspect
  assigned skills.
- Runtime actions check capabilities before mutating state.
- Runtime actions attach agent/run/session identity to emitted records whenever
  the underlying feature supports it.
- Capability scope is explicit per run. A run may update its current task, any
  task explicitly granted in the scope, or every task only when the wildcard
  scope is granted.
- Denied runtime actions fail with a forbidden error and do not call downstream
  services.
- The Office UI remains downstream of the same primitives: runs, tasks,
  comments, approvals, skills, memory, costs, and activity.

## Scenarios

- **GIVEN** an agent run scoped to task `KAN-1`, **WHEN** the agent posts a
  comment on `KAN-1`, **THEN** Office records an agent-authored comment tied to
  that run context.
- **GIVEN** an agent run scoped to task `KAN-1`, **WHEN** the agent tries to
  update `KAN-2` without explicit scope, **THEN** the runtime denies the action
  and no task mutation is attempted.
- **GIVEN** an agent run with `create_subtask` capability, **WHEN** it creates a
  subtask under its current task, **THEN** Office creates the task through the
  runtime action surface and preserves the caller agent identity.
- **GIVEN** a run without a capability, **WHEN** the agent attempts the matching
  action, **THEN** Office returns a forbidden error and logs no downstream
  mutation.

## Out of scope

- Replacing the existing scheduler in this iteration.
- Distributed scheduling or multi-backend leader election.
- A public external API contract for third-party agent runtimes.
- UI changes beyond surfaces that later consume runtime records.

