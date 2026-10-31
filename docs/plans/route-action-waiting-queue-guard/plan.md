---
created: 2026-10-29
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Hold queued delivery while a dynamic route awaits action

## Overview

A message queued for a dynamic session whose route is `action_required` and
which has no `executors_running` row was reserved on every automatic drain,
failed in `ensureSessionRunning` with "no executor record", was classified as
a launch still in progress, and was requeued at the head. Nothing told the user,
and nothing would launch the session on its own.

[Requirements](../../specs/agents/requirements/dynamic-agent-routing.md)
AC 001.15 and 001.16 define the corrected delivery contract. The
[design](../../specs/agents/system-design/dynamic-agent-routing-02.md) records
where the block sits.

## Scope

In scope: the automatic queue reservations in `event_handlers_workflow.go` and
`task_operations.go`, the post-dispatch retention in `event_handlers_agent.go`,
the new `queue_dynamic_route_block.go`, the shared orphan reason in
`dynamic_policy_recovery.go`, and a focused regression in
`dynamic_stranded_starting_route_test.go`.

Excluded: queue transfer on an explicit primary switch, the reuse selector, the
`dynamic_route_pending` guard, the runtime-unavailable requeue for sessions
without a dynamic route, and any new setting or flag.

## Technical approach

Skip automatic queue-head reservation while the route is `action_required` and
the executor row is absent. When a queued dispatch still fails because a
WAITING_FOR_INPUT session's `starting` route is stranded, apply the restart
orphan-sweep release to `action_required` and restore the entry for manual
recovery instead of requeueing it. Reuse the existing route state, claim, and
restore helpers.

## Work orders

- [x] [Task 01: Hold queued delivery while a dynamic route awaits action](task-01-hold-queue-for-route-action.md)

## Changed-path coverage

The work order owns the orchestrator files above, the focused regression in the
existing dynamic stranded-route test file, the two new criteria in the dynamic
agent routing requirement, and the matching note in part 2 of its system design.
