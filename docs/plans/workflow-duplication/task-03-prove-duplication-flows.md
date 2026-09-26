---
id: "03-prove-duplication-flows"
title: "Prove duplication flows"
status: done
wave: 3
depends_on: ["02-expose-duplicate-action"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-duplication.md"
---

# Task 03: Prove Duplication Flows

## Acceptance

- Desktop Playwright proves duplication performs no pre-save write, then persists copied configuration with remapped step relationships and no copied task after Save and reload.
- Mobile Playwright performs the same user outcome by touch, measures the Duplicate hitbox, and proves the copied editor and Save action fit without document horizontal overflow.
- Both tests use disposable workflows/tasks and leave worker-scoped canonical seed data unchanged.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/workflow/workflow-duplication.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-duplication.spec.ts)
```

## Files Likely Touched

- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-duplication.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-duplication.spec.ts`

## Dependencies

Task 02.

## Parallelism

Sequential. E2E requires the complete draft, action, translation, and save wiring.

## Inputs

- Every user-visible scenario selected in `plan.md` E2E Tests.
- Existing workflow E2E fixtures, `WorkflowSettingsPage`, `apiClient.listWorkflows`, `listWorkflowSteps`, `createTask`, and managed-runner guidance.
- Mobile parity requirements for `mobile-*.spec.ts`, `.tap()`, 44px hitboxes, and overflow assertions.

## Risks

- Confirm the desktop run builds current production assets before using `--no-build` for mobile.
- Assert the pre-save backend workflow count before pressing Save so a local draft cannot masquerade as correct persistence behavior.
- Identify copied steps by the newly persisted workflow ID, not by source names alone, and assert task step IDs remain outside that copied set.

## Output Contract

Report desktop/mobile scenarios, discovered test counts, exact managed-runner results, files changed, artifacts and cleanup evidence, blockers, risks, and task/plan status updates.

## Results

- Added disposable desktop and mobile Playwright scenarios with a shared API seed helper. The scenarios verify no pre-save workflow write, copied metadata, remapped transition and pull-from references, source-task preservation, post-save reload, touch reachability, and no mobile horizontal overflow.
- Updated `WorkflowSettingsPage.findWorkflowCard` to poll for route-local drafts. This prevents a container timing race after touch duplication from snapshotting the card list before React inserts the draft.
- Checks passed:
  - `cd apps/web && pnpm e2e:run tests/workflow/workflow-duplication.spec.ts` (1 desktop test passed).
  - `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-duplication.spec.ts` (1 mobile test passed).
  - The managed runner cleaned its test-results, blob-report, PR asset, and temporary shard-log locations. No E2E artifacts remain in the worktree.
