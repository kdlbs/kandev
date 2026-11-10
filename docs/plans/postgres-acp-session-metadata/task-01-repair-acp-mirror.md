---
id: 01-repair-acp-mirror
title: Restore the guarded PostgreSQL ACP mirror
wave: 1
depends_on: []
plan: plan.md
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.3
status: done
system: platform
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 01: restore the guarded PostgreSQL ACP mirror

Status: done. Dependencies: none.

## Outcome and ownership

Restore existing session metadata mirroring on PostgreSQL without changing the
SQLite contract. Own `apps/backend/internal/task/repository/sqlite/session.go`,
its new `session_acp_id_test.go`, and this delivery record plus the existing
Platform dialect design. No workspace production edits, schema changes or UI.

References: AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3,
AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.3;
[requirements](../../specs/platform/requirements/postgres-domain-store-parity.md)
and [design](../../specs/platform/system-design/postgres-domain-store-parity.md#schema-and-query-contracts).

## Acceptance

1. Actual PostgreSQL and SQLite execution uses identical assertions for empty,
   null and existing metadata, nested ACP preservation and idempotence.
2. A missing or rotated executor token refuses the mirror without changing the
   stored metadata or timestamp. Preserve the single guarded UPDATE.
3. Existing PostgreSQL workspace ownership coverage and orchestrator wiring
   checks pass; identify #4461 as the already merged first-symptom repair.

## Validation commands

Run from `apps/backend`, with `KANDEV_TEST_POSTGRES_DSN` pointing only at the
owned disposable PostgreSQL fixture (connection details omitted from records):

```sh
go test -trimpath -race ./internal/task/repository/sqlite -run '^Test(SetSessionACPSessionID(SQLite|Postgres)Behavior|ExecutorRunningCASStatusAndDeleteLifecycle)$' -count=1 -v
go test -trimpath -race ./internal/office/repository/sqlite -run '^Test(PostgresWorkspaceGroupOwnershipRoundTrip|WorkspaceGroup_CreateDefaultsAreSafe|WorkspaceGroupMembers_AddListRelease)$' -count=1 -v
go test -trimpath -race ./internal/orchestrator -run '^TestPersistACPSessionID$' -count=1
go test -trimpath -race ./internal/task/service -run '^TestAttachWorkspacePolicy_' -count=1
go run ./cmd/sqlguard ./internal
go test -trimpath -race ./internal/persistence/storeconformance -count=1
```

From repo root:

```sh
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/session.go apps/backend/internal/task/repository/sqlite/session_acp_id_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Evidence

RED: before production edits, the shared behavior test passed 10 of 11 SQLite
cases and failed all 11 PostgreSQL cases with SQLSTATE 42601 near `$4`.
Log: `/tmp/4525-red.log`. The original report's SQL failure is reproduced
independently of workspace creation and automation.

Initial applicability: existing workspace tests passed on unchanged main,
including real PostgreSQL ownership false/true, materialization and membership;
log `/tmp/4525-workspace.log`.

The SQLite empty-string case separately failed with malformed JSON because the
comparison did not normalize the metadata while the write did. Reuse the existing
normalization expression in that predicate. Final dual-engine GREEN is recorded
below.


## Validation results

- Final ACP/executor lifecycle race command: PASS, 2.393s, including all 11
  PostgreSQL and 11 SQLite mirror cases. Zero skipped cases.
- Workspace race command: PASS, 2.187s, including real PostgreSQL ownership
  false/true, materialization and membership behavior. Zero skipped cases.
- Orchestrator `TestPersistACPSessionID` race command: PASS, 1.503s (SQLite
  service wiring, current token, rotated token, nested metadata and no-op cases).
- Inherited workspace policy service race command: PASS, 1.097s.
- SQL guard: PASS (exit 0). Changed-file lint: PASS, zero issues.
- Document catalog validation: PASS, 394 decisions and 1679 specifications.
  Specification lint and `git diff --check`: PASS.
- Required persistence conformance race command: PASS, 158.981s. Both SQLite
  and PostgreSQL conformance and previous-stable upgrade cases executed; zero
  skips and zero failures.

PostgreSQL execution used owned disposable Docker PostgreSQL 16.15 with no
host mounts and isolated schemas through `testutil.OpenIsolatedPostgres`.
No PostgreSQL 18, ECS/Fargate/RDS, live deployment, complete MCP/automation
end-to-end, or full backend suite validation is claimed. No schema or upgrade
fixture changes are needed. Temporary logs are local evidence, not deliverables.


## Delivery preparation

Controller validation approved the mixed diagnosis and authorized protected
PR delivery. Refreshed the branch by fast-forwarding to current main
`5f4af538b132297a96c383ec731acd800b06f76d`; its review UI changes are disjoint.
Production SQL and regressions are unchanged from the validated candidate.
Added delivery-record frontmatter required by PR documentation coverage;
local `validateCoverage` returns covered with no errors.

Re-ran the focused integration checks on that base with the owned PostgreSQL
16.15 fixture using:

```sh
go test -trimpath -race ./internal/task/repository/sqlite ./internal/office/repository/sqlite ./internal/orchestrator ./internal/task/service -run '^Test(SetSessionACPSessionID(SQLite|Postgres)Behavior|ExecutorRunningCASStatusAndDeleteLifecycle|PostgresWorkspaceGroupOwnershipRoundTrip|WorkspaceGroup_CreateDefaultsAreSafe|WorkspaceGroupMembers_AddListRelease|PersistACPSessionID|AttachWorkspacePolicy_.*)$' -count=1 -v
```

PASS: task repository 2.680s, Office repository 2.214s, orchestrator 1.689s,
task service 1.115s. All 11 PostgreSQL and 11 SQLite mirror cases executed;
zero skips. Document validators passed on the refreshed base (394 decisions,
1681 specifications). Screenshots are not required for this backend-only fix.
Exact PR head, CI/review and merge evidence remain pending externally.
