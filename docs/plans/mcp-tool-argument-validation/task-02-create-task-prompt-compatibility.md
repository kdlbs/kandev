---
id: "02-create-task-prompt-compatibility"
title: "Converge create-task on prompt"
status: done
wave: 2
depends_on: ["01-enforce-registered-schemas"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/mcp-tool-argument-validation.md"
---

# Task 02: Converge create-task on prompt

## Acceptance

- `create_task_kandev` advertises `prompt` and forwards the text unchanged to the existing backend `description` payload.
- Existing callers may continue sending unadvertised legacy `description`; it is normalized to `prompt` before validation.
- Calls containing both `prompt` and `description`, or any other unknown key, return a tool error without backend dispatch.
- The registered create-task schema and description advertise only canonical `prompt`, so compatibility does not add a second schema property.

## Verification

Follow strict TDD, then run:

```bash
make -C apps/backend test
```

## Files likely touched

- `apps/backend/internal/mcp/server/tool_argument_validation.go`
- `apps/backend/internal/mcp/server/handlers_test.go`

## Dependencies

- Task 01 provides the normalization/validation boundary and compiled create-task schema.

## Parallelism

`sequential` — this extends the shared validation path established by Task 01.

## Inputs

- The create-task scenarios in the spec.
- Issue #2123 reproduction: `description` contained a short label while unknown `prompt` contained the real instructions.
- Existing `createTaskHandler` forwarding to backend `description`.

## Risks

- Shallow-copy only when the legacy alias must be normalized so the original request cannot be mutated across hooks or logs.
- Do not advertise the legacy alias or weaken rejection for any other unknown key.

## Output contract

Report red/green evidence, files changed, exact test result, blockers or residual risks, and task/plan status updates in the primary conversation.
