---
id: "04-restart-recovery-route-activation"
title: "Activate resumed dynamic routes and fail blocked explicit recoveries"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.13
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---
# Task 04: Activate resumed dynamic routes and fail blocked explicit recoveries

## Summary

A resumed dynamic session stays on a `starting` route, so later explicit recoveries are suppressed as `dynamic_route_pending` and still report success without launching anything.
Mark the route active when the resumed runtime is ready, let an explicit recovery re-claim a stranded `starting` route, and return an error when an explicit recovery is still blocked.

## Evidence and cause

- A resume re-claims the persisted route as `starting` and settles the session to WAITING_FOR_INPUT before the process-start callback runs.
- `handleAgentProcessStarted` (`orchestrator/dynamic_launch.go`) marks the route active only for STARTING and RUNNING sessions, so the route stays `starting`.
- `loadResumeSessionCandidate` (`orchestrator/task_operations.go`) rejects every non-active route with `dynamic_route_pending`, including a stranded `starting` route that an explicit recovery could resume.
- `launchResume` (`orchestrator/session_launch.go`) turns that rejection into a success response for every caller. An explicit recovery caller then acknowledges delivery or commits a continuation against a missing execution, which surfaces later as "native resume execution is unavailable" or "continuation candidate has no native session identity".

## In scope

- Write the tests below first and confirm that they fail on the base commit.
- `apps/backend/internal/orchestrator/dynamic_launch.go`: accept WAITING_FOR_INPUT in the process-start callback.
- `apps/backend/internal/orchestrator/dynamic_restart_recovery.go`: `activateResumedDynamicRoute` and `explicitRecoveryOwnsStrandedStartingRoute`.
- `apps/backend/internal/orchestrator/task_operations.go`: activate the route on the resume-ready path and let the explicit recovery guard re-claim a stranded `starting` route.
- `apps/backend/internal/orchestrator/session_launch.go`: return the block as an error when the request carries a recovery action.

## Out of scope

- Automatic `continue_from_history` after a refused resume or a blocked prompt.
- Changes to the orphaned starting-route reconcile, passive auto-resume eligibility, and durable delivery recovery.
- Frontend changes.

## Acceptance

- A resumed runtime that is ready and waiting for input leaves the claimed route active. Cancelled sessions, other executions, and stale generations stay unchanged.
- An explicit recovery resumes a stranded `starting` route. A live launch owner, a passive resume, and other non-active states keep the guard.
- Failed activation returns to the actual resume caller while preserving the healthy runtime. After storage recovery, admission reconciles only the same generation and current prompt-ready execution; cancelled sessions and predecessor/unknown executions stay guarded.
- A blocked explicit recovery returns an error. A passive session open keeps its waiting response.

## Tests

| AC suffix | Test file and test |
| --- | --- |
| 001.12 | `apps/backend/internal/orchestrator/dynamic_restart_recovery_test.go`: `TestHandleAgentProcessStartedActivatesRouteWaitingForInput`, `TestActivateResumedDynamicRouteMarksClaimedRouteActive` |
| 001.13 | `apps/backend/internal/orchestrator/dynamic_restart_recovery_test.go`: `TestExplicitRecoveryReclaimsStrandedStartingRoute`, `TestLaunchResumeRecoveryActionReportsBlockedRouteAsError` |

## Validation

```bash
cd apps/backend
go test ./internal/orchestrator -run 'Resume|Recover|DynamicRoute|ProcessStart|SessionOpen|Dynamic|LaunchSession' -count=1
go build ./... && go vet ./internal/orchestrator
```

## Review remediation results

Exact-source contained harness RED reproduced predecessor, unknown-execution, and cancelled-session admission. GREEN with the execution/state guards passed under the race detector. These use explicit fake storage/runtime boundaries and do not prove SQLite behavior. Permanent ownership and actual resume-caller tests are added alongside the source; their verification is owned by hosted Backend CI. Final exact-head CI remains externally pending.

The projection failure case now has `TestResumeAdmissionWaitsForPersistedActiveRouteProjection`: durable activation alone cannot admit a still-starting persisted session. A later admission repairs the projection and returns its authoritative reason and timestamp for the same live execution. Focused contained RED/GREEN is retained externally; this new real-package SQLite test and final-head suites await hosted CI.
