---
id: "01-ownership-policy"
title: "Persist ownership and define policy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-001
  - REQ-EXECUTORS-OWNERSHIP-002
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-001.1
  - AC-EXECUTORS-OWNERSHIP-001.2
  - AC-EXECUTORS-OWNERSHIP-001.3
  - AC-EXECUTORS-OWNERSHIP-001.4
  - AC-EXECUTORS-OWNERSHIP-002.1
  - AC-EXECUTORS-OWNERSHIP-002.3
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 01: Persist ownership and define policy

## Summary

Add compatible ownership storage and a shared eligibility policy, with atomic owner transitions and persistence across all write paths.

## In scope

- Add optional ownership fields, versioned migration, DTO foundation, and all SQLite/PostgreSQL read/write/conformance paths.
- Implement one policy using current identity/scopes, parent/profile conjunction, synthetic-only bypass, and ownership mutation rules.
- Test migration/restart/auth toggling, disabled/deleted owners, incompatible parent assignment, explicit clear versus omitted PATCH, and stale save races.

## Out of scope

Other work orders and the package exclusions. Do not add UI before the underlying authorization contract exists.

## Acceptance

- Existing rows and omitted owner fields remain shared; every update path preserves an omitted owner.
- Owner/manager policy denies foreign or actorless private use and never widens workspace/tenant reach.
- Ownership changes validate user and parent state atomically and cannot be undone by a stale partial/script save.

## Verification

Run from repository root. On a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` before the first pnpm command.
Add the planned tests with TDD; test names must contain `ExecutorOwnership`
where the Go commands filter by that pattern. Ensure new focused web tests are
added to the exact Vitest command before recording completion.

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service -run 'ExecutorOwnership|ExecutorProfile.*Script' -count=1)
(cd apps/backend && go test ./internal/persistence/storeconformance -run 'ExecutorOwnership' -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set the isolated PostgreSQL test database DSN}"
(cd apps/backend && go test ./internal/persistence/storeconformance -run 'ExecutorOwnership' -count=1)
```

Also run `bash scripts/lint-go-changed` with the explicit changed Go files,
then `git diff --check`. The PostgreSQL pass requires the existing isolated test database connection
via `KANDEV_TEST_POSTGRES_DSN`; provision it using repository test conventions.
A skipped PostgreSQL test is not evidence of parity. Record both engine results.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/dto/`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/task/service/executor_ownership.go (new)`
- `apps/backend/internal/persistence/storeconformance/executor_ownership_test.go (new)`

## Dependencies

None.

## Risks

Ownership-bearing resource persistence and same-owner child checks are coupled; keep migration and policy changes in this order.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/per-user-ownership.md).
- [System design](../../specs/executors/system-design/per-user-ownership.md).
- [Ownership ADR](../../decisions/2026-10-15-private-executor-admission.md).
- [Plan evidence and existing code patterns](plan.md#technical-approach).
- Read scoped AGENTS.md, `/tdd`, and applicable `/mobile-parity` and `/e2e`
  instructions before implementation.

## Results

Implemented nullable executor/profile owners, presence-aware patch fields, assignment policy, compatible writers, parent/profile constraints, and serialized ownership admission. Ordinary and script-intent writes retain ownership; stale ownership saves fail. Legacy fixtures verify SQL NULL/shared defaults. Trusted task/session actors survive task and Office table rebuilds.

Passed focused ownership/script regression tests and full race-enabled SQLite/PostgreSQL store conformance, including previous-stable upgrades and same-database replay. PostgreSQL used an isolated PostgreSQL 17 container via `KANDEV_TEST_POSTGRES_DSN` (no PostgreSQL skips). SQLguard and changed task/persistence/MCP Go lint passed. See the plan for the consolidated commands and results.

PR remediation: conditional executor/profile deletes now compare the authorized ownership snapshot under the parent transfer lock. Service race/positive-control regressions and race-enabled SQLite/PostgreSQL ownership conformance pass.
