# Link Existing Task to GitHub Issue Plan

## Scope

Implement issue #1470 by linking GitHub issues to existing tasks through task metadata and the existing GitHub issue fetch path.

## Backend

- Add `PUT /api/v1/github/tasks/:taskId/issue` and `DELETE /api/v1/github/tasks/:taskId/issue`.
- Add GitHub service methods to parse URL/number input, fetch the issue, validate repository ownership, merge or remove issue metadata, and rely on task service update publishing.
- Add service tests for successful link, repository mismatch, and unlink.

## Frontend

- Add typed GitHub API helpers for link/unlink.
- Add a task card menu action and a responsive dialog for URL/number input.
- Reuse existing issue metadata rendering on task cards and detail/sidebar surfaces.
- Add API helper unit tests.

## Verification

- `cd apps/backend && go test ./internal/github/...`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/github-api.test.ts lib/kanban/map-task.test.ts`
- Run format, typecheck, test, and lint before opening the PR.
