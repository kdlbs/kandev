---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Materialized route profile guard

## Overview

`startCreatedSession` relabelled a session with a different logical profile
without checking its durable dynamic route. On a session whose route was
already materialized for another profile, the label then named the new profile
while `dynamicLaunchDecision` fell back to the session's stored concrete
candidate from the previous family. A concurrent prepared-workspace route
claim could also land between the guard's route read and the session write.

## Scope

In scope: `apps/backend/internal/orchestrator/task_operations.go`, the new
`materialized_route_profile_guard.go`, the prepared-route ownership boundary,
the WebSocket validation response, and focused regression tests.

Excluded: the workflow profile-switch path that creates a new session, concrete
profile selection, route actions, and any route state migration.

## Technical approach

Reuse `dynamicRouteStateLoader.LoadRouteState` and refuse the start before any
session write when the materialized route belongs to another logical profile.
Serialize the prepared-session read, route claim, and session projection with
the same lifecycle lock as profile changes. Map the expected refusal to a
WebSocket validation error.

## Work orders

- [x] [Task 01: Refuse a profile change on a materialized dynamic route](task-01-materialized-route-profile-guard.md)

## Changed-path coverage

The work order owns the orchestrator and handler files above, their permanent
tests, the new criterion in the dynamic agent routing requirement, and the
matching note in part 2 of its system design.
