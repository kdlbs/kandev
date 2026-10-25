---
id: "02-shared-sql-rendering"
title: "Centralize shared SQL rendering"
status: done
wave: 2
depends_on:
  - "01-required-store-catalog"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.7
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.5
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 02: Centralize shared SQL rendering

## Summary

Add central schema rendering and rebound execution helpers. Replace store-local DDL text replacement in the PR #3372 store set.

## In scope

- Add validated schema tokens for timestamps, booleans, identities, and current time.
- Add final-boundary helpers for database and transaction execution.
- Support `sqlx.In` expansion before rebinding.
- Convert GitHub, GitLab, Jira, Linear, Sentry, Azure DevOps, workflow-sync, and automation schema rendering.

## Out of scope

- Add the static analyzer.
- Change domain data models.
- Convert dialect-specific SQL that already uses a central helper.

## Acceptance

- Schema rendering rejects unknown and unexpanded tokens.
- Shared execution helpers rebind database and transaction queries exactly once.
- The affected stores contain no local DDL `strings.Replace` chain.

## Verification

```bash
go test -race ./internal/db/... ./internal/github ./internal/gitlab ./internal/jira ./internal/linear ./internal/sentry ./internal/azuredevops ./internal/workflowsync ./internal/automation -run 'Test.*(Schema|Rebind|Boolean|Timestamp|Conflict|Transaction)' -v
```

## Files likely touched

- `apps/backend/internal/db/dialect/schema.go`
- `apps/backend/internal/db/dialect/schema_test.go`
- `apps/backend/internal/db/exec.go`
- `apps/backend/internal/db/exec_test.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/gitlab/store.go`
- `apps/backend/internal/jira/store.go`
- `apps/backend/internal/linear/store.go`
- `apps/backend/internal/sentry/store.go`
- `apps/backend/internal/azuredevops/store.go`
- `apps/backend/internal/workflowsync/store.go`
- `apps/backend/internal/automation/store.go`

## Dependencies

- Task 01 supplies catalog capabilities for the affected stores.

## Risks

- Token replacement order can change a legacy schema fragment if tokens are not explicit.

## Parallelism

`sequential`

## Inputs

- System design section: Schema and query contracts.
- Existing `internal/db/dialect` helpers and PR #3372 store conversions.

## Results

Implemented token-based schema rendering for SQLite and PostgreSQL, final-boundary
query rebinding, and provider-store migration updates. The dialect, execution,
share, and provider regression suites passed, including the PostgreSQL share
schema test.
