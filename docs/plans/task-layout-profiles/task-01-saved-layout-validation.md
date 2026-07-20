---
id: "01-saved-layout-validation"
title: "Saved layout validation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 01: Saved Layout Validation

## Acceptance

- `saved_layouts` rejects blank IDs, duplicate IDs, and more than one custom default.
- Existing count/name checks remain intact, and lists with zero or one custom default are accepted.
- The existing GET/PATCH representation remains unchanged.

## Files likely touched

- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`

## Dependencies

None.

## Inputs

- Spec `Data model` and `API surface` sections.
- Existing `applySavedLayouts` and its table-driven tests.

## Verification

```bash
go test -v -run TestApplySavedLayouts ./internal/user/service/...
```

Run from `apps/backend`.

## Output contract

Report validation behavior, tests run, files changed, blockers, and risks; mark this task and its plan item done.
