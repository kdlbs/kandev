---
id: "02-bind-archive-target"
title: "Bind the archive target"
status: done
wave: 2
depends_on: ["01-event-delivery-and-startup"]
plan: "plan.md"
spec: "../../specs/office/requirements/automations-pr-merged-trigger.md"
---

# Task 02: Bind the Archive Target

## Intent

Make the event-selected task a backend-enforced archive boundary while preserving the generic MCP
tool for unrelated callers.

## Acceptance

- A merged-PR run task persists the validated event target in server-owned metadata.
- The MCP server injects its bound task id into the backend request without advertising a caller-id
  argument.
- A matching archive request succeeds; a different owner-reachable target is rejected without any
  mutation.
- Missing or malformed binding metadata fails closed.
- Ordinary sessions and other automation trigger types retain existing archive behavior.

## TDD sequence

1. Add orchestrator tests proving only `github_pr_merged` run tasks persist the target metadata.
2. Add MCP server tests proving the current task id is injected and cannot be supplied through the
   public schema.
3. Add handler tests for match, mismatch, missing/malformed metadata, ordinary caller, and other
   automation-trigger caller; confirm the mismatch is RED against current behavior.
4. Introduce `automation_target_task_id`, persist the target, inject `caller_task_id`, and enforce the
   check at `handleArchiveTask` before `ArchiveTask`.
5. Extend the scripted automation E2E to attempt a wrong same-owner target and verify both tasks stay
   unchanged.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_automation_test.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/mcp/server/config_handlers_test.go`
- `apps/backend/internal/mcp/handlers/config_task_handlers.go`
- `apps/backend/internal/mcp/handlers/config_handlers_test.go`
- `apps/web/e2e/tests/automations-pr-merged-trigger.spec.ts`

## Dependencies

Task 01, so end-to-end checks exercise the repaired event path.

## Parallelism

`sequential` — the persistence, transport context, and backend enforcement form one security boundary.

## Verification

- `cd apps/backend && go test ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server`
- `cd apps/web && pnpm e2e:run --project chromium tests/automations-pr-merged-trigger.spec.ts`

## Risks

- Never trust a caller id received as a public MCP argument.
- Keep owner authorization in place; target binding narrows authority and does not replace it.
- Treat legacy/malformed metadata as an error for this trigger, without panicking on untyped values.

## Completed validation

- RED: the mismatch handler test failed before the caller/target binding was implemented.
- GREEN: orchestrator metadata, MCP caller-id injection/schema, and match/mismatch/missing-binding
  handler tests pass.
- GREEN: `cd apps/backend && go test ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server`.
- GREEN: the Chromium automation E2E includes a same-owner wrong-target archive attempt and passes
  with the full 15-test merged-PR spec.

## Implementation notes

The target task id is stored under `automation_target_task_id`; the current MCP server task is
injected as private `caller_task_id` context. Only merged-PR automation run callers are narrowed by
the new guard; ordinary callers and other trigger types keep the generic archive behavior.

## Output contract

Report RED/GREEN evidence, the enforced metadata/transport contract, test results, and compatibility
notes. Update this task and `plan.md` status in the same conversation.
