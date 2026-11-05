---
id: "06-retained-local-alias"
title: "Resolve retained local executor aliases at admission"
status: done
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
- The associated ownership requirement and design.

## Validation

The existing legacy-alias ownership scenarios were run with `local` through a
temporary overlay. The upstream baseline failed the shared and history-read
scenarios; the candidate passed all seven scenarios. Normal commit hooks passed.
No permanent test case or additional authorization path was introduced.
