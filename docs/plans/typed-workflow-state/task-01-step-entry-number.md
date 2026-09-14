---
id: "01-step-entry-number"
title: "Server-computed step-entry number in workflow prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TWS-001
  - REQ-TWS-002
  - REQ-TWS-005
acceptance_criteria:
  - AC-TWS-001.1
  - AC-TWS-001.2
  - AC-TWS-001.3
  - AC-TWS-001.4
  - AC-TWS-001.5
  - AC-TWS-001.6
  - AC-TWS-001.7
  - AC-TWS-001.8
  - AC-TWS-001.9
  - AC-TWS-001.10
  - AC-TWS-002.1
  - AC-TWS-002.2
  - AC-TWS-002.3
  - AC-TWS-002.4
  - AC-TWS-002.5
  - AC-TWS-002.6
  - AC-TWS-005.1
  - AC-TWS-005.2
system_design:
  - ../../specs/typed-workflow-state/system-design/typed-workflow-state.md
---

# Task 01: Server-computed step-entry number in workflow prompts

## Scope

Substitute `{step_entry_number}` in workflow prompt templates with the
1-based ordinal of the current step entry, computed server-side from
`task_step_transitions`, so the agent never counts its own prose to enforce a
review-round cap.

## Files touched

- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_step_entry_number_test.go`
- `apps/backend/internal/task/repository/sqlite/step_transitions.go`
- `apps/backend/internal/task/repository/sqlite/step_transitions_count_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/step_entry_number_prompt_test.go`

## Verification

- Entry number is 1-based and inclusive of the current entry
  (AC-TWS-001.2), derived from `COUNT(*) FROM task_step_transitions WHERE
  task_id = ? AND to_workflow_step_id = ?` (AC-TWS-001.3), floored at 1 for a
  task predating the ledger (AC-TWS-001.4).
- Substitution runs at both production call sites (AC-TWS-001.5), is a no-op
  when the template does not contain the token (AC-TWS-001.6), renders in
  base 10 with no separator or sign (AC-TWS-001.7), and leaves an empty
  task/step identifier unsubstituted (AC-TWS-001.8). `{task_id}` and
  `{step_entry_number}` may both appear in one template (AC-TWS-001.9), and
  every occurrence of the exact token is replaced (AC-TWS-001.10).
- A count-query error leaves the token literal and does not fail prompt
  building (AC-TWS-002.1, AC-TWS-002.2); unrecognised tokens survive
  interpolation unchanged (AC-TWS-002.3); `{task_id}` substitution and
  `{{task_prompt}}` survival are byte-for-byte unchanged (AC-TWS-002.4,
  AC-TWS-002.5); the derivation is documented in the doc comment
  (AC-TWS-002.6).
- Re-prompting the same step entry yields the same number (AC-TWS-005.1); the
  count query is a read outside any write transaction, taken as an
  unreconciled snapshot at read time (AC-TWS-005.2).
- `go test ./apps/backend/internal/sysprompt/... ./apps/backend/internal/task/repository/sqlite/... ./apps/backend/internal/orchestrator/...`
  — all green. `gofmt`, `go vet`, and `golangci-lint` clean.
