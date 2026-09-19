---
id: "03-runtime-rule-application"
title: "Apply matching workflow rules to the original session"
status: done
wave: 3
depends_on: ["01-workflow-contract-and-validation", "02-original-session-initialization"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-session-settings.md"
---

# Task 03: Apply Matching Workflow Rules to the Original Session

## Acceptance

- Runtime matching uses the original session's immutable `agent_name` and applies at most one rule.
- No match and `keep` are silent no-ops; neither writes a runtime override.
- `set` changes only named fields, while `restore_original` attempts the captured model and every still-advertised select option.
- Rules never create, switch, reactivate, or rename a session. A non-original active session is left untouched and receives a visible workflow warning.
- Running-session fields apply independently through ACP and successful values persist through the task service as explicit overrides.
- Start-step rules run through the layered initialization path after original capture and before the first prompt.
- Transition rules run after any context reset and before plan/mode setup and auto-start prompt dispatch.
- Unsupported/rejected fields and persistence failures produce one sanitized visible warning with retained values; successful fields remain applied and auto-start continues.
- Missing legacy snapshots and removed restore options warn and no-op/partially restore exactly as specified.

## TDD sequence

1. Add failing orchestrator tests for match, no-match, keep, set, restore, wrong active session, and start-step ordering.
2. Add the narrow runtime setter/result and task-service override-writer seams, then wire them in `backendapp`.
3. Implement best-effort field application and sanitized warning aggregation.
4. Add failure, persistence, restart, reset-before-config, and auto-start-continuation tests.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/... ./internal/agent/runtime/... ./internal/task/service ./internal/backendapp/...
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/agent/runtime/runtime.go`
- `apps/backend/internal/agent/runtime/facade.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- Related mocks and integration tests

## Dependencies

Tasks 01 and 02.

## Inputs

- Spec sections `State machine`, `Failure modes`, and `Scenarios`.
- Existing `processOnEnter`, `StartCreatedSession`, ACP model/config setters, runtime override persistence, routing-error sanitization, and session-message patterns.

## Output contract

Implemented family matching against the immutable original session, launch-time durable layering, live ACP model/option updates, best-effort field persistence, and sanitized warning messages. `keep` and no-match are no-ops; restore uses the immutable snapshot; non-original and passthrough sessions are never switched. Start-step and on-enter ordering are covered by orchestrator tests, including persistence and restore behavior. No provider-specific branches were added; providers without the optional ACP option setter receive a visible warning for those fields.
