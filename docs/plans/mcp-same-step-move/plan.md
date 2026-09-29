---
created: 2026-09-22
status: implemented
requirements:
  - REQ-TASKS-MCP-MOVE-RESULTS-001
system_design:
  - ../../specs/tasks/system-design/mcp-task-move-results.md
legacy_specs:
  - ../../specs/workflow-step-move-overrides/spec.md
---

# Implementation plan: Same-step MCP move results

## Overview

[Issue #3872](https://github.com/kdlbs/kandev/issues/3872) reports repeated deferred moves to a task's current step.
One sequential work order adds a validated no-op result, regression coverage, and matching tool documentation.
Implementation and targeted verification are complete.

## Evidence and root cause

Investigation used commit `c4262c644d4` and the canonical issue. The issue contains no image attachments or comments.
The authenticated GitHub user is `carlosflorencio`; issue assignment succeeded on 2026-09-22.

`handleMoveTask` routes every RUNNING or STARTING primary session to `deferMoveTask`.
`deferMoveTask` validates entry options against step equality but does not complete an option-less equal-step request.
It calls `SetPendingMove`, creates a move identifier, and returns `deferred`.
At turn end, `applyPendingMove` recognizes the equal target and skips the transition.

A temporary `TestIssue3872SameStepRepro` used the existing envelope fixture with a SQLite task service and recording queue.
The task and requested destination both used `wf-envd` / `step-work`; the primary session was RUNNING.
No entry options were supplied. The expected result was `applied` with no pending move.

```text
go test ./internal/mcp/handlers -run '^TestIssue3872SameStepRepro$' -count=1
FAIL: expected "applied", actual "deferred"
FAIL: expected empty pendingMoves, actual one record targeting step-work
```

The reproduction ran from `apps/backend` and failed for both expected assertions.
The temporary test and generator were removed. No production or permanent test files changed.

Current orchestrator code includes equal-target record consumption and stale-move recovery.
The historical nine-row leak reported for v0.95.1 was not reproduced against a live database.
This package prevents new unnecessary records. It does not claim to repair every historical row.

## Requirements and assumptions

The task system owns this contract because it owns task placement and pending transitions.
Adjacent agent and plugin specifications do not own this MCP task result.

The existing AC-TASKS-KANBAN-TASK-REORDERING-001.28 already forbids position changes for a move to the current step.
The new [result requirement](../../specs/tasks/requirements/mcp-task-move-results.md) supplies the missing observable completion semantics.
The legacy move-overrides specification continues to own normalization and one-time entry behavior.

The selected result is `applied`, one of the issue's proposed solutions and an existing protocol value.
This avoids adding a new disposition. No material product question blocks this package.
Existing queued work is preserved because a destination no-op is not a cancellation request.

## Scope

### In scope

- Validated equal-workflow, equal-step completion before session routing.
- Stored task response, unchanged position, and no new queue or transition effects.
- Regression tests, both MCP tool descriptions, and public protocol documentation.

### Out of scope

- Historical database cleanup, cancellation semantics, and global queue redesign.
- Changes to real cross-step moves, HTTP moves, WIP admission, or rendered UI.
- New dispositions, migrations, runtime flags, and prompt-only retry workarounds.

## Technical approach

Follow [the system design](../../specs/tasks/system-design/mcp-task-move-results.md).
Add a small validated completion helper in `internal/mcp/handlers` before the session lookup.
Preserve existing write authorization through `AuthorizeWorkspaceScope` rather than relying on read permission alone.
Return the existing move envelope without calling either mutation path.

Update both tool registrations and `docs/public/agent-communication.md` in the implementation work order.
Public docs remain unchanged during this design turn because the runtime fix is not implemented.
Existing move-overrides plans remain historical records; their implementation results are not rerun or rewritten by this package.

## Tests

All new named tests below belong to `config_task_handlers_same_step_test.go` unless another path is given.

| Acceptance | Planned regression |
| --- | --- |
| AC-TASKS-MCP-MOVE-RESULTS-001.1 | `TestHandleMoveTask_SameStepApplied`: RUNNING, STARTING, idle, absent, and mixed sibling sessions |
| AC-TASKS-MCP-MOVE-RESULTS-001.2 | `TestHandleMoveTask_SameStepReturnsStoredTask`: omitted and differing requested positions |
| AC-TASKS-MCP-MOVE-RESULTS-001.3 | `TestHandleMoveTask_SameStepPreservesEffects`: 47 repeats, pending moves, prompts, metadata, session state, and event counts |
| AC-TASKS-MCP-MOVE-RESULTS-001.4 | `TestHandleMoveTask_SameStepValidation`: archive, access scope, missing targets, operational lookup failures, dependency failures, and entry options |
| AC-TASKS-MCP-MOVE-RESULTS-001.5 | Existing `TestHandleMoveTask` / `TestDeferMoveTask` cases and server `TestMoveTaskToolSchemasExposeEntryOptions` |

The first regression must fail on the current code before implementation.
Empty options normalize to no options. Non-empty instructions, reset, skip, and legacy prompt remain invalid for equal-step requests.
The existing `TestPendingMove_EqualTargetRecordsAppliedMoveID` protects downstream defense behavior.

## E2E tests

The changed interface is MCP, with no rendered UI change. No Playwright test is required.
Add `TestHandleMoveTask_SameStepPersistentQueue` with the real task service and queue repository.
Send 47 requests through the registered WebSocket MCP action and inspect responses, task state, and persisted pending rows.
For a seeded unrelated move, compare its identity and payload before and after the requests.
Add `TestMoveTaskSameStepResponseForwarding` in `internal/mcp/server/config_handlers_test.go` for both MCP modes.
Together these cover the public tool adapter, backend dispatch, and persistence boundaries.

## Work orders

- [x] [Task 01: Complete same-step MCP requests](task-01-complete-same-step-moves.md)

## Verification results

Design validation on 2026-09-22:

- `python3 scripts/list-docs.py validate`: passed (299 decisions, 1114 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/mcp-same-step-move`: passed.
- Catalog discovery includes both new specifications. All four package files were inspected before staging.

Implementation validation on 2026-09-23:

- Handler regressions, including the same-step matrix, passed. The new regression failed before the fix with `deferred` and one pending move, as expected.
- MCP handler, MCP server, workflow move, and orchestrator equal-target regression commands in the work order passed.
- `golangci-lint run ./internal/mcp/handlers ./internal/mcp/server`: passed with 0 issues.
- `go build -o /tmp/kandev-issue3872-final ./cmd/kandev` from `apps/backend`: passed.
- Public docs validation tests passed (62 tests); public docs validation passed (47 pages).
- `python3 scripts/list-docs.py validate`: passed (299 decisions, 1114 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- PR review follow-up: missing and inaccessible targets retain validation errors; operational workflow or step lookup failures now return `internal_error` without exposing repository details.
- Handler, task repository, workflow repository, workflow service, MCP server, workflow move, and orchestrator regressions passed after the follow-up. Lint passed for the four changed Go packages, the backend binary built, and catalog/specification validation passed.

## Risks

- An early return can bypass authorization, target checks, or entry-option validation unless their tests cover it.
- Reusing the requested position in the response can misrepresent stored order.
- Removing pending work can cancel an unrelated accepted transition.
- A concurrent independent move can change the task after the no-op snapshot. This response does not reserve the destination.
