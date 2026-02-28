# Web E2E Tests

Browser-based end-to-end tests that exercise the full stack: Next.js frontend, Go backend, and mock agent. Tests run in parallel with complete isolation — each Playwright worker gets its own backend process, frontend process, database, and temp directory.

**Location:** `apps/web/e2e/`

## Architecture

```
┌─ Playwright ──────────────────────────────────────────────┐
│  Worker 0                         Worker 1                │
│  ┌──────────────────────┐        ┌──────────────────────┐│
│  │ Backend  :18080      │        │ Backend  :18081      ││
│  │ Frontend :13000      │        │ Frontend :13001      ││
│  │ SSR → localhost:18080│        │ SSR → localhost:18081││
│  │ HOME=/tmp/e2e-0      │        │ HOME=/tmp/e2e-1      ││
│  │ own SQLite DB        │        │ own SQLite DB        ││
│  │ mock-agent only      │        │ mock-agent only      ││
│  │ mock GitHub          │        │ mock GitHub          ││
│  └──────────────────────┘        └──────────────────────┘│
└───────────────────────────────────────────────────────────┘
```

- **One backend per worker** — port `18080 + workerIndex`, isolated temp HOME, own SQLite DB
- **One frontend per worker** — port `13000 + workerIndex`, SSR routed to that worker's backend via `KANDEV_API_BASE_URL`
- **Shared `.next/` build** — all frontend processes serve from the same pre-built output (read-only)
- **Mock agent** — deterministic responses, no API keys needed
- **Mock GitHub client** — reports as authenticated, returns configurable data via REST API
- **No external dependencies** — no Docker, no GitHub tokens, no real git credentials

## Prerequisites

Build the backend binaries and web app before running tests:

```bash
make build-backend build-web
```

This produces `apps/backend/bin/kandev`, `apps/backend/bin/mock-agent`, and `apps/web/.next/`. The global setup script verifies all exist.

## Running Tests

All commands run from the repo root via Make targets (these build automatically):

```bash
make test-e2e            # Headless, parallel (default)
make test-e2e-headed     # Visible browser window
make test-e2e-ui         # Playwright UI mode (step through, inspect)
make test-e2e-report     # Open HTML report from last run
```

Or directly via pnpm (requires pre-built binaries and web app):

```bash
cd apps
pnpm --filter @kandev/web e2e                              # All tests
pnpm --filter @kandev/web e2e -- --grep "task creation"    # By name
pnpm --filter @kandev/web e2e -- tests/create-task.spec.ts # Single file
pnpm --filter @kandev/web e2e:headed                       # With browser
pnpm --filter @kandev/web e2e:ui                           # UI mode
```

### Debug output

Set `E2E_DEBUG=1` to see backend and frontend stderr in the terminal:

```bash
E2E_DEBUG=1 make test-e2e
```

## Directory Structure

```
apps/web/e2e/
├── playwright.config.ts     # Playwright configuration
├── global-setup.ts          # Verifies backend binaries and web build exist
├── fixtures/
│   ├── backend.ts           # Worker-scoped backend + frontend process fixture
│   └── test-base.ts         # Extended test fixture (apiClient, seedData, testPage)
├── helpers/
│   └── api-client.ts        # HTTP client for API seeding
├── pages/
│   ├── kanban-page.ts       # Kanban board page object
│   └── task-detail-page.ts  # Task detail page object
└── tests/
    ├── kanban-board.spec.ts  # Kanban board display
    ├── create-task.spec.ts   # Task creation flows
    ├── task-detail.spec.ts   # Task detail views
    └── workflow-steps.spec.ts # Workflow step progression
```

## How It Works

### Backend + frontend fixture (`fixtures/backend.ts`)

Each Playwright worker spawns an isolated `kandev` backend and a dedicated `next start` frontend:

**Backend:**
1. Creates a temp directory as `HOME` with its own `.gitconfig`, data dir, and SQLite DB
2. Sets environment variables for isolation:
   - `KANDEV_DATA_DIR` — temp data directory
   - `KANDEV_SERVER_PORT` — unique port per worker (`18080 + workerIndex`)
   - `KANDEV_DATABASE_PATH` — temp SQLite path
   - `KANDEV_MOCK_AGENT=true` — only loads mock agent, skips agent discovery
   - `KANDEV_MOCK_GITHUB=true` — uses in-memory MockClient instead of real GitHub API
   - `KANDEV_DOCKER_ENABLED=false` — no Docker
   - `KANDEV_WORKTREE_ENABLED=false` — no worktrees
   - `GH_TOKEN` / `GITHUB_TOKEN` stripped — prevents accidental real API calls
3. Spawns `apps/backend/bin/kandev` and waits for `/health` to return 200

**Frontend:**
1. Spawns `npx next start --port {13000 + workerIndex}` from `apps/web/`
2. Sets `KANDEV_API_BASE_URL` to the worker's backend URL so SSR routes correctly
3. Waits for the frontend to become healthy
4. All `next start` processes share the same `.next/` build output (read-only)

**Teardown:** sends SIGTERM to frontend then backend, falls back to SIGKILL after 5s, cleans up temp dir.

### Test base (`fixtures/test-base.ts`)

Extends the backend fixture with:

- **`apiClient`** (worker-scoped) — HTTP client for seeding data via the backend API
- **`seedData`** (worker-scoped) — creates a workspace, discovers the default workflow and its steps
- **`testPage`** (per-test) — Playwright page with a browser context whose `baseURL` points to the worker's frontend, plus `localStorage` pre-configured:
  - `kandev.onboarding.completed = "true"` — skips onboarding
  - `window.__KANDEV_API_BASE_URL` — points to the worker's backend (client-side override)

### API client (`helpers/api-client.ts`)

Seeds test data via REST API. Available methods:

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `createWorkspace(name)` | `POST /api/workspaces` | Create workspace |
| `createTask(workspaceId, title, desc)` | `POST /api/tasks` | Create task |
| `listWorkflows(workspaceId)` | `GET /api/workspaces/:id/workflows` | List workflows |
| `listWorkflowSteps(workflowId)` | `GET /api/workflows/:id/workflow/steps` | List steps |
| `createWorkflowStep(name, pos)` | `POST /api/workflow/steps` | Create step |
| `moveTask(taskId, stepId)` | `POST /api/tasks/:id/move` | Move task to step |

**GitHub mock control:**

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `mockGitHubReset()` | `DELETE /api/v1/github/mock/reset` | Clear all mock data |
| `mockGitHubSetUser(username)` | `PUT /api/v1/github/mock/user` | Set authenticated user |
| `mockGitHubAddPRs(prs)` | `POST /api/v1/github/mock/prs` | Add mock PRs |
| `mockGitHubAddOrgs(orgs)` | `POST /api/v1/github/mock/orgs` | Add mock organizations |
| `mockGitHubAddRepos(org, repos)` | `POST /api/v1/github/mock/repos` | Add mock repositories |
| `mockGitHubAddReviews(owner, repo, num, reviews)` | `POST /api/v1/github/mock/reviews` | Add PR reviews |
| `mockGitHubAddCheckRuns(owner, repo, ref, checks)` | `POST /api/v1/github/mock/checks` | Add CI check runs |
| `mockGitHubGetStatus()` | `GET /api/v1/github/status` | Verify auth status |

### Page objects (`pages/`)

Page objects encapsulate selectors and navigation:

- **`KanbanPage`** — `goto()`, `board`, `createTaskButton`, `taskCard(id)`, `taskCardByTitle(title)`
- **`TaskDetailPage`** — `goto(taskId, sessionId)`, `sessionChat`, `turnCompleteIndicator`, `waitForAgentResponse(text)`

### Selectors

Tests use `data-testid` attributes for stability across layout/styling changes:

| `data-testid` | Component |
|----------------|-----------|
| `kanban-board` | Kanban board container |
| `task-card-{id}` | Individual task card |
| `create-task-button` | Create task button |
| `create-task-dialog` | Task creation dialog |
| `task-title-input` | Title input in dialog |
| `task-description-input` | Description textarea in dialog |
| `submit-start-agent` | Start agent button |
| `submit-plan-mode` | Plan mode option |
| `session-chat` | Session chat panel |
| `agent-turn-complete` | Agent turn complete indicator |

## Writing New Tests

### 1. Create a spec file

```typescript
// apps/web/e2e/tests/my-feature.spec.ts
import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("my feature", () => {
  test("does something", async ({ testPage, seedData, apiClient }) => {
    // Seed data via API
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Test Task",
      "Description",
    );

    // Navigate and interact
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workspaceId);

    // Assert
    await expect(kanban.taskCardByTitle("Test Task")).toBeVisible();
  });
});
```

### 2. Add data-testid attributes

When testing new UI components, add `data-testid` to the relevant elements:

```tsx
<div data-testid="my-component">...</div>
```

Then reference them in page objects or directly:

```typescript
page.getByTestId("my-component");
```

### 3. Seed mock GitHub data

The mock GitHub client (`KANDEV_MOCK_GITHUB=true`) reports as authenticated and serves data seeded via the `/api/v1/github/mock/` endpoints. Use the `apiClient` helper methods:

```typescript
test("shows PR feedback in settings", async ({ testPage, apiClient, seedData }) => {
  // Seed a mock PR
  await apiClient.mockGitHubAddPRs([{
    number: 42,
    title: "Add feature X",
    state: "open",
    head_branch: "feature-x",
    base_branch: "main",
    author_login: "mock-user",
    repo_owner: "myorg",
    repo_name: "myrepo",
    html_url: "https://github.com/myorg/myrepo/pull/42",
  }]);

  // Seed orgs for the org picker
  await apiClient.mockGitHubAddOrgs([{ login: "myorg" }]);

  // Seed repos for the repo search
  await apiClient.mockGitHubAddRepos("myorg", [
    { full_name: "myorg/myrepo", owner: "myorg", name: "myrepo" },
  ]);

  // Navigate and test GitHub UI features...
});
```

Call `mockGitHubReset()` between tests if you need a clean slate (each worker already starts fresh).

### 4. Extend the API client

If your test needs new seed data, add methods to `helpers/api-client.ts`:

```typescript
async createMyEntity(params: { ... }): Promise<MyEntity> {
  return this.post("/api/my-entities", params);
}
```

## Configuration

| Setting | Value | Notes |
|---------|-------|-------|
| `fullyParallel` | `true` | Tests run in parallel across workers |
| `workers` | auto (local), 2 (CI) | Worker count |
| `timeout` | 60s | Per-test timeout |
| `retries` | 0 (local), 2 (CI) | Retry count |
| `trace` | on-first-retry | Playwright trace recording |
| `screenshot` | only-on-failure | Auto-screenshot |
| `video` | on-first-retry | Video recording |
| Browser | Chromium only | Single browser project |

## CI

The GitHub Actions workflow (`.github/workflows/e2e-tests.yml`) runs on pushes and PRs to `main` that touch `apps/backend/`, `apps/web/`, or `apps/packages/`. It:

1. Builds Go binaries (`make build-backend`)
2. Installs pnpm dependencies
3. Installs Playwright Chromium
4. Runs `make test-e2e`
5. Uploads `playwright-report/` and `test-results/` as artifacts on failure

## Troubleshooting

**Tests fail with "Backend did not become healthy" or "Service did not become healthy"**
- Ensure `make build-backend build-web` completed successfully
- Check that `apps/backend/bin/kandev`, `apps/backend/bin/mock-agent`, and `apps/web/.next/` exist
- Run with `E2E_DEBUG=1` to see backend and frontend stderr

**Tests fail with "Cannot find module" or import errors**
- Run `cd apps && pnpm install` to ensure dependencies are up to date

**Port conflicts**
- Tests use ports 18080+ for backends and 13000+ for frontends (one per worker)
- Ensure nothing else is listening on those ports

**Flaky timeouts**
- Backend and frontend health checks have a 30s timeout; if your machine is slow, this may need increasing in `fixtures/backend.ts`
- Per-test timeout is 60s; adjust in `playwright.config.ts` if needed
