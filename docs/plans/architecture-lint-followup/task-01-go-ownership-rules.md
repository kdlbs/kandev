---
id: "01-go-ownership-rules"
title: "Go scheduler and runs ownership rules"
status: done
wave: 1
depends_on: []
plan: "plan.md"
decision: "../../decisions/2026-08-01-architecture-lint-budgets.md"
---

# Task 01: Go scheduler and runs ownership rules

## Acceptance

- Production Go imports of `internal/runs/scheduler` fail outside
  `internal/backendapp/**`; the current composition-root import passes and the
  checked-in scheduler baseline is empty.
- Production Go imports of `internal/office` from `internal/runs/**` fail,
  while the exact three current findings pass only through the exact baseline.
- Tests prove aliased/block imports, test and generated-file exclusions,
  baseline regression, deterministic diagnostics, and actionable ownership
  messages.

## Verification

```bash
python3 scripts/lint-architecture.test.py
python3 scripts/lint-architecture.py --all
```

## Files likely touched

- `scripts/architecture_lint/rules/go_imports.py`
- `scripts/architecture_lint/rules/run_scheduler_owner.py`
- `scripts/architecture_lint/rules/runs_office_import.py`
- `scripts/architecture_lint/rules/__init__.py`
- `scripts/architecture_lint_tests/support.py`
- `scripts/architecture_lint_tests/test_run_scheduler_owner.py`
- `scripts/architecture_lint_tests/test_runs_office_import.py`
- `config/architecture-lint/run_scheduler_owner.json`
- `config/architecture-lint/runs_office_import.json`

## Inputs and dependencies

- `docs/decisions/2026-08-01-global-run-scheduler-ownership.md`
- Existing `runtime_import.py`, `task_office_import.py`, and `go_imports.py`
  conventions.
- No dependency on other implementation tasks; later documentation and final
  integration depend on this task.

## Parallelism

Sequential. The rule registry and shared fixture are shared with Task 02.

## Results

- `python3 scripts/lint-architecture.test.py` — passed, 34 tests.
- `python3 scripts/lint-architecture.py --all` — passed with no diagnostics.
- `git diff --check` — passed.
- No generated artifacts or external side effects.
