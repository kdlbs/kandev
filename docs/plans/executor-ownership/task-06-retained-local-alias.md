---
id: "06-retained-local-alias"
title: "Resolve retained local executor aliases at admission"
status: in_progress
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-004
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-004.5
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 06: Resolve retained local executor aliases

Retained sessions can store `local` as an executor selection. Resolve this
legacy runtime-kind alias through the existing `local_pc` admission path so a
missing alias can use the built-in local executor. Actual and retired rows,
profile ownership and storage failures remain authoritative.

## Scope

- `apps/backend/internal/task/service/executor_admission.go`
- `apps/backend/internal/task/service/executor_ownership_legacy_test.go`
- The associated ownership requirement and design.

## Validation

### Contributor evidence

The contributor reported seven legacy-alias ownership scenarios with `local`
through a temporary overlay. The upstream baseline failed the shared and
history-read scenarios. The candidate passed all seven scenarios. Normal commit
hooks passed for the original submission.

### Maintainer review follow-up

The permanent scenario table now includes `local_pc` and `local`. Each alias
uses both `PrepareExecutorAdmission` and `AuthorizeExecutorConfiguration`.
The storage-error wrapper uses the selected alias. The scenarios cover shared
access, private profiles, real private rows, retired rows, unknown identifiers,
read errors, and history-read errors.

The follow-up changes tests and implementation records. Production behavior
remains the contributor's implementation. The reviewer requested no builds,
tests, linters, or other validation commands. Commit hooks are bypassed because
they run validation. Execution of the permanent cases remains pending:

```bash
cd apps/backend
go test -trimpath ./internal/task/service -run '^TestExecutorOwnershipLegacyLocalAliasUsesBuiltinPolicy$' -count=1
```
