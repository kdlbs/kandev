---
id: "04-browser-data-state"
title: "Browser application protocol"
status: completed
wave: 4
depends_on:
  - "03-isolated-browser-runtime"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-004
  - REQ-PLUGINS-ISOLATED-WEB-APPS-005
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.6
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.7
  - AC-PLUGINS-ISOLATED-WEB-APPS-004.8
  - AC-PLUGINS-ISOLATED-WEB-APPS-005.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-005.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-005.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-005.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-005.5
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 04: Browser application protocol

## Summary

Add the versioned relative data, message, action, and instance-state routes.
Reuse shared Plugin Host services and DTOs.

## In scope

- Extract shared capability-gated Host data services.
- Add `workflow_step_id` to task read DTOs and adapters.
- Add relative task, workflow, workflow-step, and task-message routes.
- Route task updates and messages through normal domain services.
- Add instance JSON state with revision preconditions.
- Add stable safe errors, body limits, deadlines, and cancellation.
- Add browser and gRPC parity contract tests.

## Out of scope

- Live event transport, canvas lifecycle, source publishing, and user interface.

## Acceptance

- Browser and gRPC adapters return equivalent task and workflow data.
- A static application can send a task prompt and move a task through services.
- A stale state write returns the current revision without overwriting state.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/... ./pkg/pluginsdk/...
```

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/pkg/pluginsdk/data_types.go`
- `apps/backend/internal/plugins/host_data_*.go`
- `apps/backend/internal/plugins/host_write.go`
- `apps/backend/internal/plugins/webapp/data_handlers.go`
- `apps/backend/internal/plugins/webapp/state_handlers.go`
- `apps/backend/internal/plugins/action_handlers.go`

## Dependencies

- Task 03 provides the authorized runtime request context.

## Risks

- A browser adapter can bypass service events or scope checks.
- A task projection without its workflow step makes ordered movement ambiguous.
- A delayed mutation response can overwrite a newer event without ordering data.

## Parallelism

`sequential`

## Inputs

- Browser data, shared state, managed action, and failure sections.
- ADR 0043 and current Host data, message, state, and wire tests.

## Results

Completed on 2026-08-28. Added the relative versioned browser protocol for
context, task and workflow data, workflow steps, task messages, actions, and
revisioned instance state. Requests use authenticated capability bindings,
bounded bodies and responses, cancellation deadlines, stable errors, and
optimistic `If-Match` revisions. Focused and package-wide plugin tests pass.
