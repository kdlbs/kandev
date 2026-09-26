---
id: "01-persist-target-contracts"
title: "Persist target and repository contracts"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
system_design:
  - ../../specs/office/system-design/automation-target-modes.md
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-TARGETS-001.1
  - AC-OFFICE-AUTOMATION-TARGETS-001.2
  - AC-OFFICE-AUTOMATION-TARGETS-001.5
  - AC-OFFICE-AUTOMATION-TARGETS-001.6
  - AC-OFFICE-AUTOMATION-TARGETS-001.7
  - AC-OFFICE-AUTOMATION-TARGETS-001.8
---

# Task 01: Persist target and repository contracts

## Summary

Add typed target and repository-mode values to automation models, requests,
storage, migrations, validation, and export. Migrate legacy empty-repository
behavior to an explicit no-repository choice and persist exact base branches.

## In scope

- `internal/automation/models.go`, `store.go`, service requests, create/update
  validation, and export.
- API and wire types for `task_mode` and `repository_mode`.
- Compatibility defaults and schema contract tests.

## Out of scope

- Orchestrator task creation and terminal lifecycle behavior.
- Editor rendering and E2E flows.

## Acceptance

- Omitted target fields load as hidden automation runs and existing empty
  repository rows migrate to no repository.
- Explicit `repository_mode=none` is persisted, exported, and does not resolve
  a workspace repository.
- Normal-task mode requires a workflow. Worktree and Local-compatible profiles
  accept repository-free scratch execution.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/automation
```

## Files likely touched

- `apps/backend/internal/automation/models.go`
- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/export.go`
- `apps/backend/internal/automation/*target*test.go`
- `apps/backend/pkg/api/v1/*automation*`

## Dependencies

None.

## Risks

- Import and old-client behavior can accidentally treat an omitted field as an
  explicit no-repository request.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/automation-target-modes.md`
- `docs/specs/office/system-design/automation-target-modes.md`
- `docs/decisions/2026-08-23-automation-target-modes.md`

## Results

- Added persisted `task_mode` and `repository_mode` values with compatibility
  defaults and migration/backfill behavior for legacy empty repository lists.
- Added request validation for hidden versus visible targets, explicit
  repository-free execution, ordered repository/base-branch pairs, and
  workspace repository ownership.
- Added export and store/service regression coverage.
- Verification: `go test -tags fts5 ./internal/automation` passed as part of
  the final automation/orchestrator suite (2,464 tests across both packages).
