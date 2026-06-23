# Link Existing Task to GitHub References Plan

## Scope

Implement issue #1470 by linking GitHub references to existing tasks:

- GitHub issues through task metadata and the existing GitHub issue fetch path.
- GitHub pull requests through the existing task PR association model.
- A shared **Link** submenu in task menus with separate **GitHub Pull Request** and **GitHub Issue** actions.

## Backend

- Add `PUT /api/v1/github/tasks/:taskId/issue` and `DELETE /api/v1/github/tasks/:taskId/issue`.
- Add GitHub service methods to parse URL/number input, fetch the issue, validate repository ownership, merge or remove issue metadata, and rely on task service update publishing.
- Reuse existing `POST /api/v1/github/task-prs` support for manual PR association.
- Add service tests for successful link, repository mismatch, and unlink.

## Frontend

- Add typed GitHub API helpers for link/unlink.
- Add responsive dialogs for GitHub issue and GitHub pull request URL/number input.
- Replace flat issue actions with a **Link** submenu containing **GitHub Pull Request** and **GitHub Issue**.
- Add that submenu to task card menus and sidebar task menus.
- Reuse existing issue metadata and task PR rendering on task cards and detail/sidebar surfaces.
- Add API helper unit tests.

## Verification

- `cd apps/backend && go test ./internal/github/...`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/github-api.test.ts lib/kanban/map-task.test.ts`
- `cd apps && pnpm --filter @kandev/web typecheck`
- `cd apps && pnpm --filter @kandev/web lint`
- Run format, typecheck, test, and lint before opening the PR.
