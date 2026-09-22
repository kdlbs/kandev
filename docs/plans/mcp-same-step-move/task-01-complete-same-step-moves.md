---
id: "01-complete-same-step-moves"
title: "Complete same-step MCP requests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-MOVE-RESULTS-001
acceptance_criteria:
  - AC-TASKS-MCP-MOVE-RESULTS-001.1
  - AC-TASKS-MCP-MOVE-RESULTS-001.2
  - AC-TASKS-MCP-MOVE-RESULTS-001.3
  - AC-TASKS-MCP-MOVE-RESULTS-001.4
  - AC-TASKS-MCP-MOVE-RESULTS-001.5
system_design:
  - ../../specs/tasks/system-design/mcp-task-move-results.md
---

# Task 01: Complete same-step MCP requests

## Summary

Implement the validated no-op branch before session routing.
Prove that repeated calls return completion without changing task state or queued work.

## In scope

- The handler helper and regression matrix in the plan.
- Both tool registrations, response forwarding tests, and public reference wording.
- A persistent-queue integration case with repeated MCP backend dispatch.

## Out of scope

- Production database cleanup, queue cancellation, and orchestrator rewrites.
- HTTP, plugin, frontend, or position-reorder behavior changes.

## Acceptance

1. The named handler regressions fail before the correction and pass after it, including authorization and mixed-session cases.
2. A valid no-op preserves the complete stored task, existing queue state, and all transition effects across 47 requests.
3. Both tool modes expose the same result semantics, and all commands below pass.

## Implementation sequence

1. Read the requirement, design, scoped backend guidance, and existing envelope and validation fixtures.
2. Add the same-step regression first. Record its expected failure.
3. Implement the small handler helper described in the design.
4. Add validation, preservation, and persistent-queue coverage from the plan's test matrix.
5. Update tool descriptions and public reference text. Preserve the existing result envelope.
6. Run the commands below. Record results here and synchronize the plan.

The validation matrix includes nonexistent tasks, unauthorized tasks, read-only access, archived tasks, missing steps, and mismatched workflows.
It also includes cross-workspace targets, task-read failures, missing validation dependencies, and option normalization errors.
The positive cases include absent or unavailable queue support because a no-op does not use the queue.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/handlers -run 'TestHandleMoveTask|TestDeferMoveTask|TestMoveTaskErrorMessage' -count=1)
(cd apps/backend && go test ./internal/mcp/server -run 'TestMoveTask' -count=1)
(cd apps/backend && go test ./internal/workflow/move -count=1)
(cd apps/backend && go test ./internal/orchestrator -run '^TestPendingMove_EqualTargetRecordsAppliedMoveID$' -count=1)
(cd apps/backend && golangci-lint run ./internal/mcp/handlers ./internal/mcp/server)
(cd apps/backend && go build -o /tmp/kandev-issue3872-final ./cmd/kandev)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/handlers/config_task_handlers.go`
- `apps/backend/internal/mcp/handlers/config_task_handlers_same_step_test.go` (new)
- `apps/backend/internal/mcp/handlers/handlers_test.go` (shared fixture)
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/mcp/server/config_handlers_test.go`
- `docs/public/agent-communication.md`

## Dependencies

None.

## Risks

The current fixtures sometimes omit workflow controllers. Do not preserve that omission as a production validation bypass.
Existing task-write authorization must run even though this branch performs no write.
Existing queued work must survive the no-op unchanged.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/mcp-task-move-results.md)
- [System design](../../specs/tasks/system-design/mcp-task-move-results.md)
- [Plan and diagnostic evidence](plan.md)
- Existing `pendingMoveRecordingQueuer`, `newTestTaskService`, and `seedRunningTask` fixtures.
- Existing `TestHandleMoveTask_DeferredReturnsMoveResultEnvelope` and `TestPendingMove_EqualTargetRecordsAppliedMoveID`.

## Results

Implemented and verified on 2026-09-23. The regression was first confirmed red: the same-step request returned `deferred`, included a move ID, and recorded one pending move. After the fix, the handler matrix, 47-request preservation cases, persisted queue dispatch, response forwarding, and tool-description checks passed. The validated no-op returns the stored task and position without changing task, queue, session, prompt, metadata, event, or transition state.

All verification commands above passed. Additional checks passed: `golangci-lint run ./internal/mcp/handlers ./internal/mcp/server` (0 issues), backend binary build, public documentation validation (62 tests and 47 pages), catalog validation (299 decisions and 1114 specifications), specification lint, and `git diff --check`.
