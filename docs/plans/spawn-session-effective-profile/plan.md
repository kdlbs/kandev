---
spec: docs/specs/agents/requirements/spawn-session-effective-profile.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Spawn Session Effective Agent Profile

## Overview

`spawn_session_kandev` sends a requested profile to `LaunchSession`, then reports that request value. The orchestrator can replace it with the workflow launch profile.

The launch path already retains the effective profile in `executor.TaskExecution`. The fix adds it to `LaunchSessionResponse` and uses that value in the MCP response.

## Confirmed root cause

`resolveSpawnAgentProfile` selects the pre-launch profile. `handleSpawnSession` reports that value after `LaunchSession` returns.

`startTask` can replace that value through `resolveEffectiveAgentProfile`. `executionToLaunchResponse` drops the resulting `TaskExecution.AgentProfileID`, so the MCP handler cannot report it.

`create_task_kandev` does not need a code change. PR #2341 already aligns its stored and reported profile with workflow-default precedence.

## Backend

### Authoritative launch response

- Update `apps/backend/internal/orchestrator/session_launch.go`.
- Add `AgentProfileID` to `LaunchSessionResponse`.
- Copy `TaskExecution.AgentProfileID` in `executionToLaunchResponse`.
- Keep launch precedence and session persistence unchanged.

### MCP response and tool contract

- Update `apps/backend/internal/mcp/handlers/spawn_session.go`.
- Build `agent_profile_id` from `LaunchSessionResponse.AgentProfileID`.
- Update `apps/backend/internal/mcp/server/server.go`.
- State workflow profile precedence and the full success response shape.

## Tests

- **What:** The launch response carries the effective profile from `TaskExecution`.
  **File:** `apps/backend/internal/orchestrator/session_launch_test.go`.
  **How:** Pass a `TaskExecution` with an effective profile to `executionToLaunchResponse`.
- **What:** The MCP handler reports the launch result when it differs from the requested profile.
  **File:** `apps/backend/internal/mcp/handlers/spawn_session_test.go`.
  **How:** Use the real task service and SQLite repository for target authorization. Return a different effective profile from the fake launcher.
- **What:** The registered tool describes workflow precedence and the `agent_profile_id` response field.
  **File:** `apps/backend/internal/mcp/server/spawn_session_tool_test.go`.
  **How:** Inspect the registered tool description without adding tests to existing files above the test-file size limit.
- **What:** The existing `create_task_kandev` workflow-default regressions remain green.
  **File:** `apps/backend/internal/mcp/handlers/handlers_test.go`.
  **How:** Run the focused tests from PR #2341 without changing them.

## Public documentation

Update `docs/public/agent-communication.md`, which is the explanation page for task-agent messages and related MCP tools. Add the effective profile to both documented response shapes and explain workflow precedence.

No frontend or E2E change is required. The change affects an MCP response and its documentation, not a visual UI flow.

## Verification Results

- Task 01 red regressions failed as expected: the launch response omitted the
  effective profile, the MCP handler returned the requested profile, and the
  tool description omitted the effective response field.
- Task 01 exact verification passed: 6 tests across the orchestrator, MCP
  handlers, and MCP server packages.
- Affected backend package verification passed: `go test ./internal/orchestrator
  ./internal/mcp/handlers ./internal/mcp/server -count=1`.
- Full backend verification passed: `make -C apps/backend test` ran
  `CGO_ENABLED=1 go test -tags fts5 ./...` successfully.
- Backend lint passed: `make -C apps/backend lint` reported no issues.
- Task 02 public-doc verification passed: 60 validation tests and 41 published
  pages validated.
- Formatting and whitespace checks passed: `gofmt -l` reported no files and
  `git diff --check` reported no errors.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Propagate the effective profile](task-01-propagate-effective-profile.md)

Wave 2:

- [x] [Task 02: Update public MCP documentation](task-02-update-public-docs.md)

Execution is sequential. The documentation task depends on the final response contract from Task 01.

## Risks and limits

- `LaunchSessionResponse` serves several launch intents. The new field is additive and comes from the existing execution record.
- `spawn_session_kandev` always uses `IntentStart`, so its successful response has an effective execution profile.
- This work does not change profile precedence. It only reports the result of existing precedence.
