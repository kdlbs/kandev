---
id: "02-workflow-event-contracts"
title: "Workflow event contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/subtask-completion-trigger.md"
---

# Task 02: Workflow event contracts

## Acceptance

- Workflow step API responses preserve `on_children_completed` and other
  existing generic trigger fields instead of dropping them.
- Frontend workflow event types accept `on_children_completed` generic actions.
- E2E API helpers can configure `on_children_completed` without type escapes.

## Verification

```bash
make -C apps/backend test ARGS="./internal/task/dto/..."
cd apps && pnpm --filter @kandev/web typecheck
```

## Files Likely Touched

- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/converters.go`
- `apps/backend/internal/task/dto/converters_test.go`
- `apps/web/lib/types/workflow-actions.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/e2e/helpers/api-client.ts`

## Dependencies

None.

## Inputs

- Spec: `docs/specs/tasks/subtask-completion-trigger.md`, section
  `API surface`.
- Existing backend model: `apps/backend/internal/workflow/models/models.go`.
- Existing frontend type file: `apps/web/lib/types/workflow-actions.ts`.

## Output Contract

When finished, update this task frontmatter to `status: done`, update the
checkbox in `plan.md`, and report:

- Summary of backend DTO and frontend type changes.
- Files changed.
- Tests/typechecks run and their result.
- Any blockers or follow-up risks.
