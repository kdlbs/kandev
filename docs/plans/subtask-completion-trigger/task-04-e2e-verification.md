---
id: "04-e2e-verification"
title: "E2E verification"
status: done
wave: 3
depends_on: ["03-orchestrator-trigger-dispatch"]
plan: "plan.md"
spec: "../../specs/tasks/subtask-completion-trigger.md"
---

# Task 04: E2E verification

## Acceptance

- An E2E test creates a parent task whose workflow step defines
  `on_children_completed`.
- At least two child tasks are created with `parent_id` pointing at the parent.
- The parent stays on its current step after the first child completes while a
  sibling remains non-terminal.
- When all children complete, the parent advances according to the configured
  `on_children_completed` action without any parent polling loop.

## Verification

```bash
cd apps/web && pnpm e2e workflow-children-completed.spec.ts
make fmt
make typecheck test lint
```

## Files Likely Touched

- `apps/web/e2e/tests/workflow/workflow-children-completed.spec.ts`

## Dependencies

- `03-orchestrator-trigger-dispatch`

## Inputs

- Spec: `docs/specs/tasks/subtask-completion-trigger.md`, `Scenarios`.
- Existing E2E patterns: `apps/web/e2e/tests/task/subtask.spec.ts`,
  `apps/web/e2e/helpers/api-client.ts`, and mock-agent MCP script examples.

## Output Contract

When finished, update this task frontmatter to `status: done`, update the
checkbox in `plan.md`, and report:

- Summary of E2E coverage.
- Files changed.
- Tests run and their result.
- Any blockers or follow-up risks.
