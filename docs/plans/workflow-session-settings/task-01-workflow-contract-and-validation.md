---
id: "01-workflow-contract-and-validation"
title: "Add and validate the workflow session-settings contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow-session-settings/spec.md"
---

# Task 01: Add and Validate the Workflow Session-settings Contract

## Acceptance

- Backend and frontend workflow action types represent `configure_session` rules without provider-specific fields.
- One shared backend validator covers create, update, and import and rejects duplicate actions/families, invalid operations or payloads, empty `set` rules, and coexistence with `agent_profile_id`.
- Valid rules survive snapshot, export, and import byte-for-byte at the semantic JSON level without agent-profile matching.
- Existing workflows without the action retain their current behavior and serialized shape.

## TDD sequence

1. Add failing model/import/controller tests for the accepted and rejected shapes.
2. Add typed parsing/validation helpers and call them from every persistence boundary.
3. Add the frontend discriminated union and serialization tests.
4. Refactor duplicate validation logic only after the focused tests pass.

## Verification

```bash
cd apps/backend && go test ./internal/workflow/models ./internal/workflow/controller ./internal/workflow/service
cd apps && pnpm --filter @kandev/web test -- --run lib/types lib/api/domains/workflow-api
```

## Files likely touched

- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/models/export_test.go`
- `apps/backend/internal/workflow/controller/controller.go`
- Related workflow controller/service tests
- `apps/web/lib/types/workflow-actions.ts`
- `apps/web/lib/types/http.ts`
- Related frontend workflow serialization tests

## Dependencies

None.

## Inputs

- Spec sections `Data model`, `API surface`, and `Persistence guarantees`.
- Existing `OnEnterAction`, workflow-step create/update, and import validation paths.

## Output contract

Report the final typed rule shape, all persistence boundaries using the validator, import/export behavior, files changed, tests run, and any compatibility concern.
