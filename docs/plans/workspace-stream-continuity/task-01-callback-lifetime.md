---
id: "01-callback-lifetime"
title: "Correct workspace callback lifetime"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.43
system_design:
  - ../../specs/platform/system-design/workspace-stream-continuity.md
---

# Task 01: Correct workspace callback lifetime

## Summary and inputs

Keep attached workspace callbacks valid across ACP startup on the same runtime.
Read the [plan](plan.md), [requirement](../../specs/platform/requirements/workspace-git-status.md), and linked design before implementation.
Read backend `AGENTS.md` and `/tdd`. Existing ACP startup fencing remains mandatory.

## Responsibilities

1. Add a permanent regression with the original callback closure, a current agentctl client, and an already-attached workspace stream.
2. Advance startup generation and enter the production attached-stream reuse path before delivering post-promotion events.
3. Prove the continuity assertion fails before changing production code.
4. Remove ACP generation gating from the common workspace callback closure while retaining the client source lease.
5. Cover repeated startup and every workspace callback channel through the same closure.
6. Retain or extend client replacement/detachment, retired execution, ACP generation, readiness, and shutdown/drain tests.

Replace `TestBuildWorkspaceCallbacksRejectsRetiredStartupGeneration` with a workspace-continuity assertion.
Preserve stale-generation assertions for ACP callbacks in their existing suites.
Use channels to prove an in-flight callback holds the client lease while replacement waits.
After replacement/detachment, old callbacks must not reach downstream handlers.
Do not add sleeps as evidence of lease ownership.

## Acceptance

- The pre-fix regression fails at post-promotion forwarding, and the corrected code passes first/repeated startup cases.
- All workspace channels retain their originating execution and payload. Retired clients and executions remain rejected.
- ACP stale-startup rejection, stream reuse, readiness completion, and shutdown drain remain intact under the race detector.

## Exclusions

Polling, tracker publication, foreground read guards, UI, wire formats, and general startup-lock refactoring are outside this order.

## Files and ownership

- `apps/backend/internal/agent/runtime/lifecycle/streams.go`: common workspace callback closure and invariant comment.
- `apps/backend/internal/agent/runtime/lifecycle/streams_callbacks_test.go`: continuity and source-lease regressions.
- `apps/backend/internal/agent/runtime/lifecycle/streams_test.go`: existing attached-stream and drain behavior.
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_workspace_test.go`: existing retired execution rejection.
- Existing startup/client tests in this package: compatibility evidence, with edits only if the regression requires them.

`types.go` supplies the existing client lease. No new lifetime primitive is planned.
Coordinate with any current edits and retain their source-ownership checks.

## Verification

Run from repository root. Record the behavioral RED command and its failed assertion before the production edit.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestBuildWorkspaceCallbacks|TestWorkspaceStreamPromotion' -count=1 -timeout=60s -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -count=1 -timeout=300s)
git diff --check
```

## Dependencies and parallelism

No implementation prerequisite. Task 02 follows this order.
`sequential`. This work order does not authorize delegation.

## Results

Completed.

- RED: `go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestBuildWorkspaceCallbacksContinueAfterRepeatedStartup$' -count=1 -timeout=60s -v` failed because no callbacks were forwarded after promotion.
- GREEN: `go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestBuildWorkspaceCallbacks|TestWorkspaceStreamPromotion' -count=1 -timeout=60s -v` passed.
- After merging the current base, the full tagged race suite passed: `go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -count=1 -timeout=300s` (104.243s).
- Added repeated-startup coverage for every workspace channel, client replacement and detachment rejection, and channel-controlled source-lease ownership.
