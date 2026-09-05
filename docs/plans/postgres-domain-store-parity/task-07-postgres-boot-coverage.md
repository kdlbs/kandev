---
id: "07-postgres-boot-coverage"
title: "Harden PostgreSQL boot coverage"
status: done
wave: 7
depends_on:
  - "06-workflow-execution-stores"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.5
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 07: Harden PostgreSQL Boot Coverage

## Summary

Make the full PostgreSQL boot test fail when an affected service or store is unavailable. Record the final package results in the plan.

## In scope

- Assert availability for GitHub, GitLab persistence, Jira, Linear, Sentry, Azure DevOps, workflow sync, and automation.
- Preserve nonfatal provider isolation in production startup.
- Run the focused PostgreSQL package tests in the CI-shaped environment.

## Out of scope

- Make one provider error stop the complete backend.
- Add browser automation for a backend startup defect.

## Acceptance

- The old code fails the strengthened boot test because affected stores are unavailable.
- The corrected code creates the full affected service graph on PostgreSQL.
- The plan records every focused command and result.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/backendapp -run '^TestPostgresBootInitializesRepositories$' -v
```

## Files likely touched

- `apps/backend/internal/backendapp/postgres_boot_test.go`
- `docs/plans/postgres-domain-store-parity/plan.md`
- `docs/plans/postgres-domain-store-parity/task-07-postgres-boot-coverage.md`

## Dependencies

- Tasks 01 through 06 complete all store conversions.

## Risks

- GitLab can construct its outer service while its task store is absent. The test must exercise or inspect that store boundary.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/postgres_boot_test.go`

## Results

- Strengthened the PostgreSQL boot test to require every affected provider
  service and the workflow-sync and automation components.
- Added store-backed reads for GitHub, GitLab, Jira, Linear, Sentry, Azure
  DevOps, and workflow sync so a service cannot pass only because its outer
  constructor returned a non-nil value.
- Checked the automation component and service boundary directly while
  preserving nonfatal provider initialization behavior in production.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/backendapp -run '^TestPostgresBootInitializesRepositories$' -v`
  passed.
