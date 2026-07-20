---
id: "02-task-selector-ux"
title: "Render the task-only compact configuration summary"
status: done
wave: 2
depends_on: ["01-backend-contract-and-baseline"]
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 02: Render the Task-only Compact Configuration Summary

## Acceptance

- Task chat/context selectors render model plus every changed non-model value in ACP order using ` / `.
- Shared/profile selector triggers continue listing every selected value.
- Provider descriptions render when open and are accessible from the closed task trigger on pointer and keyboard devices; mobile users reach them by opening the selector.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/model-config-selector.test.tsx components/task/model-selector.test.ts lib/ws/handlers/session-models.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/ws/handlers/session-models.ts`
- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/task/model-selector.tsx`
- Related frontend tests and backend payload types

## Dependencies

Task 01.

## Inputs

- Spec scenarios for unchanged, one changed, several changed, reset, profile settings, and mobile.
- Existing `triggerLabel`, `ModelConfigSelector`, and task `ModelSelector` patterns.

## Output contract

Report the opt-in API used by task surfaces, label comparison behavior, accessibility/mobile behavior, files changed, tests run, and remaining visual risks.
