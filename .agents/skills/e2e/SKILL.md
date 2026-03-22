---
name: e2e
description: Write Playwright E2E tests following project patterns and best practices
---

# Writing Playwright E2E Tests

## Golden Rule

**Every test step must follow the exact flow a real user would.** Never use `apiClient` or direct API calls for actions that a user performs through the UI. The test should click, type, and navigate — not call endpoints.

## When API is Acceptable

- **Fixture setup** in `test-base.ts` (workspace, workflow, seed data) — this is infrastructure, not user behavior
- **`e2eReset`** between tests — cleanup, not user action
- Never for creating tasks, subtasks, or any entity the user would create through a dialog or button
- Never for verifying results — assert via visible UI elements instead

## Project Structure

```
apps/web/e2e/
├── fixtures/test-base.ts    # Test fixtures: testPage, apiClient, seedData
├── pages/
│   ├── kanban-page.ts       # KanbanPage: board, createTaskButton, taskCardByTitle()
│   └── session-page.ts      # SessionPage: chat, sidebar, idleInput(), sendMessage(), etc.
└── tests/*.spec.ts          # Test files
```

## Steps

1. **Read the fixtures and page objects first** — understand `SeedData`, `KanbanPage`, `SessionPage` before writing anything.
2. **Identify the user flow** — write down exactly what a user would click, type, and see. Every step maps to a locator interaction.
3. **Use page objects** — never write raw `page.locator(...)` when a page object method exists.
4. **Use `data-testid` locators** — prefer `getByTestId()` over CSS selectors or text matchers.
5. **Use `taskCardByTitle()` for kanban cards** — pass a unique title or regex; if ambiguity is possible, filter further.
6. **Wait for UI state, not time** — use `toBeVisible({ timeout })`, `toBeEnabled({ timeout })`, never `page.waitForTimeout()`.
7. **Verify through the UI** — navigate to the page where the result should be visible and assert on DOM elements.

## Fixtures and Seed Data

```typescript
import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test("example", async ({ testPage, seedData }) => {
  // testPage: a fresh Page with localStorage pre-seeded
  // seedData: { workspaceId, workflowId, startStepId, repositoryId, agentProfileId, steps }
});
```

## Common Patterns

### Create a task via UI dialog

```typescript
const kanban = new KanbanPage(testPage);
await kanban.goto();
await kanban.createTaskButton.first().click();

const dialog = testPage.getByTestId("create-task-dialog");
await expect(dialog).toBeVisible();
await testPage.getByTestId("task-title-input").fill("My Task");
await testPage.getByTestId("task-description-input").fill("description or /e2e:simple-message");

const startBtn = testPage.getByTestId("submit-start-agent");
await expect(startBtn).toBeEnabled({ timeout: 30_000 });
await startBtn.click();
await expect(dialog).not.toBeVisible({ timeout: 10_000 });
```

### Wait for agent to complete

```typescript
const parentCard = kanban.taskCardByTitle("My Task");
await expect(parentCard).toBeVisible({ timeout: 10_000 });
await parentCard.click();
await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

const session = new SessionPage(testPage);
await session.waitForLoad();
await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
```

### Create subtask via UI button

```typescript
await testPage.getByTestId("new-subtask-button").click();
await expect(testPage.getByTestId("subtask-title-input")).toBeVisible();
await testPage.getByTestId("subtask-prompt-input").fill("/e2e:simple-message");

await testPage.getByRole("button", { name: "Create Subtask" }).click();
// Navigates to the new subtask session
await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });
```

### Agent creates subtask via MCP tool

Use the mock agent's e2e script with `e2e:mcp:kandev:create_task`:

```typescript
const script = [
  'e2e:thinking("Planning...")',
  'e2e:mcp:kandev:create_task({"parent_id":"self","title":"My Subtask"})',
  'e2e:message("Done.")',
].join("\n");

// Pass this as the task description in the create task dialog
```

`"self"` resolves to the agent's current task ID. You can also use `{task_id}` placeholder.

### Verify a card on the kanban board

```typescript
await kanban.goto();
const card = kanban.taskCardByTitle("Expected Title");
await expect(card).toBeVisible({ timeout: 10_000 });
// Check parent badge on subtask cards
await expect(card.getByText("Parent Title")).toBeVisible();
```

## Rules

- **No `apiClient` in test body** — only in fixtures/setup
- **No `page.waitForTimeout()`** — use condition-based waits
- **Unique titles** — use descriptive, unique task/subtask titles to avoid locator ambiguity
- **One flow per test** — each test exercises one specific user journey
- **Assert visible state** — always verify the end state through the UI (kanban cards, session page, badges)
- **Clean up is automatic** — `e2eReset` in fixtures handles cleanup between tests

## Mock Agent Script Commands

| Command | Description |
|---------|-------------|
| `e2e:message("text")` | Agent sends a text message |
| `e2e:thinking("text")` | Agent sends a thinking block |
| `e2e:delay(ms)` | Wait for N milliseconds |
| `e2e:mcp:kandev:<tool>({json})` | Call an MCP tool (e.g., `create_task`, `create_task_plan`) |
| `/e2e:simple-message` | Shorthand: agent sends `"simple mock response"` and completes |
