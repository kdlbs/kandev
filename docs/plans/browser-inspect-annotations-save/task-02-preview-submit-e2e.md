---
id: "02-preview-submit-e2e"
title: "Cover preview annotation submission"
status: done
wave: 2
depends_on: ["01-inspector-state"]
plan: "plan.md"
spec: "../../specs/ui/requirements/browser-inspect-annotations-save.md"
---

# Task 02: Cover preview annotation submission

## Acceptance

- A real preview iframe test proves clicking Save creates the marker and
  Browser annotations-panel entry.
- A real preview iframe test proves plain Enter follows the same successful
  path for a pin selection.
- The test restores seeded repository settings and runs in the desktop
  Chromium project without enabling the currently unrelated skipped suite.

## Verification

- Bootstrap a fresh worktree if needed:
  `cd apps && pnpm install --frozen-lockfile`.
- Run the focused desktop E2E test with a production build:
  `cd apps/web && pnpm e2e:run --project chromium tests/preview/inspector-submission.spec.ts`.
- Run `git diff --check`.

## Files likely touched

- `apps/web/e2e/tests/preview/inspector-submission.spec.ts`

## Dependencies

Task 01 must pass first.

## Parallelism

Sequential. This test depends on the repaired injected script and the same
preview fixture state.

## Inputs

- `docs/specs/ui/requirements/browser-inspect-annotations-save.md` scenarios.
- `docs/plans/browser-inspect-annotations-save/plan.md` E2E section.
- `apps/web/e2e/README.md` preview-fixture guidance.
- `apps/web/components/task/dockview-header-actions.tsx` Browser panel action.
- `apps/web/e2e/tests/preview/preview-annotations.spec.ts` mock server and
  preview setup patterns.

## Output contract

Report the exact E2E command, test count and result, any browser artifacts,
cleanup performed, and synchronized task/plan statuses.

## Results

- `cd apps && pnpm install --frozen-lockfile` — dependencies already up to date.
- `cd apps/web && pnpm e2e:run --project chromium tests/preview/inspector-submission.spec.ts` — 2 passed in Chromium (Save and plain Enter).
- The test uses a local mock HTTP server, restores the seeded repository's
  `dev_script`, and closes the server in teardown.
