---
spec: docs/specs/tasks/link-existing-task-github-issue.md
created: 2026-07-13
status: completed
---

# Implementation Plan: GitHub Issue Task Linking

## Overview

Reuse the metadata-backed GitHub issue link introduced for existing tasks. Add a workspace reverse lookup, call the existing link endpoint after issue quick-launch creation, and reuse the PR task-indicator interaction for issue rows without changing issue-watch deduplication.

## Backend

- Add `GET /api/v1/github/task-issues?workspace_id=<id>` in `apps/backend/internal/github/controller.go`.
- Extend `apps/backend/internal/github/service_task_issue.go` with a workspace lookup that normalizes manual-link and issue-watch metadata into `TaskIssueLinkResponse` records grouped by task ID.
- Add an indexed workspace-scoped metadata projection in `apps/backend/internal/github/store.go`; it reads `tasks.id`, `tasks.title`, and `tasks.metadata` without adding a second association table.

## Frontend

- Add the workspace task-issue API and types in `apps/web/lib/api/domains/github-api.ts` and `apps/web/lib/types/github.ts`.
- Add `taskIssues.byTaskId` state and a `useIssueKeyToTasks` reverse-map hook following `usePRKeyToTasks`.
- Call `linkTaskIssue` from `quick-task-launcher.tsx` after an issue task is created.
- Extract the shared task-row indicator interaction from `PRRowTaskIndicator`, then render it from `issue-list.tsx` only for linked issues.
- Pass the reverse map through `github-page-client.tsx`. Existing responsive row wrapping remains the desktop and mobile layout.

## Tests

- Backend service/store/controller tests cover manual and watched metadata shapes, malformed metadata, required workspace scope, and cross-workspace isolation.
- Frontend API, store, hook, launcher, and indicator tests cover fetch shape, grouping multiple tasks, the automatic link call, empty issue behavior, and navigation labels.

## E2E Tests

- Desktop: seed linked and unlinked issues, verify single/multiple indicators, and navigate to a linked task.
- Mobile: open the issues view, verify the linked indicator remains reachable without horizontal overflow, and tap through to the task.

## Implementation Waves

1. [x] [task-01-backend-reverse-lookup](task-01-backend-reverse-lookup.md)
2. [x] [task-02-frontend-link-and-indicator](task-02-frontend-link-and-indicator.md)
3. [x] [task-03-e2e-and-verification](task-03-e2e-and-verification.md)

## Verification

```bash
cd apps/backend && go test ./internal/github/...
cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/github-api.test.ts lib/state/slices/github/github-slice.test.ts hooks/domains/github/use-issue-key-to-tasks.test.ts components/github/my-github/quick-task-launcher.test.tsx components/github/my-github/pr-row-task-indicator.test.tsx
cd apps/web && pnpm e2e:run tests/github/issue-list-task-indicator.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/github/mobile-issue-list-task-indicator.spec.ts
make fmt
make typecheck test lint
```

## Risks

- Metadata has two repository shapes; URL parsing is the canonical normalization path.
- A task may be archived or absent from hydrated kanban snapshots, so the API must include a task-title fallback.
- Automatic linking is a follow-up request after task creation and must not turn a successful task creation into a failed dialog submission.
