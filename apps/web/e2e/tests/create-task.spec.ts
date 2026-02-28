import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Task creation", () => {
  test("opens create task dialog from kanban header", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
  });

  test("can fill in task title and description", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const titleInput = testPage.getByTestId("task-title-input");
    await titleInput.fill("My E2E Test Task");
    await expect(titleInput).toHaveValue("My E2E Test Task");

    const descInput = testPage.getByTestId("task-description-input");
    await descInput.fill("This is a test description");
    await expect(descInput).toHaveValue("This is a test description");
  });

  test("start agent: creates task, starts session, navigates to session", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill in title and description — description enables the "Start task" button
    await testPage.getByTestId("task-title-input").fill("Start Agent Task");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    // The dialog auto-selects the E2E Repo (first available repository) and the main branch.
    // Wait for the button to become enabled (repo + branch + agent profile all resolved).
    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 10_000 });

    // Click "Start task" — the agent starts, the dialog closes, we stay on kanban
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // The new task card appears on the kanban board (pushed via WS)
    const card = kanban.taskCardByTitle("Start Agent Task");
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Clicking the card fetches the session and navigates to /s/<sessionId>
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Default layout: sidebar, terminal, and file tree are all visible
    await expect(session.sidebar).toBeVisible();
    await expect(session.terminal).toBeVisible();
    await expect(session.files).toBeVisible();
    await expect(session.chat).toBeVisible();

    // Task title appears in the sidebar
    await expect(session.taskInSidebar("Start Agent Task")).toBeVisible();

    // The mock agent's simple-message scenario emits this response text —
    // waiting for it confirms the agent ran and completed its turn.
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
  });
});
