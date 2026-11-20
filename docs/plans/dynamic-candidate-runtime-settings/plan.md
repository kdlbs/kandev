---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
legacy_specs: []
---

# Implementation Plan: Isolate dynamic candidate runtime settings

## Overview

A dynamic session records the model, permission mode, and ACP options that the
running concrete provider reports (`runtime_config` and `session_mode` session
metadata). When the route moved to a different concrete candidate, that
metadata stayed, and the next candidate's launch applied it as the session's
runtime settings. The successor could then start with a model or permission
mode that only the previous provider offers and fail before its first prompt.
One sequential work order clears the previous provider's observations on a
candidate switch while keeping explicit user choices.

## Scope

In scope: the orchestrator's automatic candidate switch
(`persistDynamicLaunchDecision`), the two launch-resolution projections in
`dynamic_launch.go` and `task_operations.go`, the manual route action selection
in `backendapp/dynamic_routing.go`, and the mode/model event staleness check.

Excluded: mapping modes between providers, profile defaults, explicit
`runtime_config_overrides`, ACP model selector snapshots, and repair of
sessions that already failed before this change.

## Technical approach

Follow the candidate-switch rule in
[dynamic agent routing part 1](../../specs/agents/system-design/dynamic-agent-routing-01.md#profile-configuration).
A switch is a projection from one non-empty concrete profile to a different
one. The new `dynamic_runtime_config.go` drops `runtime_config` and
`session_mode`, restores the current workflow step's declared mode, and writes
the projection with `UpdateTaskSessionIfCurrentSnapshot` under the session
settings lock. A changed revision returns a stale result instead of overwriting
newer state. Settings events carry their execution profile and are stale once
it differs from the session's current concrete profile.

## Work orders

- [x] [Task 01: Drop previous candidate runtime settings on a switch](task-01-candidate-runtime-reset.md)

## Changed-path coverage

The work order owns the orchestrator and backendapp files above, the new
regression test file, the extended
`docs/specs/agents/requirements/dynamic-agent-routing.md` acceptance criterion,
and the updated `dynamic-agent-routing-01.md` design.

## Verification

Permanent regression coverage lives in
`apps/backend/internal/orchestrator/dynamic_candidate_runtime_settings_test.go`:

- a candidate switch drops the previous provider's model and mode and keeps an
  explicit override;
- a workflow-declared mode survives the switch;
- a same-candidate retry keeps its runtime settings;
- a late mode or models event from the previous candidate does not restore its
  mode or model;
- the exported switch used by manual route actions rejects a changed revision.

## Risks

- Clearing too much would drop explicit user choices; only provider
  observations are removed.
- Fencing events by execution profile could drop a legitimate event; events
  without an execution profile keep the existing checks.
- A revision conflict now fails the launch attempt instead of silently
  overwriting a concurrent update.
