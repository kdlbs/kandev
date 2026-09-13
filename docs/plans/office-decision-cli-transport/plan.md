---
created: 2026-09-07
status: in_progress
requirements:
  - REQ-TASKS-QUORUM-RECORDING-001
system_design:
  - ../../specs/tasks/system-design/workflow-quorum-decision-recording.md
legacy_specs: []
---

# Implementation Plan: Office Decision CLI Transport

## Overview

Move Office workflow-step decisions from the Office MCP catalog to the signed,
task-bound Office runtime CLI. Delivery is sequential: establish and verify the
CLI/API path, teach eligible seat holders through an injected skill and prompt,
then remove the old MCP transport and update its inventories.

## Scope

### In scope

- Add `$KANDEV_CLI kandev task decision` and a closed Office runtime endpoint.
- Derive task, session, and agent identity from the signed run context.
- Delegate through `DashboardService.RecordAgentDecision` and the existing
  workflow-engine decision path.
- Inject decision guidance for reviewer and approver seat holders.
- Remove `record_step_decision_kandev` from Office MCP registration, context,
  websocket transport, and transport-only tests.
- Update internal and public Office MCP/CLI inventories.

### Out of scope

- Changes to quorum thresholds, seat canonicalization, persistence, reactivity,
  or transition semantics.
- Changes to the separate Office approval-inbox command.
- A Kanban decision command or any Kanban MCP profile change.

## Technical approach

### Signed runtime API and CLI

Add `task decision` to `apps/backend/cmd/agentctl/kandev_task.go`. It accepts
only `--decision` and `--reason` and posts to
`/api/v1/office/runtime/task/decision` using the existing runtime client.

The runtime handler derives the task, session, and agent from the validated JWT.
Its request body is closed over `decision` and `reason`, so identity-shaped or
unknown fields fail instead of being ignored. A small adapter at the Office
composition boundary translates runtime DTOs to
`dashboard.RecordAgentDecisionInput` and maps the existing structured result
back to JSON. Validation, live role/seat resolution, persistence, events,
reactivity, and quorum advancement remain in `DashboardService` and the engine.

### Seat-specific skill and prompt

Add the bundled `kandev-step-decision` system skill without assigning it as a
static role default. When `RunContext.AvailableActions` contains the
launch-time decision affordance, the scheduler adds that skill to the run's
manifest. Review and approval prompts name the CLI command as the final action.
The endpoint still performs live authorization, so a stale launch-time seat
snapshot cannot grant access.

### MCP retirement

After the first two work orders pass, remove the Office decision tool group,
tool implementation, websocket action and handler wiring, and transport-only
tests. Keep the service and engine regression suites. Reduce the documented
Office tool count from 12 to 11 and remove the decision tool from first-turn and
public inventories. Add negative assertions for both Office MCP schema absence
and Kanban instruction/metadata isolation.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-QUORUM-RECORDING-001.1` | CLI request and seat-specific skill/prompt tests |
| `AC-TASKS-QUORUM-RECORDING-001.2`–`.5` | Runtime identity forwarding plus existing dashboard seat/persistence/quorum tests |
| `AC-TASKS-QUORUM-RECORDING-001.6`–`.8` | CLI/API validation and existing no-write dashboard tests |
| `AC-TASKS-QUORUM-RECORDING-001.9` | Kanban prompt/context isolation regression |
| `AC-TASKS-QUORUM-RECORDING-001.10`–`.11` | Runtime adapter and existing dashboard structured-result tests |
| `AC-TASKS-QUORUM-RECORDING-001.12` | Office MCP inventory and exact first-turn context tests |

## Work orders

- [done] [Task 01: Establish the task-bound decision transport](task-01-task-bound-decision-transport.md)
- [done] [Task 02: Inject the decision skill](task-02-inject-decision-skill.md)
- [done] [Task 03: Retire the decision MCP transport](task-03-retire-decision-mcp.md)

## Verification results

- Task 01: `cd apps/backend && rtk go test ./cmd/agentctl ./internal/office/runtime ./internal/office/dashboard -run 'Test(TaskDecision|RuntimeHandler_RecordAgentDecision|RecordAgentDecision)' -count=1` — passed (18 tests).
- Task 02: `cd apps/backend && rtk go test ./internal/office/configloader ./internal/office/skills ./internal/office/service -run 'Test.*(DecisionSkill|DecisionContract|ReviewStage|ApprovalStage|SkillManifest|SystemSkills)' -count=1` — passed (36 tests); full affected packages passed (564 tests).
- Task 03: `cd apps/backend && rtk go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp -run 'Test.*(ModeOffice|ModeTask|Sysprompt|Decision|HandlerRegistration)' -count=1` — passed (22 tests); public docs tests and validator passed (61 tests, 46 pages).

## Risks

- Importing `office/dashboard` from `office/runtime` would create a package
  cycle; the composition adapter must keep DTO translation at `internal/office`.
- A permissive JSON binder could silently ignore a forged `task_id`; the new
  endpoint needs a closed request decoder.
- Static role-based skill assignment cannot represent live participant seats;
  injection must follow the seat-derived advisory action and retain live checks.
