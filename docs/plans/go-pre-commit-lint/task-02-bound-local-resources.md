---
id: "02-bound-local-resources"
title: "Bound local lint resources and align harness guidance"
status: done
wave: 2
depends_on:
  - "01-scope-go-lint"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GO-PRE-COMMIT-LINT-001
acceptance_criteria:
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.8
  - AC-PLATFORM-GO-PRE-COMMIT-LINT-001.9
system_design:
  - ../../specs/platform/system-design/go-pre-commit-lint.md
---

# Task 02: Bound local lint resources and align harness guidance

## Summary

Supply conservative resource defaults through the existing selected-package
wrapper and replace routine full-backend lint instructions in the harness.

## Scope

- Default Go memory, analysis concurrency, and build parallelism.
- Preserve explicit resource overrides and unrelated Go flags.
- Align backend guidance, PR fixup, merge recovery, verification, runtime-flag
  checks, and public testing guidance with the wrapper and sequential validation.

## Exclusions

Application behavior, lint rules, base-resolution precedence, CI scan scope,
Makefile full-scan targets, and OS-level process isolation.

## Acceptance

- Process-boundary tests prove defaults, overrides, and preservation of Go flags.
- Routine harness checks use selected packages and serialize heavyweight jobs.
- Documentation distinguishes soft per-process memory targets from hard limits.

## Verification

Run from the repository root:

```bash
python3 scripts/lint-go-changed.test.py
bash scripts/resolve-go-lint-base.test.sh
bash -n scripts/lint-go-changed
pre-commit validate-config
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the targeted `harness-lint` pre-commit hook for all changed harness files.
The lint integration suite uses real Git/pre-commit and a recording linter;
this task does not require an expensive full-backend Go analysis.

## Owned files

- `scripts/lint-go-changed` and `scripts/lint-go-changed.test.py`.
- Root and backend `AGENTS.md`.
- PR-fixup skill and merge-conflicts reference, verify skill and impact matrix,
  runtime-feature-flags skill.
- Linked Go lint requirements, system design, plan, and this work order.
- `docs/public/testing.md`.

## Dependencies and risks

Task 01 is complete. Selected packages still load dependencies. Go memory
targets are soft and per process; higher caller overrides can increase resource
use. CI retains coverage of unchanged reverse dependents.

## Results

- RED: the recording-linter suite failed on missing resource defaults and
  analysis-concurrency flags before the wrapper changed.
- GREEN: all nine hook tests and all 15 comparison-base resolver checks passed.
  Explicit CPU and build-worker overrides differ in the fixture, proving
  independently supplied build parallelism is preserved.
- Bash syntax and pre-commit configuration validation passed.
- All 19 harness-linter tests, all 204 harness files, and the targeted
  `harness-lint` hook passed. Root and backend instruction files remain 300 lines.
- All 36 specification-linter tests, specification lint, and documentation
  catalog validation passed.
- All 63 public-doc validator tests and validation of 48 published pages passed.
- `git diff --check` passed. Full Go lint was not needed: the implementation
  changes only the shell wrapper and harness; the process-boundary fixture
  verifies the linter's flags and environment without analysis of the backend.
