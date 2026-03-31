---
name: e2e
description: Write and run web E2E tests (Playwright) using TDD — locations, patterns, commands, and debugging.
---

# E2E Tests

Write E2E tests using TDD (Red-Green-Refactor). Always run the tests you create and watch them fail before implementing.

## Available skills and subagents

- **`/tdd`** — Follow the Red-Green-Refactor cycle when writing tests.
- **`/verify`** — Run after completing tests to ensure everything passes across the monorepo.
- **`/playwright-cli`** — Interactive browser automation. Use to validate features against the dev server before writing tests, and to debug failing tests with `--debug=cli`.

## Location

`apps/web/e2e/`

```
apps/web/e2e/
├── fixtures/
│   ├── backend.ts           # Worker-scoped backend + frontend process
│   └── test-base.ts         # Extended fixture (apiClient, seedData, testPage)
├── helpers/
│   └── api-client.ts        # HTTP client for seeding data (read for available methods)
├── pages/                   # Page objects (read for available pages and methods)
└── tests/                   # Spec files (*.spec.ts), grouped by feature
    ├── task/                # Task creation, deletion, archiving, environment, subtasks
    ├── kanban/              # Kanban board, mobile kanban, preview panel
    ├── session/             # Session lifecycle, resume, recovery, multi-session, layout
    ├── workflow/            # Workflow steps, settings, automation, import/export
    ├── git/                 # Git changes panel, commits, diffs, symlinks
    ├── pr/                  # PR detection, watchers, changes panel
    ├── terminal/            # Terminal agent, keyboard, settings
    ├── chat/                # Quick chat, message queue, clarification, markdown, toolbar
    ├── settings/            # Config management, agent profiles, editor integration
    └── review/              # Code review diffs
```

Each worker gets an isolated backend, frontend, database, and mock agent — no Docker, no API keys needed.

## Run commands

```bash
make test-e2e                                                      # all tests, headless
make test-e2e-headed                                               # with visible browser
make test-e2e-ui                                                   # Playwright UI mode
cd apps && pnpm --filter @kandev/web e2e -- tests/task/my-test.spec.ts  # single file
cd apps && pnpm --filter @kandev/web e2e -- --grep "task creation" # by name
```

Prerequisites: `make build-backend build-web` (Make targets do this automatically).

## Writing a test

1. Read `helpers/api-client.ts` and `pages/` to discover available seed methods and page objects
2. Import fixtures from `../../fixtures/test-base` — provides `testPage`, `apiClient`, and `seedData` (pre-created workspace with default workflow)
3. Use `data-testid` attributes for selectors — add them to components as needed
4. Use page objects for common interactions; create new ones for new pages
5. For GitHub features, use `apiClient.mockGitHub*()` methods to seed mock data

Example:

```typescript
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("my feature", () => {
  test("does something", async ({ testPage, seedData, apiClient }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Test Task", "Description");
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workspaceId);
    await expect(kanban.taskCardByTitle("Test Task")).toBeVisible();
  });
});
```

## Dev-first workflow

Before writing an E2E test, validate the feature works interactively using `playwright-cli` against a dev server. This gives a fast feedback loop — code changes are picked up by hot reload in ~1-2 seconds, no production rebuild needed. Once confirmed working, translate the interactions into a proper E2E test.

### Start the dev environment

Multiple agents may run in parallel, so use random ports to avoid collisions. Fixture ports use 18080-18089 (backend) and 13000-13009 (frontend) — stay outside those ranges.

```bash
OFFSET=$((RANDOM % 100))
BACKEND_PORT=$((19000 + OFFSET))
FRONTEND_PORT=$((14000 + OFFSET))
```

Start the backend:
```bash
E2E_TMP=$(mktemp -d) && mkdir -p "$E2E_TMP/.kandev" && \
printf '[user]\n  name = E2E Test\n  email = e2e@test.local\n[commit]\n  gpgsign = false\n' > "$E2E_TMP/.gitconfig" && \
HOME="$E2E_TMP" KANDEV_DATA_DIR="$E2E_TMP/.kandev" KANDEV_SERVER_PORT=$BACKEND_PORT \
KANDEV_DATABASE_PATH="$E2E_TMP/kandev.db" KANDEV_MOCK_AGENT=only \
KANDEV_MOCK_GITHUB=true KANDEV_DOCKER_ENABLED=false KANDEV_WORKTREE_ENABLED=false \
KANDEV_LOG_LEVEL=warn apps/backend/bin/kandev &
```

Start the dev frontend:
```bash
KANDEV_API_BASE_URL=http://localhost:$BACKEND_PORT NEXT_PUBLIC_KANDEV_API_PORT=$BACKEND_PORT \
pnpm --filter @kandev/web dev --port $FRONTEND_PORT &
```

### Validate with playwright-cli

```bash
playwright-cli open http://localhost:$FRONTEND_PORT
playwright-cli snapshot                    # see page structure and element refs
playwright-cli click e5                    # interact using refs from snapshot
playwright-cli fill e3 "test input"
playwright-cli snapshot                    # verify result
```

### Fast iteration cycle

1. Make a code change in `apps/web/`
2. HMR picks it up in ~1-2 seconds
3. `playwright-cli snapshot` or `playwright-cli screenshot` to verify
4. Repeat until the flow works correctly

### Translate to E2E test

Once validated, write the Playwright test using project fixtures and page objects. The `playwright-cli` interactions map directly to Playwright API calls:

| playwright-cli | Playwright API |
|---|---|
| `playwright-cli click e5` | `page.getByTestId('...').click()` |
| `playwright-cli fill e3 "text"` | `page.getByTestId('...').fill('text')` |
| `playwright-cli snapshot` (verify element visible) | `expect(page.getByTestId('...')).toBeVisible()` |

Use `data-testid` selectors in the test (not snapshot refs), and wrap common flows in page objects.

### Capture PR evidence

After confirming the feature works, capture screenshots or a video as proof for the PR:

```bash
# Screenshots of key states
playwright-cli screenshot --filename=apps/web/.pr-assets/feature-before.png
# ... interact to show the feature ...
playwright-cli screenshot --filename=apps/web/.pr-assets/feature-after.png

# Or record a video walkthrough
playwright-cli video-start apps/web/.pr-assets/feature-demo.webm
# ... perform the user flow ...
playwright-cli video-stop
```

Create `apps/web/.pr-assets/manifest.json` so the `/pr` skill picks them up:
```json
{
  "assets": [
    {"name": "feature-demo", "file": "feature-demo.webm", "format": "gif", "caption": "Feature demo"},
    {"name": "feature-after", "file": "feature-after.png", "format": "png", "caption": "Result"}
  ]
}
```

### Final verification

Always verify against the production build before finishing — dev mode can hide SSR/hydration issues:

```bash
playwright-cli close
# Kill dev server and backend
make build-web
cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/test.spec.ts
```

## Test organization

Tests are grouped by feature area in subdirectories under `tests/`. When creating a new test:

- **Place it in the matching feature directory.** A test for PR detection goes in `pr/`, a test for session resume goes in `session/`, etc.
- **Merge related tests into the same file.** Tests covering the same feature (e.g., git commit body and pre-hooks) belong in one file with separate `test.describe` blocks. Don't create a new file for each narrow scenario.
- **Import paths from subdirectories** use `../../` (e.g., `from "../../fixtures/test-base"`).
- **Standalone root files** are allowed for truly cross-cutting tests that don't fit any group.

## Test quality guidelines

- **Test through the UI, not the API.** E2E tests verify user-facing behavior. Don't write tests that only call the API and assert the response -- those are integration tests. Instead, navigate to the page, interact with UI elements, and assert what the user sees.
- **Verify persistence with page reload.** After changing a setting or creating data, reload the page (`testPage.reload()`) and assert the state is still correct. This catches hydration bugs and SSR/client mismatches.
- **Seed via API, assert via UI.** Use `apiClient` to set up preconditions quickly, but always verify the result by opening the page and checking the DOM.

## Debugging failures

### Triage

When a test fails:

1. **Read the error output** — the Playwright error message, expected vs. actual, and which locator timed out
2. **Read the failure screenshot** from `e2e/test-results/` — see what the page actually rendered
3. **Attach to the failure** for deeper debugging using `playwright-cli`:
   ```bash
   cd apps && PLAYWRIGHT_HTML_OPEN=never pnpm --filter @kandev/web e2e -- tests/path.spec.ts --debug=cli &
   # Wait for "Debugging Instructions" with session name
   playwright-cli attach tw-<session>
   playwright-cli snapshot    # inspect page state at failure point
   playwright-cli console     # check for JS errors
   playwright-cli network     # check API responses
   ```

### Classify and fix

| Category | Signals | Fast loop |
|---|---|---|
| **Test logic** | Wrong selector, wrong expected text, missing page object method | Fix test files, re-run immediately (no rebuild -- Playwright transpiles TS at runtime) |
| **Frontend-only** | Screenshot shows wrong UI, missing element, client error. API calls succeed. | Start dev server, fix with hot reload, verify with `playwright-cli`, then `make build-web` + re-run test |
| **Backend** | 500 errors, wrong API response, "Backend did not become healthy" | Fix Go code, `make build-backend`, re-run test |

### Common issues

- **"Backend did not become healthy"** — run `make build-backend build-web`, check with `E2E_DEBUG=1`
- **"Cannot find module"** — run `cd apps && pnpm install`
- **Port conflicts** — backends use 18080+, frontends use 13000+ (per worker)
- **Flaky timeouts** — **never increase locator timeouts to fix flaky tests.** If a locator times out, the root cause is almost always something else: a setup failure, missing navigation, race condition, or the element genuinely not rendering. Investigate why the element never appears instead of giving it more time. Note: infrastructure health timeouts (30s in `fixtures/backend.ts`) and overall test timeouts (60s in `playwright.config.ts`) are separate and should not be modified either.
- Screenshots on failure, video on first retry (CI)

### Debugging CI shard failures

CI splits tests across 10 shards. To reproduce a specific shard locally:

```bash
# List which tests are in a shard
npx playwright test --config e2e/playwright.config.ts --shard=2/10 --list

# Run that shard locally (requires production build)
make build-backend build-web
cd apps/web && npx playwright test --config e2e/playwright.config.ts --shard=2/10
```

E2E tests run against the **production build** (`next build`), not dev mode. Always rebuild with `make build-web` (or `pnpm --filter @kandev/web build`) after code changes before running E2E tests locally.

## Selector guidelines

- **Prefer `data-testid` selectors** over text-based locators. Text content can change when UI is updated (e.g., hiding a badge), breaking tests that match by text. Use `getByTestId()` or `locator("[data-testid='...']")` for stable targeting.
- **Use page object methods** like `clickSessionChatTab()` (stable `data-testid`) instead of `sessionTabByText("1")` (fragile text match) for session tabs.
- **Dropdown menus can detach** from the DOM when React re-renders the parent (e.g., WS events updating the sidebar). The `openSidebarMenuAndClick()` helper in `session-page.ts` retries the full open-click sequence on detachment — use this pattern for similar interactions.

## TDD workflow

Follow `/tdd` when writing E2E tests:

1. **RED** — Write the spec, run it, watch it fail (missing `data-testid`, feature not implemented, etc.)
2. **GREEN** — Implement the feature/fix, add `data-testid` attributes, run the test until green
3. **REFACTOR** — Extract page objects, clean up selectors, keep tests green
4. Run `/verify` when done
