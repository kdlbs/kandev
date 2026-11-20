---
id: "01-binding-and-retries"
title: "Correct shared environment capture and summary retry termination"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.1
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-004.2
  - AC-TASKS-TURN-CHANGES-004.4
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 01: Binding and retries

## Contract and acceptance

Requirements: [Turn changed files](../../specs/tasks/requirements/turn-changed-files.md).

- AC-TASKS-TURN-CHANGES-003.1: an admitted shared environment can persist its capture policy before dispatch.
- AC-TASKS-TURN-CHANGES-003.6: permanent summary errors terminate independently of completion and capture fences.
- AC-TASKS-TURN-CHANGES-004.2: temporary publication errors remain retryable beyond eight attempts.
- AC-TASKS-TURN-CHANGES-004.4: wrong task, session, turn, and unbound environment identities remain rejected.

## Evidence and implementation

The baseline query incorrectly requires `environment.task_id = s.task_id`.
The supplied SQLite regression fails only for the bound shared environment.
The supplied lifecycle regression exposes repeated retries for permanent errors
and a warning during background shutdown. Current runtime wiring already ignores
missing summaries, so the historical incident's exact version remains unknown.

Change `apps/backend/internal/task/repository/sqlite/turn_changes.go` and add
`turn_change_binding_test.go` beside it. Change
`apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go` and add
`manager_turn_change_summary_retry_test.go` beside it.
Update the linked system design. Do not include the supplied blanket retry cap.

## Verification

From `apps/backend`:

- `go test -race ./internal/task/repository/sqlite ./internal/agent/runtime/lifecycle -run 'Test(Postgres)?TurnChangeSetUsesSessionEnvironmentBinding|TestTurnChangeSummary|TestTurnChangeCapture' -count=1`
- `go test ./internal/backendapp -run '^TestRuntimeTurnChangeSummaryProcessing$' -count=1`

From the repository root:

- `bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/turn_changes.go apps/backend/internal/task/repository/sqlite/turn_change_binding_test.go apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go apps/backend/internal/agent/runtime/lifecycle/manager_turn_change_summary_retry_test.go`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

## Lifecycle evidence

The binding matrix covers shared ownership and rejects mismatches. The summary
matrix covers wrapped errors, successful completion, and publication recovery
after eight failures. A controlled handler waits for cancellation to prove
shutdown joins the retry worker without warnings. Existing capture tests cover
generation fences and duplicate completion. This repair changes no durable retry
state, restart recovery, pause semantics, or capture authority.

## Documentation impact

Internal design and delivery records change. Public instructions, configuration,
and API contracts do not change. This restores existing shared-workspace behavior.
No screenshots are required for these backend changes.

## Results

RED: the ownership, permanent-error, and shutdown regressions fail on the baseline.
GREEN: all named SQLite and lifecycle regressions passed with the race detector.
The existing runtime summary test passed. PostgreSQL was initially skipped because
`KANDEV_TEST_POSTGRES_DSN` was unset. Changed-package lint reported zero issues.
Documentation catalog validation, specification lint, and `git diff --check` passed.
The shared lint lock was allowed to finish before the local run. The installed
Nix linter was added to the command PATH without changing repository tooling.

PR remediation: declare acceptance criteria in frontmatter for the PR coverage validator.
Use portable repository fixture methods for both SQLite and PostgreSQL.

The original fixture failed against disposable PostgreSQL 17.11 with SQLSTATE
42601 at `INSERT OR IGNORE`. After the correction, both SQLite and PostgreSQL
binding matrices passed with `-race` and the exact binding-test command above.
The PostgreSQL server and temporary data were removed. The local
`validateCoverage` evaluation passed with the full linked documentation set.
