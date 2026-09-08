---
id: "03-e2e-status-scoped-delete"
title: "E2E status-scoped delete"
status: done
wave: 3
depends_on:
  - "02-runs-table-header-delete-all"
plan: "plan.md"
spec: "../../specs/office/requirements/automation-runs-delete-all-by-status.md"
---

# Task 03: E2E status-scoped delete

Prove the user-facing flow end to end: delete-all in a filtered view removes
only that view's runs, from the new table-header position.

## Acceptance

- New test in `apps/web/e2e/tests/automations-settings.spec.ts`: seed an
  automation with 2 skipped and 1 succeeded run; expand Recent Runs; click
  the Skipped filter (2 rows); assert the delete-all button lives inside the
  table header (`<thead>`); click it and confirm the dialog shows the scoped
  copy (`permanently remove the Skipped runs shown in this view`); assert the
  table drops to 1 row; switch to the All filter and assert the succeeded run
  remains.
- The existing "delete individual and all runs from Recent Runs" test keeps
  passing unchanged (it already asserts header visibility of
  `delete-all-runs` and the unqualified All-view dialog copy).

## Verification

From `apps/` (fresh worktrees must first run `pnpm install --frozen-lockfile`
from `apps/` when `apps/node_modules/` is absent):

```bash
pnpm --filter @kandev/web e2e:raw -- automations-settings.spec.ts
```

Also run `git diff --check` and Prettier on the changed files before commit.

## Files likely touched

- `apps/web/e2e/tests/automations-settings.spec.ts`

## Dependencies

Tasks 01 and 02 (the E2E exercises both the hook scope and the header
placement).

## Parallelism

Sequential. Runs after the frontend tasks land.

## Inputs

- Spec scenarios: Skipped-filtered delete-all, All-view delete-all.
- Existing patterns in `automations-settings.spec.ts`:
  `apiClient.seedAutomation` / `seedAutomationRun`, the settings-scroll
  reveal, the Recent Runs expansion, and the existing delete-all test
  (dialog copy assertion, `delete-all-runs` / `delete-all-runs-confirm`
  test ids).
- The seeded `succeeded` run needs no task; stored statuses render as-is.

## Output contract

Report the exact Playwright command and pass/fail counts, files changed, any
blockers or risks, and synchronized task/plan status in the same
conversation. No backend changes.

## Results

- E2E: `pnpm --filter @kandev/web e2e:raw -g "delete" --workers=4
  automations-settings.spec.ts` — "delete all only removes the runs in the
  active status view" and "delete individual and all runs from Recent Runs"
  passed; an earlier full chromium-project run (67 tests) passed with exit 0.
- `git diff --check` and Prettier — passed. No backend changes.
