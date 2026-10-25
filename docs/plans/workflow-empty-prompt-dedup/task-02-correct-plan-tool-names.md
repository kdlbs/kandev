---
id: "02-correct-plan-tool-names"
title: "Correct plan-tool names"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-TOOL-NAMES-001
acceptance_criteria:
  - AC-TASKS-MCP-TOOL-NAMES-001.3
system_design:
  - ../../specs/tasks/system-design/mcp-tool-name-stability.md
---

# Task 02: Correct Plan-Tool Names

## Summary

Correct the two canonical MCP names in active-plan context. Add focused coverage for the model-facing instruction.

## In scope

- Replace `plan_get` with `get_task_plan_kandev`.
- Replace `plan_update` with `update_task_plan_kandev`.
- Add a pure-helper unit test for both names.

## Out of scope

- MCP registration and transport behavior.
- User-interface copy or localization catalogs.
- Backend prompt composition.

## Acceptance

- Active-plan context names only registered canonical plan tools.
- The focused test rejects the two obsolete names.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- --run hooks/use-message-handler.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/use-message-handler.ts hooks/use-message-handler.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/hooks/use-message-handler.ts`
- `apps/web/hooks/use-message-handler.test.ts`

## Dependencies

None.

## Risks

- Exporting the helper increases the module surface. Keep the export limited to the existing pure function.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-MCP-TOOL-NAMES-001`
- Prompt consistency in the MCP tool-name system design.
- Canonical names in `apps/backend/config/prompts/plan-mode.md`.

## Results

- Corrected the active-plan context to name `get_task_plan_kandev` and
  `update_task_plan_kandev`.
- Exported the existing pure context builder only for focused unit coverage;
  no user-facing copy or localization changed.
- `cd apps && pnpm --filter @kandev/web test -- --run hooks/use-message-handler.test.ts` passed with 27 tests.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm exec eslint hooks/use-message-handler.ts hooks/use-message-handler.test.ts` passed.
- `cd apps/web && pnpm run i18n:check` passed.
- `cd apps/web && pnpm run i18n:ratchet` passed with 0 added violations.
