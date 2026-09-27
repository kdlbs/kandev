---
created: 2026-09-19
status: complete
requirements:
  - REQ-TASKS-SELF-SIBLING-001
  - REQ-TASKS-SELF-SIBLING-002
system_design:
  - ../../specs/tasks/system-design/mcp-self-sibling-placement.md
legacy_specs: []
---

# Implementation Plan: MCP self sibling placement

## Overview and approval

Deliver the automatic sibling behavior selected for
[issue #3773](https://github.com/kdlbs/kandev/issues/3773). The
[requirements](../../specs/tasks/requirements/mcp-self-sibling-placement.md) and
[design](../../specs/tasks/system-design/mcp-self-sibling-placement.md) form the
approved implementation contract. The maintainer authorized implementation on
2026-09-21 and renewed that authorization on 2026-09-24. Implementation and
work-order validation are complete. The changes remain uncommitted for review.

One sequential vertical work order covers intent propagation, authenticated
resolution, result reporting, regression evidence and public documentation.
These pieces must land together so agents never receive an unexplained change
of coordination owner.

## Scope

In scope: session-bound Kanban literal-self fallback, additive explanatory
result, current inheritance/attribution, permission and idempotency regression
coverage, task-mode schema descriptions and public reference updates.

Out of scope: UI, deeper Kanban hierarchy, independent delegator identity,
Office creation enablement, external-client self support, generic depth-error
recovery, data migration, contributor-branch changes and GitHub publication.

## Technical approach

1. Preserve literal-self intent as internal `parent_is_self` in
   `internal/mcp/server/handlers.go`; keep explicit UUID traffic unmarked.
2. Extend `mcpCreateTaskRequest` and admission metadata in
   `internal/mcp/handlers/create_task_mode.go`. Resolve only after verified
   Kanban identity and before effective-destination admission. Keep the
   generic service and repository-resolution depth guards strict.
3. Decorate every successful `mcpCreateTaskResult` branch with optional
   `parent_resolution` when redirected. Keep persisted parent, source session,
   idempotency flags and existing launch behavior authoritative.
4. Update `registerCreateTaskTool` task-mode descriptions in server/server.go
   and the existing coordination/MCP public docs. Do not advertise the internal
   field or change external-mode semantics.

Use a small dedicated placement file instead of enlarging handlers.go with
conditional trees. This is a local helper, not a new service or policy engine.
No schema changes. Verify exact symbols against current source before edits.

## Tests and acceptance mapping

The table maps the implemented evidence. The work order records exact commands
and validation results. Paths below are relative to `apps/backend/internal/mcp`.

| Evidence | Acceptance |
| --- | --- |
| `server/create_task_self_placement_test.go`: `TestCreateTaskSelfPlacementIntent`, `TestCreateTaskSelfPlacementRejectsPublicMarker`, `TestCreateTaskSelfPlacementExternalBoundary`, `TestCreateTaskSelfPlacementResult`, `TestCreateTaskSelfPlacementExistingResult`, `TestCreateTaskSelfPlacementToolDescription` | 001.1, 001.2, 001.4, 002.4, 002.5 |
| `handlers/create_task_self_placement_test.go`: `TestMCPCreateTaskSelfPlacement`, `TestMCPCreateTaskSelfPlacementExplicitParentKeepsDepthGuard`, `TestMCPCreateTaskSelfPlacementOmittedParent`, `TestMCPCreateTaskSelfPlacementRejectsForgedMarker`, `TestMCPCreateTaskSelfPlacementRootAndBoundaryCallers`, `TestMCPCreateTaskSelfPlacementDeduplicatedResultExplainsActualParent` | 001.1, 001.2, 001.4, 002.4 |
| `handlers/create_task_self_placement_admission_test.go`: `TestMCPCreateTaskSelfPlacementAdmission`, `TestMCPCreateTaskSelfPlacementParentReadFailure`, `TestMCPCreateTaskSelfPlacementEphemeralCaller` | 001.3, 002.3, 002.4 |
| `handlers/create_task_self_placement_inheritance_test.go`: `TestMCPCreateTaskSelfPlacementInheritance` with distinguishable parent/caller repositories, workflow, materialized group, profile and runtime settings | 002.1, 002.2 |
| `handlers/create_task_self_placement_outcomes_test.go`: `TestMCPCreateTaskSelfPlacementOutcomes`, `TestMCPCreateTaskSelfPlacementIdentityLost`, `TestMCPCreateTaskSelfPlacementFailures` | 001.3, 001.4, 002.2, 002.3 |
| Same outcomes file: `TestMCPCreateTaskSelfPlacementLaunch` for start=false, one launch, blocker-deferred start and disabled deferred start | 002.3 |
| `server/create_task_self_placement_integration_test.go`: `TestCreateTaskSelfPlacementJourney`, including real scope resolution, ledger attribution, one launch and retry | 001.1, 001.2, 001.4, 002.1-002.4 |
| Existing service depth, parent access, stop/message/question and children-completed tests | 001.2, 002.2, 002.4 |

Full IDs use prefixes `AC-TASKS-SELF-SIBLING-001` and
`AC-TASKS-SELF-SIBLING-002` from the linked requirement.

## End-to-end evidence

`server/create_task_self_placement_integration_test.go` contains
`TestCreateTaskSelfPlacementJourney`: real MCP tool argument validation through
`NewDispatcherBackendClient`, registered handlers, trusted principal and a
temporary SQLite task service. Follow the dispatcher/service pattern in
`task_plan_safe_edits_integration_test.go`, supplying creation identity and
profile fixtures from handler tests. Assert one call from C creates S under P,
source attribution is C's session, output describes the fallback, a retry does
not relaunch or duplicate, and the explicit C UUID is rejected. This covers
001.1, 001.2, 001.4 and 002.1-002.4 through the consumer boundary. Use a launcher
spy; do not start a real agent or create persistent Kandev tasks.

No Playwright or mobile layout checks: this package changes MCP behavior and
agent-facing diagnostics, not a rendered interface.

## Work orders

- [x] [Task 01: Implement and verify self placement](task-01-self-placement.md)

## Verification results

Implementation and all nine acceptance criteria are covered. Full MCP package
tests, selected service/orchestrator regressions, focused race tests, changed-code
lint, formatting, specification checks and public-doc validators passed on
2026-09-24. Exact commands and results are in Task 01. Earlier triage's five
passing tests prove the old behavior only; they are not implementation evidence.

## Risks and compatibility

- Losing the self marker changes explicit-ID semantics. Test both paths.
- Resolving after admission can authorize the wrong destination. Normalize
  within authenticated admission before destination checks and inheritance.
- Claiming a deduplicated task was created or reparented misleads agents. Keep
  request-resolution metadata distinct from actual returned task parent.
- Creator runtime/profile precedence differs from parent scope inheritance.
- Mixed server/backend versions retain the prior depth error until both halves
  support the marker. Do not add heuristic fallback for old clients.
- Parent completion aggregates C and S with all other active siblings. There
  is no per-creator completion signal.

## Decisions and handoff

No unresolved product decision blocks this approved package. The maintainer
selected automatic self fallback; inheritance and control use existing contracts.
The implementation authorization is recorded in the Kandev task context.

Worktree: `/workspace`; branch: `feature/issue-3773-define-de-8d9203`;
planning base: `794773dd6887ae5c326ee77757dedd7bafb1ecf7`.
Source intake owner remains `zeval`, reverified as the sole assignee on
2026-09-24; no implementation PR/head exists yet.

The Kandev task context owns coordinator/worker session IDs and dispatch state.
The user promoted the designated implementation session to coordinator; work
continues in that existing session. Do not create extra tasks, workers or
worktrees for this package. Preserve current files and recheck ownership before
resuming substantive implementation.

## Recovery and current checkout

On 2026-09-22, the five original package documents were recovered from retained
tool responses in session `4f2d32d7-62bb-4431-a544-c27945286d3b`. Those responses
contained the old coordinator's successful document-writing commands:
`6f76755b-b27a-42a6-b4ba-2cc8840d4f28` (requirements, design and ADR) and
`b53ee892-8894-4800-b8b9-b718869d0991` (plan and work order). The literal document
strings were extracted without executing the historical commands. Recovery
preserved the nine acceptance criteria and technical design. Approval and
delivery status were reconciled with the task plan after restoration. Later edits in
the old checkout could not be recovered byte for byte.

Current checkout: `/workspace`, branch `feature/issue-3773-define-de-8d9203`,
base `10726dfe8ea41240846db4a6d2ba26ebe9ce9c1a`. This is distinct from the old
coordinator's planning checkout at `794773dd6887ae5c326ee77757dedd7bafb1ecf7`.
The earlier implementation-complete report was superseded during recovery.
The user renewed implementation authorization on 2026-09-24. The current work
order records the completed acceptance coverage and final checks from this
checkout. All 18 package, implementation, test and public-documentation files
remained uncommitted at the implementation checkpoint.

## Publication authorization

The user authorized opening the PR on 2026-09-26 and requested continuation on
2026-09-27. This authorizes the normal commit, push and PR workflow for this
package. The current checkout contains all 18 files; the Go diff still matches
the patch validated on 2026-09-24. A later review session reported a different
checkout with no implementation, so publishing this branch supplies a durable
review target. Publication identifiers and hook receipts belong in the Kandev
task handoff. No merge or issue closure is authorized by this request.
