---
id: "10-final-verification"
title: "Final verification"
status: completed
wave: 6
depends_on: ["08-end-to-end-coverage", "09-public-documentation"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 10: Final Verification

## Acceptance

- Repository-wide formatting runs before typecheck, tests, and lint, and every command result is
  recorded without source/test fixes by the verification worker.
- Focused desktop/mobile rendered checks and executor coverage from Task 08 are linked in the report.
- The spec, ADR, plan, task statuses, scoped `AGENTS.md` files, and public docs match the final
  behavior; any failure or deferred executor is reported explicitly.

## Verification

```bash
rtk make fmt
rtk make typecheck
rtk make test
rtk make lint
```

## Files likely touched

- `docs/plans/attach-workspace-sources/plan.md` (status only)
- `docs/plans/attach-workspace-sources/task-10-final-verification.md` (status/report only)
- scoped `AGENTS.md` only if implementation changed a documented architectural pattern

## Dependencies

Tasks 08 and 09; all earlier tasks must already be done.

## Inputs

- Completed implementation tasks and their verification reports.
- Root and scoped `AGENTS.md` verification order.

## Output contract

Exact commands/results, failures without fixes, coverage gaps, blockers, risks, divergence, final
task/plan status, and readiness recommendation.

## Result

Final verification used `TMPDIR=/tmp/kvt` to avoid the runner's Unix-socket path-length limit.

- `make fmt`: passed.
- `make typecheck`: passed.
- `make test`: passed.
- `make lint`: passed.
- Backend compile-only and focused race suites: passed.
- `git diff --check`: passed.
- Desktop Chromium: 2/2 passed.
- Mobile Chrome: 1/1 passed.
- Independent final security review: approved.

Live Docker/SSH attachment execution remains an environment coverage limitation, not a source
failure; see Task 08.
