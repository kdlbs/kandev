---
status: current
system: cross-workspace-task-handoff
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-START-001
---

# Handoff mechanism System Design

## Purpose and boundaries

This document maps every requirement in this system onto one route handler,
one action, and one CLI subcommand, and states the evaluation order that
makes their interaction deterministic. The shared external-id idempotency
mechanism, the Office permission surface, and the platform's per-user
workspace scoping are consumed as-is and are designed in their owning
systems, not here.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001` | CLI dispatch, route registration, and body decoding, below |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001` | Authorization gates and D3a steps 1, 4-5 |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001` | Target-resource resolution, D3a step 6 |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001` | D4, D3b steps 2-4 |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001` | D1, D2, D2a, D3a step 3 |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001` | D3a step 7, D3b steps 1-2 |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001` | D5, D3b step 3 |
| `REQ-CROSS-WORKSPACE-TASK-HANDOFF-START-001` | D9, D3b step 5 |

## Implementation surfaces

- **CLI**: `apps/backend/cmd/agentctl/kandev_task.go` dispatches `task handoff`
  from `runTaskCmd`'s switch alongside `get`/`update`/`create`/`decision`,
  using `newKandevClient()` + `client.do`. Flag-presence handling (a flag set
  to empty must still reach the request body) uses `flag.FlagSet.Visit`
  rather than the sibling subcommands' `if value != "" { … }` idiom, which
  would silently drop a blank optional value.
- **Route**: `POST /api/v1/office/runtime/handoffs`, registered in
  `internal/office/runtime` alongside `POST /runtime/tasks`, authenticated
  through the existing `Handler.contextFromRequest`, and decoded with the
  runtime surface's closed-JSON body binder (`DisallowUnknownFields`, single
  JSON value).
- **Action**: `Actions.Handoff` in `internal/office/runtime/handoff.go`,
  taking a narrow `HandoffDependencies` bundle of small interfaces
  (workspace scoping, task read/create/settle, workflow steps, agent
  profiles, the reverse-link store, the session launcher, and the activity
  logger) so each dependency is independently fakeable in tests.
- **Permission and capability**: `PermCanHandoffTasks` in
  `internal/office/shared/permissions.go`; `CapabilityHandoffTask` and
  `Capabilities.CanHandoffTasks` in `internal/office/runtime/capabilities.go`
  and `context.go`.
- **Withdrawn MCP mechanism**: every artifact the CLI-route mechanism
  replaces — the MCP tool, its handlers, its capability value, and the
  system-prompt placeholder and resolution function that advertised it — is
  removed outright, not deprecated. Two neighbours are unrelated to the
  withdrawal and are reused by the new action: the agent-profile
  workspace-membership predicate, and the task-metadata compare-and-set
  primitive backing the reverse link.

## D1-D2a — the delivery task's own identity

- **D1.** The delivery task is a kanban task, not an Office task: created
  through the same task-service path same-workspace creation uses, so it
  carries no Office launch metadata and reads back as a non-office task at
  launch time.
- **D2.** The action supplies no task origin, so the delivery task's origin
  defaults to manual, exactly as same-workspace creation's own default path
  does. Introducing a dedicated origin value for handoffs would itself be a
  behavioural change disguised as an audit label; the handoff is
  distinguished by its provenance metadata and its activity verbs instead.
- **D2a.** "Office" means two different things in this codebase: a
  write-time predicate that gates office-only creation side effects (keyed
  on origin and project association), and a read-time projection that
  selects the office launch surface (keyed on workflow membership and
  project association). The delivery task must be non-office under both, so
  it is created with no project association and lands in a non-office target
  workflow, independent of which origin value is or is not supplied.

## D3 — authorization is two gates, plus advisory discoverability

Execution (the route re-derives the capability from the signed run token)
and ownership (the platform's existing per-user workspace scoping) are both
independently necessary, per the authorization requirements. Role
instructions are advisory discoverability only and are never treated as a
third gate.

### D3a — pre-create evaluation order (total order; first failure wins)

1. **Authentication.** No token, an invalid token, or an unloadable agent
   ends the call at HTTP 401, because this step produces the run context
   every later step reads.
2. **Argument shape** (unknown/missing/blank/oversized), naming no target
   resource.
3. **Same-workspace refusal**, comparing only the run token's workspace
   against the payload — it reads no resource, so it cannot leak, and it is
   why a caller targeting itself is told so before being told it lacks a
   permission.
4. **Capability gate.**
5. **Target-workspace ownership.**
6. **Target-resource checks**, in this order: `workflow_id`; destination-step
   resolution; `agent_profile_id`; `executor_profile_id`; `repository_id`;
   `base_branch`.
7. **Idempotency resolution and the create itself.**

Steps 1-3 name no target resource; every check from step 6 onward reads one,
and running any of them before steps 4-5 would let an unauthorized agent
distinguish a real resource id from a fabricated one by the error returned.

### D3b — post-create evaluation order (total order)

1. **Resolve the idempotency outcome.** On either found outcome, skip steps 2
   and 4 below and continue directly at step 3.
2. **Settle the external id** (created path only).
3. **Write, or repair, the reverse link.**
4. **Write both activity entries** — this step cannot fail the call, so its
   position changes no outcome.
5. **Dispatch the launch** (created path only, and only when `start_agent`
   was true).

A step that refuses ends the call; the only reachable mid-sequence refusal is
the reverse-link ownership mismatch at step 3, which writes no reverse link,
no activity entry, and dispatches no launch.

**This ordering does not gate the launch on the reverse link.** What makes a
running delivery task's source findable is the forward record D4 writes
inside the create itself; the reverse link is a source-side index over that
fact, repaired by a later replay. A reverse-link failure is therefore not a
refusal, and steps 4-5 still run even when step 3 failed. Only gating step 5
on step 3 could guarantee every running delivery task has a findable source
card, and this design deliberately does not gate.

### D3c — a lookup that fails to execute is never a validation error

Across every check in D3a step 6, a backend read that fails for a reason
other than absence is HTTP 500 and safe to retry; only a read that executes
and returns no row, or a row belonging to another workspace, is HTTP 400. In
both cases no write occurs. Folding a transient failure into a validation
refusal would make an automated caller give up on a call it should retry.

## D4-D10 — provenance, ordering, and scope rules

- **D4.** The forward-provenance record is part of the create request's
  metadata, never a follow-up update, so no interleaving can produce a
  delivery task with no recorded source.
- **D5.** The reverse-link append is a compare-and-set on one metadata key,
  never a read of the whole task followed by a write of the whole task.
- **D6.** Activity-logging failure never fails the call; that contract is
  inherited from the existing activity logger, not re-litigated here.
- **D7.** One clock, server-side: the handoff timestamp is the backend's UTC
  time at the moment the delivery task is created, written once to both
  sides, and never re-stamped on replay. A replay's own responses and
  repaired entries read that stored value back rather than substituting the
  replay's own clock.
- **D8.** Both provenance records live under their own named metadata keys
  with no schema-version field: the keys are additive, so a later shape
  change adds a new key rather than reinterpreting an old one.
- **D9.** This action never stamps the marker that would make the destination
  step auto-start on creation; `start_agent: false` is therefore honoured
  even when the resolved step would otherwise auto-start.
- **D10.** A supplied agent or executor profile is authoritative and is never
  overridden by a workflow, step, or workspace default — see the
  profile-resolution requirements.

## Corrected citation: workflow-workspace validation messages

An earlier draft of this design suggested reusing an existing task-creation
helper's workflow-ownership error message for this action. That helper's
message embeds the workflow's actual owning workspace id, which is exactly
the disclosure the authorization requirements forbid here, and the helper
itself is a private method on a different package's handler type, not
callable from this action's package. The action therefore implements its own
independent validation with its own non-disclosing message, matching only
the *shape* of that helper's three-way absent/wrong-workspace/failed-to-read
branching — not its message text, and not its code.
