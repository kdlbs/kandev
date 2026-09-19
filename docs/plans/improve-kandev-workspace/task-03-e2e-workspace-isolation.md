---
id: "03-e2e-workspace-isolation"
title: "E2E: workspace isolation coverage"
status: completed
wave: 3
depends_on: ["02-frontend-bootstrap-workspace"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 03: E2E — improve tasks land in the dedicated workspace

Prove via Playwright that a task submitted through the Improve Kandev dialog
lands in the dedicated workspace, not the active one.

## Acceptance

1. Both bootstrap mocks (`improve-kandev.spec.ts`, `mobile-improve-kandev.spec.ts`)
   include `workspace_id` in the mocked response (default: `seedData.workspaceId`
   for existing tests; overridable).
2. New test "improve task lands in the dedicated workspace":
   - Seed a real dedicated workspace: `apiClient.createWorkspace("Improve
     Kandev")`, a workflow in it, and a repository
     (`apiClient.createRepository(dedicated.id, <seed repo dir>)`).
   - Mock bootstrap to return `workspace_id: dedicated.id`,
     `workflow_id: <dedicated workflow id>`,
     `repository_id: <dedicated repo id>`.
   - Submit a task through the dialog; assert
     `apiClient.listTasks(dedicated.id)` contains it and
     `apiClient.listTasks(seedData.workspaceId)` does not.

## Verification

```sh
cd apps && pnpm install --frozen-lockfile   # fresh worktree bootstrap, if needed
cd apps/web && pnpm e2e improve-kandev
```

(The full E2E suite runs in the final verification phase, not here.)

## Files likely touched

- `apps/web/e2e/tests/improve-kandev.spec.ts`
- `apps/web/e2e/tests/mobile-improve-kandev.spec.ts`

## Dependencies

Task 01 (response contract) and task 02 (frontend uses the response
workspace id). The E2E fixtures (`test-base.ts` `seedData`) already expose
`workspaceId`, `workflowId`, `repositoryId`, and the seeded repo directory;
`ApiClient` has `createWorkspace`, `createWorkflow`, `createRepository`,
`createTask`, `listTasks`.

## Parallelism

Sequential (wave 3).

## Inputs

- Spec: "Scenarios" — task lands in the dedicated workspace; active workspace
  untouched.
- Plan: `docs/plans/improve-kandev-workspace/plan.md` E2E Tests section.
- Existing patterns: `mockImproveKandevApis` helper in
  `improve-kandev.spec.ts`; the "Open issue creates a task" test's
  `apiClient.listTasks` assertion.

## Output contract

Summary, files changed, exact e2e command + result (test count/pass),
blockers, risks, and task/plan status update in the same conversation.

## Risks

- The mocked `workspace_id` must reference a workspace that exists in the
  seeded backend, else `listRepositories` 404s and the dialog shows a
  bootstrap error. The new test seeds the dedicated workspace for this reason.
- The create-task endpoint validates workflow/workspace consistency; the
  mocked `workflow_id` must belong to the dedicated workspace (seeded via
  `createWorkflow(dedicated.id, ...)`).

## Results

E2E `apps/web/e2e/tests/improve-kandev.spec.ts` (13 desktop + 1 mobile) all pass:
`playwright test --config e2e/playwright.config.ts improve-kandev.spec` → 14 passed.
The 4 submit-flow tests needed `agent_generated_task_titles: false` in the seed
(after upstream main merged the auto-title setting, whose default hides the title
field); the earlier 4 failures were caused by a stale `apps/backend/bin/kandev`
e2e binary missing upstream's `agent_generated_task_titles` backend field —
rebuilt via `go build -o bin/kandev ./cmd/kandev`.
