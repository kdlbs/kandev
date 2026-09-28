---
id: "01-preview-repair"
title: "Preview and validate exact inventory repairs"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.1
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 01: Preview and validate exact inventory repairs

## Summary

Implement the read-only half of the local maintenance utility. Produce a bounded
repair description that binds every proposed change to independently verified
database, marker, and Git identities.

## In scope

- Proposed `cmd/worktree-inventory-repair` and `internal/task/inventoryrepair`.
- JSON operation identity, exact expected rows, allowed replacement fields, and
  repository/workspace scope. Reject unknown fields and broad selection predicates.
- Registration, common-directory, backlink, symbolic HEAD, marker, and destination
  inspection. Validate multi-repository workspace paths and all affected consumers.
- Default preview, `--plan <file>`, and a verification-only mode. Do not initialize
  migrating application repositories against a inspected production database.

## Out of scope

Application, backend restart, automatic repairs, or new schema migrations.

## Acceptance

1. Real SQLite/Git fixtures reproduce both stale branches, missing repository
   identity, and cross-root shared inventory. Preview leaves database bytes,
   worktrees, refs, and markers unchanged.
2. Foreign markers, ambiguous authorized repositories, stale expected values,
   detached or malformed Git metadata, and unsupported stores/executors refuse
   application eligibility with a precise field-level reason.
3. The command reports exact proposed changes and unresolved consumers; it does
   not mistake a stopped runtime row or clean ordinary Git status for complete
   evidence that a workspace is unused or has no ignored files.

## Verification

Run from the repository root after implementing the proposed packages:

```bash
(cd apps/backend && go test -race ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair -count=1)
git diff --check
```

## Files likely touched

- Proposed `apps/backend/internal/task/inventoryrepair/{plan,inspect}.go`
- Proposed `apps/backend/internal/task/inventoryrepair/inspect_test.go`
- Proposed `apps/backend/cmd/worktree-inventory-repair/main.go` and tests
- Existing `internal/worktree/manager_archive_source_manifest.go` and
  `internal/common/subproc` as inspection patterns; keep public guards intact

## Dependencies

None. Read the paired requirement/design and `apps/backend/AGENTS.md`, then use
the repository's TDD skill and backend test reference.

## Risks

Repository names and primary-session selections are not repository identity.
Avoid turning file contents or paths into executable shell fragments.

## Parallelism

`sequential`

## Results

Implemented the bounded CLI and read-only inspector. Real SQLite/Git tests pass under the race detector for branch mismatch, missing repository ID, shared-root bindings, stale rows/content, competing path owners, unsupported executors, stopped rows with live PIDs, and unchanged preview state.
