---
id: "05-mcp-create-task-profile-validation"
title: "Validate create_task_kandev agent profile"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001
acceptance_criteria:
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.1
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.2
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.3
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.4
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.5
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.6
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.7
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.8
  - AC-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001.9
system_design:
  - ../../specs/tasks/system-design/mcp-create-task-agent-profile-validation.md
---

# Task 05: Validate `create_task_kandev` agent profile

## Summary

Reject an `agent_profile_id` that does not resolve, before anything is created;
reject the two setting values with a message naming the setting they belong to;
and make an asynchronous auto-start failure visible on the task instead of only
in the backend log.

## Scope

- Add profile validation in `internal/mcp/handlers` at the point where the final
  profile is resolved, before `taskSvc.CreateTask`, and only for a non-empty
  caller-supplied `agent_profile_id`:
  - exact match on `current_task` or `workspace_default` returns a validation
    error naming `mcp_task_agent_profile_default` and stating that omitting the
    argument selects the configured policy;
  - otherwise resolve through the existing agent-settings reader; a miss returns
    the shared create-task validation error code naming the unresolvable ID; a
    store error fails the call.
- Run the check after workspace and parent admission, before any write.
- Restructure `agentProfileDesc` in `registerCreateTaskTool` so the argument
  contract and the setting are separate sentences; update the schema test that
  pins `current_task` to pin the setting sentence instead.
- Change `launchAutoStartTask`'s failure handling from log-and-return to
  recording the sanitized reason on the task and publishing it through
  `publishTaskEvent`.

## Exclusions

- No change to omitted-argument resolution precedence, to
  `mcp_task_agent_profile_default`, or to creating-session runtime inheritance.
- No new sentinel argument values.
- No validation of `executor_profile_id`, `workflow_id`, or other arguments.
- No change to `spawn_session_kandev`.

## Acceptance

1. `agent_profile_id: "current_task"` returns a validation error naming the
   setting and leaves no task, session, or deferred-launch record.
2. An unknown or deleted profile ID returns the same error shape and leaves no
   task; omitting the argument keeps existing resolution unchanged, proven by
   the existing workflow-default and creating-session regressions staying green.
3. An auto-start failure after the tool returned is readable on the task and is
   delivered on the task event stream.

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/create_task_mode.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- New `apps/backend/internal/mcp/handlers/create_task_profile_validation_test.go`
  (`handlers.go` and `handlers_test.go` are large; add new tests in a new file)

## TDD sequence

1. Add failing tests: `current_task` and `workspace_default` are rejected with
   the setting-naming message and create nothing; an unknown UUID is rejected
   and creates nothing; a store error fails the call; omitting the argument
   still resolves through the existing precedence; a failing asynchronous launch
   records a reason on the task and publishes a task event.
2. Add a failing schema test for the restructured description.
3. Run the focused command and confirm the expected failures.
4. Implement validation, the description change, and the failure recording.
5. Re-run the focused command; all pass, including the existing
   `create_task_kandev` workflow-default regressions.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/mcp/... -race -count=1
```

## Dependencies

None.

## Risks

Validation must not run for an omitted argument, or it would change the
documented omitted-argument precedence. The test that keeps the existing
precedence regressions green is the guard.

## Results

Done.

Implemented:
- `Handlers.validateExplicitAgentProfile` runs in `handleCreateTask` before any
  write. It rejects `current_task` and `workspace_default` with a message naming
  `mcp_task_agent_profile_default` and stating that omitting the argument selects
  the policy, and rejects an ID that resolves to no profile. Both return
  `ErrorCodeValidation`; a store outage returns an internal error rather than
  creating a task with an unverified profile.
- New narrow `AgentProfileVerifier` seam on `Handlers`, satisfied by
  `agentsettingscontroller.Controller.AgentProfileExists`, wired in
  `backendapp/helpers.go`. An unwired verifier degrades to the previous behavior
  rather than rejecting every explicit profile.
- An omitted `agent_profile_id` is not validated and never reads the profile
  store, so the documented resolution precedence is unchanged.
- Both tool descriptions now open with "Accepts an agent profile ID only" and
  name the two words as values of the user setting. The schema test pins the new
  sentences.
- `launchAutoStartTask` records a failed asynchronous launch on the task
  (`auto_start_error` with reason and timestamp) through the task service, so it
  travels on the task event stream instead of reaching only the backend log.

Verification (2026-09-22): `go test ./internal/mcp/... -race -count=1` all `ok`.
Regression sweep over `./internal/task/...`, `./internal/agent/settings/...`
and `./internal/backendapp/...` also clean.
