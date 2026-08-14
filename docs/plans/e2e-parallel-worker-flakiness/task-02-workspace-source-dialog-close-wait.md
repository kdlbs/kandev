---
id: "02-workspace-source-dialog-close-wait"
title: "Response-gated workspace-source close wait"
status: completed
wave: 1
depends_on: ["01-rename-focus-wait"]
plan: "plan.md"
spec: "../../specs/e2e-parallel-worker-flakiness/spec.md"
---

# Task 02: Response-gated workspace-source close wait

## Acceptance

- In `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`, both
  submit-and-close sequences — the repository source at line 163 and the
  folder source at lines 187-188 — wait for the
  `POST /api/v1/tasks/{taskId}/workspace-sources` response and await its body completion before
  asserting `expect(dialog).not.toBeVisible()`, following the existing
  `waitForRequest` pattern already used for `open-folder` at lines 93-101 in
  the same file.
- The dialog-close assertion still fails, and fails for the right reason, if
  the dialog never closes (do not simply delete the assertion or replace it
  with only a network wait).
- No change to production code (`kanban-api.ts`, the dialog component, or the
  backend).

## Verification

```sh
cd apps/web
pnpm run build:e2e
pnpm run e2e:raw tests/task/add-workspace-sources.spec.ts --project=chromium --workers=2
pnpm run e2e:raw tests/task tests/kanban tests/gitlab --project=chromium --workers=4
```

Run the second command at least twice; the original finding reproduced the
failure inconsistently across runs.

## Files likely touched

- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts` (lines 147-163 for
  the repository submit, lines 176-188 for the folder submit)

## Dependencies

None.

## Parallelism

Sequential after Task 01 for coherent verification records.

## Inputs

- Root cause in `plan.md` and the spec's "Broken behavior" section, including
  the captured Playwright call log showing the dialog's `open` -> `closed`
  transition landing after the 5s window.
- `apps/web/lib/api/domains/kanban-api.ts:166-176`
  (`attachTaskWorkspaceSources`, `POST /api/v1/tasks/{taskId}/workspace-sources`).
- The existing `waitForRequest` pattern at
  `add-workspace-sources.spec.ts:93-101` (`open-folder`) as the template to
  follow for both submit sites.

## Output contract

Report the exact wait added at each of the two submit sites, and the
targeted + 4-worker verification results.

## Results

- Added `submitWorkspaceSources(page, submit)` in `add-workspace-sources.spec.ts`,
  used at both the repository-source and folder-source submit sites (previously
  lines 163 and 187-188). It arms `waitForHttp(page, "POST", WORKSPACE_SOURCES_PATH)`
  before `submit.click()`, asserts `response.ok()`, and awaits `response.finished()`
  before the existing `expect(dialog).not.toBeVisible()` assertion.
- Mid-implementation, `origin/main` was merged into this branch (by a concurrent
  session sharing the worktree) and had already patched this exact race in this
  exact file with a `{ timeout: 30_000 }` bump on both dialog-close assertions,
  plus landed a new canonical `waitForHttp` helper in
  `apps/web/e2e/helpers/causal-waits.ts`. Resolved the resulting merge conflict by
  keeping this task's response-gated approach (which makes the timeout bump
  unnecessary) and rewriting it on top of `waitForHttp` instead of a hand-rolled
  `page.waitForResponse`, per current repo convention. No production code changed.
- 2/2 tests pass standalone (`--retries=0`); 2/2 pass inside both loaded 4-worker
  runs (see plan.md Results).
