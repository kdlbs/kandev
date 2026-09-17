---
id: "09-ci-contributor-gates"
title: "Install CI and contributor gates"
status: done
wave: 9
depends_on:
  - "03-sql-dialect-safety"
  - "07-upgrade-fixtures"
  - "08-required-store-health"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008.3
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 09: Install CI and contributor gates

## Summary

Replace marker-based PostgreSQL discovery with fixed gates. Add the PostgreSQL 18 smoke matrix and document the required-store workflow.

## In scope

- Invoke catalog completeness, conformance, upgrade, boot, and SQL guard commands explicitly.
- Remove the `PostgresDSNFromEnv` package discovery shell.
- Mirror and pin PostgreSQL 18 for clean-boot and upgrade tests.
- Add workflow contract tests that reject grep-based discovery and missing fixed commands.
- Update public backend-development and operations documentation.
- Update root and backend agent guidance.

## Out of scope

- Add more PostgreSQL majors than 16 and 18.
- Change release support outside the documented database matrix.
- Add browser E2E coverage.

## Acceptance

- A catalog entry without both engine adapters fails a fixed CI command.
- CI runs full PostgreSQL 16 conformance and PostgreSQL 18 boot plus upgrade coverage.
- Contributor docs state the catalog, test, fixture, SQL safety, and provider-isolation rules.

## Verification

```bash
python3 .github/scripts/backend-tests-workflow-contract_test.py
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/workflows/backend-tests.yml`
- `.github/workflows/ci-base-image.yml`
- `.github/scripts/backend-tests-workflow-contract_test.py`
- `docs/public/backend-development.md`
- `docs/public/operations.md`
- `AGENTS.md`
- `apps/backend/AGENTS.md`

## Dependencies

- Task 03 supplies the SQL guard command.
- Task 07 supplies upgrade tests and PostgreSQL 18 entry points.
- Task 08 supplies final readiness behavior for documentation.

## Risks

- A second service image can increase CI startup time and registry load.

## Parallelism

`sequential`

## Inputs

- System design sections: CI execution, PostgreSQL version coverage, Contributor workflow.
- Existing backend workflow and CI image mirror process.

## Results

Implemented fixed PostgreSQL test commands, SQL guard and conformance CI gates,
the pinned PostgreSQL 18 image mirror, workflow contract tests, and contributor
and operator documentation. Workflow, public-doc, specification, and diff
validation passed.
