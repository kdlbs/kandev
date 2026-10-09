---
id: "01-scope-go-lint"
title: "Scope Go commit lint to selected packages"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GO-PRE-COMMIT-LINT-001
acceptance_criteria:
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.1
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.2
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.3
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.4
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.5
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.6
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.7
system_design:
  - ../../specs/platform/system-design/go-pre-commit-lint.md
---

# Task 01: Scope Go commit lint to selected packages

## Summary

Pass selected filenames to a package-scoping wrapper and preserve lint failures,
comparison-base policy, and staged-file handling. Cover inherited Git repository
overrides and record the completed implementation's delivery contracts.

## In scope

- Hook filename forwarding and serial execution.
- Package selection, deduplication, empty selection, and Git environment isolation.
- Focused process-boundary tests and linked requirement/design documentation.

## Out of scope

- Other hooks, lint rules, backend CI selection, and application changes.
- Deleted-only changes and changes to module/configuration triggers.

## Acceptance

- Selected source/test files produce exact containing-package targets once per batch.
- Empty selections succeed without lint; invalid bases and lint failures block commits.
- Inherited Git overrides do not redirect the wrapper, and unstaged edits survive.

## Verification

Run from the repository root:

```bash
python3 scripts/lint-go-changed.test.py
bash scripts/resolve-go-lint-base.test.sh
pre-commit run go-lint --files apps/backend/internal/common/gitref/gitref.go
pre-commit validate-config
bash -n scripts/lint-go-changed
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.pre-commit-config.yaml`
- `scripts/lint-go-changed`
- `scripts/lint-go-changed.test.py`
- `docs/specs/platform/requirements/go-pre-commit-lint.md`
- `docs/specs/platform/system-design/go-pre-commit-lint.md`
- `docs/specs/platform/README.md`
- `docs/plans/go-pre-commit-lint/plan.md`
- This work order.

## Dependencies

None.

## Risks

Dependencies still load for type checking; full-backend CI covers cross-package
effects outside the selected lint targets.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/go-pre-commit-lint.md)
- [System design](../../specs/platform/system-design/go-pre-commit-lint.md)
- Existing hook configuration, resolver, and resolver tests.

## Results

Package-selection and inherited-environment regressions were observed before
their fixes. Final local results:

- `python3 scripts/lint-go-changed.test.py`: 7 passed.
- `bash scripts/resolve-go-lint-base.test.sh`: 15 checks passed.
- The explicit `pre-commit run go-lint` command above passed with the real linter.
- `pre-commit validate-config` and `bash -n scripts/lint-go-changed`: passed.
- `python3 scripts/list-docs.py validate`: 294 decisions and 1077 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 passed.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check`: passed.
- The repository PR documentation evaluator accepted this work order and its
  linked plan, requirement, and design with status `covered`.

PR #3860 owns subsequent remote CI, review, and merge evidence.
