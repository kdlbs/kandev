---
created: 2026-09-11
status: implemented
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-001
  - REQ-TASKS-MCP-WORKSPACE-MODE-002
  - REQ-TASKS-MCP-WORKSPACE-MODE-003
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
legacy_specs: []
---

# Implementation plan: MCP workspace modes

## Overview

Restrict in-session Kanban MCP creation to Kanban destinations. Keep Office
agents on skills and CLI. Preserve external MCP creation and management of
both workspace modes through existing tools.

The user confirmed the Office CLI boundary on 2026-09-11. The earlier proposal
for an Office MCP creation tool and external tool-name migration is withdrawn.

## Scope

In scope: trusted creation admission, destination preflight, Office catalog
regression coverage, external compatibility tests, and documentation.

Out of scope: new tools, Office runtime adapters, new assignment fields,
scheduler changes, external schema changes, UI, or migrations.

## Technical approach

Use the existing principal and external-transport marker at
`handleCreateTask`, before repository resolution. Validate source and
destination workspace identity with `Workspace.OfficeWorkflowID` and existing
task classification. Keep the gate separate from native Office creation and
generic management services.

See the [requirements](../../specs/tasks/requirements/mcp-workspace-mode.md)
and [system design](../../specs/tasks/system-design/mcp-workspace-mode.md).

## Tests

- 001.1, 001.3 through 001.6, 002.2: `handlers/create_task_mode_test.go`
  covers explicit and inherited targets, spoofing, missing identity, disabled
  authentication, foreign references, no side effects, and profile/repository
  inheritance.
- 001.2, 002.1, 002.3 through 002.5: the Office catalog test and direct
  backend denial test preserve the skills and CLI boundary; existing Office
  runtime tests remain green.
- 003.1 through 003.5: the external handler mode matrix, the real
  `backendapp.TestExternalMCPTaskModesReachPersistenceAndManagement` HTTP
  composition test, external dispatcher transport test, and existing external
  MCP HTTP tests preserve the existing tool name, schemas, and management
  surface.

Criterion suffixes above refer to `AC-TASKS-MCP-WORKSPACE-MODE-`.

## End-to-end evidence

Use the Go external MCP HTTP harness and disposable backend composition.
Drive discovery and tool calls, then inspect persisted state and events.
Cover Kanban/Office external creation, external-ID retries, foreign targets,
missing external workspace selection with mixed modes, and retained management
operations.
No browser test is required because rendered UI does not change.

## Work orders

- [x] [01: Enforce creation admission](task-01-creation-admission.md)
- [x] [02: Preserve Office CLI creation](task-02-office-creation.md)
- [x] [03: Preserve external task management](task-03-external-management.md)

Order: 01 -> 02 -> 03. All work orders are complete and ran sequentially.

## Verification results

Implementation and validation complete. The admission gate runs before
repository resolution and rejects untrusted, conflicting, Office-session, and
wrong-mode Kanban requests without task creation. Verified Kanban principals
bind omitted source identity to the caller session, preserving creator profile,
runtime, and genesis-ledger attribution. Found external-ID results are
revalidated for workspace visibility and task mode before any DTO is returned,
including settled and unsettled legacy mixed-mode Office collisions. Office
catalog and direct action tests preserve the skills and CLI boundary. External
creation remains available for both workspace modes, with real HTTP persistence
and management composition coverage, while fabricated source identity is
rejected.

Verification:

- `go test ./internal/mcp/handlers -count=1`: 644 passed.
- `go test ./internal/mcp/server ./internal/backendapp ./internal/integration -count=1`: 1,382 passed, including the external HTTP composition and MCP WebSocket integration scenarios.
- `go build ./...`: passed.
- Scoped `golangci-lint`: no issues.
- Public docs tests and validator: 62 tests passed; 46 pages validated.
- Specification tests and lint: 36 tests passed; all specification files passed.
- `git diff --check`: passed.

A repository-wide `go test ./... -count=1` audit was also completed with the
running instance's launcher variables removed. The changed MCP, backendapp,
integration, and Office packages passed. Only the two existing
real-process-tree probe tests (`TestProbeRealTree_AllDescendantsPreTurn_Settled`
and `TestProbeRealTree_NewDescendantAfterTurnStart_Live`) remained
environment-sensitive failures; no orchestrator package failure remained.
Those unrelated probe failures are not part of this work order.

## Risks

- Missing principal must never imply external authority.
- The shared handler also serves automation; preserve its existing guard.
- Test fixtures must model actual transport identity instead of requiring a bypass.
- Office sessions must be denied even when a stale tool or direct action is used.
- External Office calls must not inherit the Kanban session restriction.
- Repository registration and external-ID data lookup can precede task insertion;
  admission must precede those effects too.
