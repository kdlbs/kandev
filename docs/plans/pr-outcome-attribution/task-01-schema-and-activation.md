---
id: "01-schema-and-activation"
title: "Add outcome storage and activation"
status: done
wave: 1
plan: "plan.md"
spec: "../../specs/integrations/requirements/pr-outcome-attribution.md"
---

# Task 01: Schema and activation

Add the five nullable outcome columns to `github_task_prs`, expose them on
`TaskPR`, and add the one-time activation marker in `kandev_meta`.

## Implementation

- Add `ReadMetaKey` and `WriteMetaKeyIfAbsent` in
  `apps/backend/internal/persistence/meta.go`.
- Add fail-loud, idempotent column migration and activation handling in
  `apps/backend/internal/github/store.go`.
- Include all five columns in the table DDL, explicit projections, create,
  replace, restore, update, and legacy rebuild statements.
- Keep all fields nullable with no default and perform no row backfill.
- Add nullable pointers and writer-health documentation to `models.go`.

## Verification

`store_task_pr_outcome_migration_test.go`, `store_taskpr_schema_drift_test.go`,
`store_task_pr_detach_test.go`, and `internal/persistence/meta_test.go` cover
fresh installs, replay, activation write-once behavior, fail-loud migration,
column-list drift, and legacy rebuild preservation.
