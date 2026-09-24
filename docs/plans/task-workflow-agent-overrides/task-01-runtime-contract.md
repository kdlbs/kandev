---
id: "01-runtime-contract"
title: "Persist and apply task overrides"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.2
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.3
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.4
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.5
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.9
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.10
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.11
  - AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.12
system_design:
  - ../../specs/tasks/system-design/workflow-agent-overrides.md
---

# Task 01: Persist and apply task overrides

## Summary

Add the optional create contract and durable task map. Resolve fixed workflow
profiles through that map on every execution path.

## In scope

- Add the typed record, replayable migration, task persistence, and DTO projections.
- Validate HTTP/WS and service requests before writes. Keep old callers compatible.
- Integrate task-aware resolution into initial launch, ensure, preflight, preview,
  and manual/automatic transitions. Preserve recipient bindings and lifecycle policies.
- Preserve existing external-ID replay and prevent implicit child inheritance.

## Out of scope

UI controls, public docs, and new MCP parameters belong outside this work order.

## Acceptance

1. Valid overrides survive reload/restart and unrelated updates. Old tasks route unchanged.
2. Invalid input causes no task/session write. Runtime replacement failure never falls back silently.
3. All routing paths agree. Initial Agent remains A; Implement uses B; Review returns to A; PR reuses B's Implement session.

## Verification

Use TDD. Add focused cases in `workflow_agent_overrides_test.go` beside each
changed service/repository/model boundary. Extend existing handler and
`event_handlers_workflow_profile_test.go` patterns. Include same-profile `new`,
nonrecursive A-to-B/B-to-C, another workflow, deferred start, terminal sessions,
dynamic-profile compatibility, unavailable replacements, and external-ID replay.
Add a three-task isolation case in one workflow: default profile, replacement B,
and replacement C. Interleave routing and repeat after repository reload. Assert
that each task retains its own profile and the shared workflow remains unchanged.
Extend `workflow_move_preview_test.go` to prove read-only effective model/profile
projection, existing session configuration precedence, unknown recipients, and
three-task isolation. Compare projected recipients with actual routing.
Use dialect-aware migration tests and existing PostgreSQL schema test patterns.

```bash
(cd apps/backend && go test ./internal/task/models ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/orchestrator)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/models/models.go` and a focused override helper/test file.
- `apps/backend/internal/task/repository/sqlite/task.go`, `base.go`, and related migrations/scanners.
- `apps/backend/internal/task/service/service_requests.go` and `service_tasks.go`.
- `apps/backend/internal/task/handlers/task_http_handlers.go` and the WS create adapter.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`, `session_ensure.go`, and `task_operations.go`.
- `apps/backend/internal/orchestrator/workflow_move_preview.go` and its tests.
- Existing task DTO projections and focused tests for every changed boundary.

## Dependencies

None. Read the requirement and full system design first.

## Risks

Do not mutate shared workflow steps to implement substitution. Do not change a
live session's profile in place. A task-specific result cannot use a workflow-only cache.

## Parallelism

`sequential`

## Inputs

System design: Data and persistence, Creation validation, and Runtime routing.
Existing fixed-profile routing and profile-session lifecycle specifications.

## Confirmed interview cases

- After task creation, change Implement's shared fixed profile from Luna to Sol.
  The overridden task still uses Terra. A task without an override uses Sol.
- Before Implement starts, both PR disclosures show Terra as planned. After
  its session exists, both show its actual effective model and session behavior.
- A new workflow step does not inherit an existing task's replacement.

## Results

Implemented typed task-owned persistence with expanded step bindings, creation
validation, DTO projection, task-aware execution and preview resolution, and
session-target binding preservation. Added migration, repository, service,
handler, orchestrator, preview, and three-task isolation coverage. The exact
backend package suite passed. Review remediation added fail-closed task reads
through routing, credential preflight, and start, plus executor compatibility
validation at the create boundary. HTTP/WS and service tests prove rejected
replacements leave task and session storage unchanged; the full orchestrator
package and backend build also pass.
