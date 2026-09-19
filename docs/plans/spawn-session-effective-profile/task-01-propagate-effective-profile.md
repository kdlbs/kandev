---
id: "01-propagate-effective-profile"
title: "Propagate the effective profile"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/spawn-session-effective-profile.md"
---

# Task 01: Propagate the effective profile

## Intent

Make the synchronous launch result authoritative for the profile that `spawn_session_kandev` reports.

## Inputs

- The spec `What`, `API surface`, and `Scenarios` sections.
- The confirmed root cause in `plan.md`.
- The response propagation pattern in `executionToLaunchResponse`.
- The existing handler test setup in `spawn_session_test.go` and `message_task_test.go`.

## Acceptance

- `LaunchSessionResponse.AgentProfileID` equals the effective profile in `TaskExecution` for a successful start.
- `spawn_session_kandev` reports that effective profile, including workflow-default and step-pinned overrides.
- The tool description states the precedence and includes `agent_profile_id` in its response shape.
- Existing `create_task_kandev` workflow-default tests remain green.

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_launch_test.go`
- `apps/backend/internal/mcp/handlers/spawn_session.go`
- `apps/backend/internal/mcp/handlers/spawn_session_test.go`
- `apps/backend/internal/mcp/handlers/message_task_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/spawn_session_tool_test.go`

## TDD sequence

1. Add the launch-response and MCP-response regressions.
2. Run the focused command and make sure that the new regressions fail for the expected missing field or echoed value.
3. Add the response field and propagate the effective value.
4. Update the tool description and its contract test.
5. Run the same focused command and make sure that all selected tests pass.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server -run 'TestExecutionToLaunchResponseReportsAgentProfile|TestHandleSpawnSessionReports(Effective|StepPinned)AgentProfile|TestSpawnSessionToolDescribesEffectiveAgentProfile|TestResolveMCPAutoStartConfig_WorkflowDefaultOutranksExplicit(OnUnpinnedStep|WhenStepOmitted)' -count=1
```

## Dependencies

None.

## Parallelism

`sequential`. This task changes the shared launch response contract and its MCP consumer.

## Output contract

Report the files changed, the red and green test results, blockers, and risks. Update this task and `plan.md` in the same conversation.

## Results

- Red: the launch-response and tool-description regressions failed because the
  response omitted `agent_profile_id`; the handler regression failed because it
  returned the requested profile instead of the effective launch result.
- Green: `go test ./internal/orchestrator ./internal/mcp/handlers
  ./internal/mcp/server -run 'TestExecutionToLaunchResponseReportsAgentProfile|TestHandleSpawnSessionReportsEffectiveAgentProfile|TestSpawnSessionToolDescribesEffectiveAgentProfile|TestResolveMCPAutoStartConfig_WorkflowDefaultOutranksExplicit(OnUnpinnedStep|WhenStepOmitted)' -count=1`
  passed 5 tests in 3 packages.
- Fixup regression extension: the focused command with
  `TestHandleSpawnSessionReportsStepPinnedAgentProfile` passed 6 tests in 3
  packages.
- Full backend gate: `make -C apps/backend test` passed
  `CGO_ENABLED=1 go test -tags fts5 ./...`.
- Backend lint: `make -C apps/backend lint` passed with no issues.
- Affected package suite: `go test ./internal/orchestrator
  ./internal/mcp/handlers ./internal/mcp/server -count=1` passed.
- Formatting and whitespace checks passed. No throwaway files or logs remain.
- External side effects: None.
