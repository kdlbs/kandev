---
id: "03-sql-dialect-safety"
title: "Enforce SQL dialect safety"
status: done
wave: 3
depends_on:
  - "02-shared-sql-rendering"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.4
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 03: Enforce SQL dialect safety

## Summary

Add an AST-based SQL safety command and an exact exemption registry. Establish a clean baseline for all non-test backend source.

## In scope

- Detect every risky SQL class named in the system design.
- Resolve package constants, function-local literals, concatenation, and direct executor calls.
- Add exact file, symbol, rule, and reason exemptions for SQLite-only code.
- Reject invalid, broad, stale, and unused exemptions.
- Add a backend Make target for the command.

## Out of scope

- Infer arbitrary SQL returned by remote data or reflection.
- Add the command to GitHub Actions.

## Acceptance

- Each rule has positive and negative analyzer fixtures.
- Current shared-store source passes with no implicit directory exemptions.
- A raw placeholder sent through a database or transaction executor fails the command.

## Verification

```bash
go test -race ./internal/db/sqlguard/... && go run ./cmd/sqlguard ./internal
```

## Files likely touched

- `apps/backend/internal/db/sqlguard/analyzer.go`
- `apps/backend/internal/db/sqlguard/analyzer_test.go`
- `apps/backend/internal/db/sqlguard/testdata/`
- `apps/backend/internal/db/sqlguard/exemptions.json`
- `apps/backend/cmd/sqlguard/main.go`
- `apps/backend/Makefile`

## Dependencies

- Task 02 removes known ad hoc schema replacements before the baseline is recorded.

## Risks

- Incomplete local data-flow analysis can miss a placeholder carried through a helper.

## Parallelism

`sequential`

## Inputs

- System design section: Dialect safety check.
- Existing SQL under `apps/backend/internal`.

## Results

Implemented the AST-based SQL guard, exact exemption registry, command, and
Make target. Both the guard tests and the complete `./internal` source scan
passed with no unused exemptions.
