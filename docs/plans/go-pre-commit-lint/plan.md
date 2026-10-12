---
created: 2026-09-22
status: done
requirements:
  - REQ-PLATFORM-GO-PRE-COMMIT-LINT-001
system_design:
  - ../../specs/platform/system-design/go-pre-commit-lint.md
legacy_specs: []
---

# Implementation Plan: Go pre-commit lint

## Overview

Limit commit-time Go lint to the packages represented by pre-commit's selected
files. One work order owns hook wiring, the wrapper, regression coverage, and
the linked documentation. This record includes the implementation and PR
remediation; external CI and merge evidence remain in the PR task record.

## Scope

- Select and deduplicate complete containing packages, with serial execution.
- Preserve comparison-base policy, error propagation, and pre-commit staging.
- Isolate Git repository context from inherited location and storage overrides.
- Exclude changes to application behavior, other hooks, and full-backend CI.

## Technical approach

`.pre-commit-config.yaml` forwards filenames to `scripts/lint-go-changed`.
The wrapper selects exact package paths and reuses `scripts/resolve-go-lint-base`.
The regression suite drives real Git/pre-commit and substitutes only the linter.

## Tests

All named tests are in `scripts/lint-go-changed.test.py`.

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-PLATFORM-GO-PRE-COMMIT-LINT-001.1`, `.2` | `test_lints_each_staged_package_once`, `test_lints_multiple_packages_in_one_process_without_recursing` |
| `AC-PLATFORM-GO-PRE-COMMIT-LINT-001.3` | `test_non_go_commit_skips_lint`, `test_empty_package_selection_never_falls_back_to_full_lint` |
| `AC-PLATFORM-GO-PRE-COMMIT-LINT-001.4` | `test_lint_failure_blocks_commit`, `test_invalid_comparison_base_fails_before_lint`, and the 15 existing resolver checks |
| `AC-PLATFORM-GO-PRE-COMMIT-LINT-001.5` | `test_ignores_inherited_repository_environment` |
| `AC-PLATFORM-GO-PRE-COMMIT-LINT-001.6` | `test_lints_each_staged_package_once` preserves unstaged edits |
| `AC-PLATFORM-GO-PRE-COMMIT-LINT-001.7` | Inspection confirms the CI and Makefile lint commands retain `./...`; neither file is modified |

## Work orders

- [x] [Task 01: Scope Go commit lint to selected packages](task-01-scope-go-lint.md)

## Verification results

The original package-selection regression failed on `./...` before the wrapper
was introduced. The inherited-environment regression failed during base resolution
before the additional Git variables were cleared.

- All seven hook tests and all 15 resolver checks passed.
- The real `go-lint` hook passed for `internal/common/gitref/gitref.go`.
- Pre-commit configuration and Bash syntax validation passed.
- The documentation catalog validated 294 decisions and 1077 specifications.
- All 36 specification-linter tests and the complete specification lint passed.
- The PR coverage evaluator returned `covered` for the changed files and this
  linked delivery package.
- `git diff --check` passed. Public docs need no change because product behavior
  and public APIs are unaffected.

Remote CI, automated review disposition, and merge status are tracked externally
on PR #3860; these local results do not claim those gates are complete.

## Risks

- Go still loads dependencies for type information; package selection does not
  guarantee a fixed latency improvement.
- Local lint does not analyze unchanged reverse dependents. Full-backend CI
  retains that responsibility.
