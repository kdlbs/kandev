---
id: "01-bind-execution-profile"
title: "Bind the Office execution profile"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENTS-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-OFFICE-AGENTS-001.2
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.4
system_design:
  - ../../specs/office/system-design/agents-01.md
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
---

# Task 01: Bind the Office execution profile

## Summary

Persist the selected execution profile on the stable Office identity and resolve
dynamic candidates through it for task-backed and taskless launches, leaving a
task actionable when resolution fails.

## In scope

- `execution_agent_profile_id` column, model field, and replayable migration in
  the shared settings store.
- Binding on Office onboarding and profile configuration for a dynamic source.
- Shared dynamic resolver source selection through the binding.
- Office taskless launcher concrete-candidate resolution.
- Orchestrator restore of the pre-`SCHEDULING` state on resolution failure.
- Focused Go tests alongside each behavior.

## Out of scope

- Concrete-profile behavior and empty-binding compatibility.
- Public API, logical Office identity, and provider routing policy ownership.
- Unrelated timezone or UI changes.

## Acceptance

- A selected dynamic execution profile is persisted and survives restart.
- Launch resolution selects a concrete candidate through the bound profile while
  the Office ID remains the logical agent/session owner.
- A resolution failure leaves the task/run retryable and never launches the
  virtual family.
- Existing concrete selections and blank bindings behave as before.

## Verification

```bash
cd apps/backend && go build ./...
cd apps/backend && go test -p 2 ./internal/agent/runtime ./internal/agent/settings/store \
  ./internal/office/agents ./internal/office/onboarding ./internal/backendapp ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/agent/settings/models/models.go`
- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/office/agents/service.go`
- `apps/backend/internal/office/onboarding/service.go`
- `apps/backend/internal/office/repository/sqlite/agents.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/office_run_session_launcher.go`
- `apps/backend/internal/orchestrator/task_operations.go`

## Dependencies

None.

## Risks

- The binding must remain a routing source only, never a session or cost owner.
- The migration must be idempotent and preserve existing rows with an empty
  binding.
