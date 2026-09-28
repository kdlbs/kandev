---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Agent and executor profile resolution Requirements

## Overview

The caller names both the agent profile and the executor profile explicitly,
and both are used exactly as named — no workflow, step, or workspace default
may fill or override either one. The caller is choosing on behalf of a
workspace it does not run in, so a silently-defaulted or silently-overridden
profile reproduces the exact failure this system exists to stop.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001: Explicit, validated, authoritative profiles

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.1:** The action shall reject
  a call omitting `agent_profile_id` or `executor_profile_id` with HTTP 400
  naming the missing one, even when a workspace, workflow, or step default
  could have supplied it.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.2:** Both ids shall be
  validated before any write; one that does not resolve shall be rejected
  with HTTP 400 naming which of the two failed. When both are unresolvable,
  `agent_profile_id` shall be the one named.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.3:** The two ids resolve by
  different predicates:
  - `agent_profile_id` resolves when the profile exists and its own
    workspace scope is either empty (global) or equal to
    `target_workspace_id`. A profile scoped to any other workspace —
    including the source workspace — is refused: accepting a source-scoped
    profile would let a caller run its own workspace's agent inside a
    workspace it does not run in, the cross-boundary leak the authorization
    gates exist to prevent.
  - `executor_profile_id` resolves on existence alone; executor profiles
    carry no workspace scope, so no further scoping test applies.

  A read that fails to execute for either check is HTTP 500 and shall be
  safe to retry, per the absence-versus-failure rule stated in the
  authorization requirements.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.4:** The supplied ids shall
  be the ones recorded and used. No inheritance or defaulting chain — the
  target workflow's launch profile, the destination step's pinned profile,
  the target workspace's defaults, or the source session's own profile —
  shall override either value, and none shall be consulted. A workflow or
  step whose own launch profile differs shall not cause a refusal and shall
  not change the outcome.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.5:** On success the
  delivery task shall carry the supplied agent profile and executor profile
  in its launch metadata, so it can be started later from the board even
  when `start_agent` was false.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.6:** The action shall not
  accept a `workflow_step_id` argument. The destination step shall be
  resolved server-side for the requested `start_agent` value: the target
  workflow's first eligible auto-start step when `start_agent` is true,
  otherwise its start step. The caller does not run in the target workspace,
  so pinning a step there is not a decision it can make competently.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.7:** Step resolution shall
  distinguish configuration from failure:
  - a target workflow whose steps list successfully but yield no resolvable
    step, and a workflow with zero steps, shall be refused with HTTP 400;
  - a failure to read the steps shall be refused with HTTP 500 and shall be
    safe to retry;
  - in both cases no write shall occur, and no delivery task shall be
    created sitting on no step.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.8:** Step selection shall be
  deterministic and reproducible: the target workflow's steps shall be
  ordered by position ascending, with step id ascending as the tiebreak for
  equal positions, before selection. When `start_agent` is true, the action
  shall select the earliest step in that order carrying an auto-start
  `on_enter` action, falling back to the start-step rule when there is none;
  otherwise it shall select the earliest step in that order flagged as the
  workflow's start step, falling back to the earliest step overall when none
  carries that flag. Two calls with identical arguments against an unchanged
  workflow shall resolve the same step.
