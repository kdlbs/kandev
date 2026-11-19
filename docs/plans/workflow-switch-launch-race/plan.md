---
created: 2026-11-19
status: done
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-switch-launch-supersede.md
legacy_specs: []
---

# Implementation plan: Workflow switch during an in-flight launch

## Overview

A task created with an immediate start and moved to a step with another
profile right away can end `FAILED`. The move parks the original session
(`CREATED` to `WAITING_FOR_INPUT`) while its launch is still preparing the
workspace; the launch then fails its state compare-and-set with "state changed
from CREATED to WAITING_FOR_INPUT before runtime persistence", marks the
session and task `FAILED`, shows "the task failed to start", and the switch to
the step's profile never completes. This package implements
`AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.17` as described in the
[system design](../../specs/tasks/system-design/workflow-switch-launch-supersede.md).

## Scope

In scope: classifying a launch that lost its session to a parking switch as
superseded in both persistence paths and in every launch-failure consumer.

Out of scope: switch and parking policy, recovery actions, automatic retries
of failed launches, new settings, and any frontend change.

## Work orders

- [x] [Task 01: Supersede a launch that a profile switch parked](task-01-supersede-switched-launch.md)

One work order: one classification and its consumers, with one verification
boundary.

## Verification strategy

Orchestrator and executor unit tests: the parking classification and its
negatives in both persistence paths, the launch-failure handler, and a real
`PrepareTaskSession` race with a blocked launch and a concurrent park. No
end-to-end test: the race needs a launch blocked at a precise point, which the
browser suite cannot hold.

## Risks

- A switch that classification misses keeps the previous failure path.
- A parking record is the only accepted proof, so an unrelated state change
  still fails the launch.

## References

- [Requirement](../../specs/tasks/requirements/workflow-profile-session-lifecycle.md)
- [System design](../../specs/tasks/system-design/workflow-switch-launch-supersede.md)
- [Session lifecycle design](../../specs/tasks/system-design/workflow-profile-session-lifecycle.md)
