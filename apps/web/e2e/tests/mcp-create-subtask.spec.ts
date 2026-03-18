import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

const START_AGENT_TEST_ID = "submit-start-agent";
const START_ENABLED_TIMEOUT = 30_000;

test.describe("MCP create_task subtask", () => {
  test("agent creates subtask via MCP create_task with parent_id", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Use a subtask title that won't appear in the parent card's description
    const subtaskTitle = "MCP-subtask-e2e-verify";

    const script = [
      'e2e:thinking("Planning subtasks...")',
      "e2e:delay(100)",
      `e2e:mcp:kandev:create_task({"parent_id":"{task_id}","title":"${subtaskTitle}"})`,
      "e2e:delay(100)",
      'e2e:message("Done.")',
    ].join("\n");

    // 1. Open kanban and create task via the UI dialog
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("MCP Subtask Parent");
    await testPage.getByTestId("task-description-input").fill(script);

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // 2. Click the parent task card to navigate to its session
    const parentCard = kanban.taskCardByTitle("MCP Subtask Parent");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await parentCard.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    // 3. Wait for the agent to complete — the MCP create_task call happens during execution
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // 4. Verify the subtask was created via API
    const { tasks } = await apiClient.listTasks(seedData.workspaceId);
    const subtask = tasks.find((t) => t.title === subtaskTitle);
    expect(subtask).toBeDefined();

    // 5. Go back to kanban — subtask card should be visible with parent badge
    await kanban.goto();

    const subtaskCard = testPage.getByTestId(`task-card-${subtask!.id}`);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
    await expect(subtaskCard.getByText(subtaskTitle)).toBeVisible();
    // Subtask badge shows parent title
    await expect(subtaskCard.getByText("MCP Subtask Parent")).toBeVisible();
  });
});
