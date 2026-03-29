---
name: e2e
description: Write and run web E2E tests (Playwright) using TDD — locations, patterns, commands, and debugging.
---

# E2E Tests

Write E2E tests using `/tdd` (Red-Green-Refactor). Always run the tests you create and watch them fail before implementing.

**Location:** `apps/web/e2e/`

```
apps/web/e2e/
├── fixtures/
│   ├── backend.ts           # Worker-scoped backend + frontend process
│   └── test-base.ts         # Extended fixture (apiClient, seedData, testPage)
├── helpers/
│   └── api-client.ts        # HTTP client for seeding data (read for available methods)
├── pages/                   # Page objects (read for available pages and methods)
└── tests/                   # Spec files (*.spec.ts)
```

Each worker gets an isolated backend, frontend, database, and mock agent — no Docker, no API keys needed.

## Run commands

```bash
make test-e2e                                                      # all tests, headless
make test-e2e-headed                                               # with visible browser
make test-e2e-ui                                                   # Playwright UI mode
cd apps && pnpm --filter @kandev/web e2e -- tests/my-test.spec.ts  # single file
cd apps && pnpm --filter @kandev/web e2e -- --grep "task creation" # by name
```

Prerequisites: `make build-backend build-web` (Make targets do this automatically).

## Writing a test

1. Read `helpers/api-client.ts` and `pages/` to discover available seed methods and page objects
2. Import fixtures from `../fixtures/test-base` — provides `testPage`, `apiClient`, and `seedData` (pre-created workspace with default workflow)
3. Use `data-testid` attributes for selectors — add them to components as needed
4. Use page objects for common interactions; create new ones for new pages
5. For GitHub features, use `apiClient.mockGitHub*()` methods to seed mock data

Example:

```typescript
import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("my feature", () => {
  test("does something", async ({ testPage, seedData, apiClient }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Test Task", "Description");
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workspaceId);
    await expect(kanban.taskCardByTitle("Test Task")).toBeVisible();
  });
});
```

## Test quality guidelines

- **Test through the UI, not the API.** E2E tests verify user-facing behavior. Don't write tests that only call the API and assert the response -- those are integration tests. Instead, navigate to the page, interact with UI elements, and assert what the user sees.
- **Verify persistence with page reload.** After changing a setting or creating data, reload the page (`testPage.reload()`) and assert the state is still correct. This catches hydration bugs and SSR/client mismatches.
- **Seed via API, assert via UI.** Use `apiClient` to set up preconditions quickly, but always verify the result by opening the page and checking the DOM.

## Debugging failures

```bash
E2E_DEBUG=1 make test-e2e          # see backend/frontend stderr
make test-e2e-ui                   # step through interactively
make test-e2e-report               # open HTML report from last run
```

- **"Backend did not become healthy"** — run `make build-backend build-web`, check with `E2E_DEBUG=1`
- **"Cannot find module"** — run `cd apps && pnpm install`
- **Port conflicts** — backends use 18080+, frontends use 13000+ (per worker)
- **Flaky timeouts** — **never increase locator timeouts to fix flaky tests.** If a locator times out, the root cause is almost always something else: a setup failure, missing navigation, race condition, or the element genuinely not rendering. Investigate why the element never appears instead of giving it more time. Note: infrastructure health timeouts (30s in `fixtures/backend.ts`) and overall test timeouts (60s in `playwright.config.ts`) are separate and should not be modified either.
- Screenshots on failure, video on first retry (CI)

## TDD workflow

Follow `/tdd` when writing E2E tests:

1. **RED** — Write the spec, run it, watch it fail (missing `data-testid`, feature not implemented, etc.)
2. **GREEN** — Implement the feature/fix, add `data-testid` attributes, run the test until green
3. **REFACTOR** — Extract page objects, clean up selectors, keep tests green
4. Run `/verify` when done
