---
id: "02-frontend-state-ui-rule"
title: "Frontend state to UI dependency rule"
status: done
wave: 2
depends_on:
  - "01-go-ownership-rules"
plan: "plan.md"
decision: "../../decisions/2026-08-01-architecture-lint-budgets.md"
---

# Task 02: Frontend state to UI dependency rule

## Acceptance

- Production files under `apps/web/lib/state/**` reject resolved imports from
  `apps/web/components/**` and `apps/web/app/**` across static, type-only,
  export-from, and string-literal dynamic import forms.
- Alias and relative imports resolve correctly, while comments, arbitrary
  strings, local state modules, tests, fixtures, and generated files do not
  produce findings.
- The exact two current sidebar-constant findings pass through the exact
  baseline, and any third finding fails with a message explaining that UI/app
  layers consume state.

## Verification

```bash
python3 scripts/lint-architecture.test.py
python3 scripts/lint-architecture.py --all
```

## Files likely touched

- `scripts/architecture_lint/rules/frontend_imports.py`
- `scripts/architecture_lint/rules/frontend_state_ui_import.py`
- `scripts/architecture_lint/rules/__init__.py`
- `scripts/architecture_lint_tests/support.py`
- `scripts/architecture_lint_tests/test_frontend_state_ui_import.py`
- `config/architecture-lint/frontend_state_ui_import.json`

## Inputs and dependencies

- `apps/web/AGENTS.md` state structure and Vite alias contract.
- Existing `frontend_root_state_cast.py` scanner style and shared baseline
  tests.
- Task 01's registry/fixture changes.

## Parallelism

Sequential because the rule registry and test fixture are shared with Task 01.

## Results

- `python3 scripts/lint-architecture.test.py` — passed, 41 tests.
- `python3 scripts/lint-architecture.py --all` — passed with no diagnostics.
- `git diff --check` — passed.
- No generated artifacts or external side effects.
