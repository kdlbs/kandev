---
id: "03-reduce-test-setup"
title: "Reduce frontend test setup"
status: in_progress
wave: 3
depends_on: ["02-repair-frontend-cache"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-003.1
  - AC-PLATFORM-CI-PERFORMANCE-003.2
  - AC-PLATFORM-CI-PERFORMANCE-004.2
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 03: Reduce frontend test setup

## Summary

Separate explicitly reviewed Node-compatible helpers from browser tests.
Load all locales only for suites that need them, while retaining isolated test state.

## In scope

- Test dependency inventory, explicit project selection, minimal Node setup, and multilingual setup.
- A selection contract that detects omissions and overlapping file assignments.
- Comparable baseline/candidate unit-test reports and phase timings.

## Out of scope

- Workflow sharding, disabled isolation, worker-budget increases, application behavior, and deleted tests.

## Acceptance

- Every existing test file belongs to exactly one project, including files under E2E helper directories.
- Browser and multilingual regressions pass under the inherited production environment; unknown dependencies retain full setup.
- At least three comparable runs per configuration show lower median test execution with identical selected identities and no new failures.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
(cd apps/web && pnpm exec vitest run scripts/vitest-project-selection.test.ts scripts/vitest-worker-budget.test.ts vitest-environment.test.tsx lib/i18n)
(cd apps/web && NODE_ENV=production pnpm test --reporter=json --outputFile=/tmp/kandev-ci-unit-candidate.json)
(cd apps/web && pnpm run typecheck)
python3 .github/scripts/frontend-tests-workflow-contract_test.py
git diff --check
```

Before editing setup, run the full JSON command with a baseline output filename.
Repeat baseline and candidate measurements three times on the same source and runner class, changing only setup configuration.
Compare normalized file/test identities, skips, failures, and per-phase times. Keep raw reports outside the repository.

## Files likely touched

- `apps/web/vitest.config.ts`
- `apps/web/vitest.setup.ts`
- `apps/web/vitest.setup.node.ts (new)`
- `apps/web/vitest.setup.locales.ts (new)`
- `apps/web/scripts/vitest-project-selection.test.ts (new)`
- `apps/web/lib/i18n/*.test.*`
- `apps/web/vitest-environment.test.tsx`

## Dependencies

Task 02.

## Risks

File suffixes do not prove DOM independence. Inventory indirect locale helpers and preserve all default cleanup and network guards.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), corresponding implementation boundary.
- [Plan](plan.md), baseline and companion-package status.
- Existing workflow contract tests under `.github/scripts/`.

## Results

Implemented 2026-09-12. The explicitly reviewed Node test list uses a minimal
Node project, two explicitly reviewed browser tests retain happy-dom and React
setup without loading all locale catalogs, and every other discovered test uses
the full browser and locale setup. New tests do not receive reduced setup from
their directory or source markers. The selection contract uses Vitest's
canonical test-file glob and excludes the existing Playwright-only paths.

The selection contract passed and found all 1,999 current test files in exactly
one of `node`, `browser`, or `browser-locales`. The focused production-mode
run passed 12 files and 106 tests. The final unsharded candidate report passed
1,999 files with 17,180 passed tests, 4 pending tests, and 0 failures. The
first candidate attempt exposed one Node-project timeout under contention; the
Node project now has a 15-second test timeout and the repeat passed. Typecheck
passed after adding the selection module's explicit test type.

The required three comparable setup runs on a hosted runner have not been
collected. Local evidence proves selection and correctness, not a hosted
median speed improvement, so this work order remains in progress.
