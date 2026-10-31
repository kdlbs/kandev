---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Explicit retry of a failed dynamic route

## Overview

A dynamic-profile launch that fails before the route becomes active leaves a
FAILED session whose route generation is still `starting` with no owner. The
Retry route action resumes only routes in a pending or `action_required`
state, so it answered `dynamic_route_pending` forever and the user could not
recover the task. Normal cleanup also keeps a stopped `executors_running` row
for conversation and workspace resume, which an ownership check must not read
as a live launch. One sequential work order reconciles the stranded claim on an
explicit Retry and lets the manual relaunch reopen the failed session.

## Scope

In scope: a pre-handler reconciliation in `ApplyRouteAction` for an explicit
Retry, the manual-retry allowance in the dynamic relaunch session reset, and the
task scheduler admitting a FAILED task for a newly admitted session.

Excluded: timers or passive paths that reclaim routes, deleting or settling
executor records, and any change to retry policy, candidate order, or the
backend route-action handler.

## Technical approach

`prepareFailedDynamicRouteRetry` runs after the active-turn check. Under the
session lifecycle lock it reloads the session and acts only for a FAILED session
whose `starting` generation equals the request. It probes runtime liveness and
accepts only the stopped standalone local/worktree history row of the session's
own execution, then marks the same generation
`action_required` through the generation CAS and mirrors the projection. The
relaunch reset re-checks the generation and allows FAILED to
WAITING_FOR_INPUT only for a manual prompt relaunch.

## Work orders

- [x] [Task 01: Allow explicit retry of a failed dynamic route](task-01-failed-route-explicit-retry.md)

## Changed-path coverage

The work order owns `dynamic_failed_retry.go` and its test,
`dynamic_launch.go`, `service.go`, and `task_operations.go` in
`apps/backend/internal/orchestrator/`, the new acceptance criterion `.24` in
`docs/specs/agents/requirements/dynamic-agent-routing.md`, and the failure-mode
entry in `docs/specs/agents/system-design/dynamic-agent-routing-02.md`.

## Verification

- `dynamic_failed_retry_test.go` drives `ApplyRouteAction` Retry through a real
  route engine and SQLite repository, plus the relaunch reset and task
  scheduling seams.
- Existing orchestrator package tests.

## Risks

- Reclaiming a route that still has a live launch would duplicate a runtime;
  a live or unprobeable runtime and any non-history executor row keep the guard.
- Without an executor row the manual relaunch cold-resumes and is refused for
  the missing record, so an absent row keeps the guard instead of admitting a
  Retry that cannot start a runtime.
- Reopening a terminal session from an automatic path could resurrect a
  cancelled task; only a manual prompt relaunch for the current generation may
  reopen FAILED.
