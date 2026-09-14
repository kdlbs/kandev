---
id: "02-repair-frontend-cache"
title: "Repair frontend dependency caching"
status: in_progress
wave: 2
depends_on: ["01-bound-claude-execution"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-002
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-002.1
  - AC-PLATFORM-CI-PERFORMANCE-002.2
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 02: Repair frontend dependency caching

## Summary

Resolve the pnpm store from the actual frontend container environment.
Prove a compatible cache save and later restore without weakening dependency installation.

## In scope

- Store discovery before the pinned cache action, with compatible versioned keys.
- Cold and unavailable cache behavior, plus exact lockfile enforcement.
- Curated save/restore and install timing evidence in this work order.

## Out of scope

- E2E cache changes, `node_modules` transfer, toolchain upgrades, and runner activation.

## Acceptance

- Cache restore and save use the path returned by pnpm under the installation environment.
- A cold cache installs successfully; a later compatible run reports a real restore without path warnings.
- Cache unavailability does not skip installation or hide installation errors.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/frontend-tests.yml
git diff --check
```

For cache evidence, set `CI_RUN_ID` and `CI_JOB_ID` from an authorized run of the changed workflow.
Use these read-only commands for the cold and warm jobs:

```bash
: "${CI_RUN_ID:?Set a run ID}"
: "${CI_JOB_ID:?Set its frontend job ID}"
gh api "repos/kdlbs/kandev/actions/runs/$CI_RUN_ID/jobs?per_page=100"
gh api "repos/kdlbs/kandev/actions/jobs/$CI_JOB_ID/logs" > /tmp/kandev-ci-cache-job.log
rg 'Cache|cache|store|reused|downloaded|Path Validation' /tmp/kandev-ci-cache-job.log
```

Record a cache-service failure or controlled unavailable-cache check with a successful frozen installation.
Do not weaken dependency failure handling to obtain that result.

## Files likely touched

- `.github/workflows/frontend-tests.yml`
- `.github/scripts/frontend-tests-workflow-contract_test.py`

## Dependencies

Task 01.

## Risks

Cache permissions and branch scope affect hits. Record runner identity, pnpm version, resolved path, key, and save/restore outcomes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), corresponding implementation boundary.
- [Plan](plan.md), baseline and companion-package status.
- Existing workflow contract tests under `.github/scripts/`.

## Results

Implemented 2026-09-12. The frontend container now resolves its pnpm store
with `pnpm store path --silent` before the cache action, records the pnpm
version, uses OS, architecture, version, and lockfile compatible keys, and
keeps `pnpm install --frozen-lockfile` after the best-effort cache step. The
same wiring is present in the frontend unit-test matrix jobs.

Local evidence:

- `cd apps && pnpm store path --silent` returned
  `/root/.local/share/pnpm/store/v3`.
- `cd apps && pnpm --version` returned `9.15.9`.
- `python3 .github/scripts/frontend-tests-workflow-contract_test.py` passed (10 tests).
- `python3 .github/scripts/lint-action-pinning_test.py` passed (9 tests).
- `git diff --check` passed.

Hosted cold-install, cache-save, and compatible cache-restore evidence remains
open because the changed workflow was not dispatched or pushed in this task.
The previous hosted baseline recorded a path-validation warning and no cache
save; it is recorded in [CI performance evidence](evidence.md).
