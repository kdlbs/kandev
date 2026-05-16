import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression test: navigating from a task with chat messages to a sessionless
 * task must not display the previous task's messages.
 *
 * Root cause: setActiveTask() only set activeTaskId without clearing
 * activeSessionId, so the stale session from the previously viewed task
 * remained in the store and its messages appeared on the new task page.
 */
test.describe("Stale session navigation", () => {
  test("navigating from a task with messages to a sessionless task does not show stale chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Create Task A with an agent session that produces messages
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task With Messages",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Navigate to Task A's session page and wait for its message.
    // We wait for the chat message directly instead of polling session state
    // because the mock agent emits the message quickly; the backend session
    // state transition to COMPLETED/WAITING_FOR_INPUT can be slow in CI.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCardByTitle("Task With Messages");
    await expect(cardA).toBeVisible({ timeout: 15_000 });
    await cardA.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(
      session.activeChat().getByText("simple mock response", { exact: false }),
    ).toBeVisible({
      timeout: 30_000,
    });

    // 3. Create Task B (no agent session — simulates a PR watcher or new task)
    await apiClient.createTask(seedData.workspaceId, "Sessionless Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // 4. Go back to kanban
    await kanban.goto();
    await expect(kanban.board).toBeVisible({ timeout: 10_000 });

    // 5. Click on Task B from kanban
    const cardB = kanban.taskCardByTitle("Sessionless Task");
    await expect(cardB).toBeVisible({ timeout: 10_000 });
    await cardB.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // 6. Verify Task B's page shows the correct title
    await expect(testPage.getByText("Sessionless Task")).toBeVisible({ timeout: 10_000 });

    // 7. Task A's messages must NOT appear on Task B's page
    await expect(testPage.getByText("simple mock response", { exact: false })).not.toBeVisible({
      timeout: 5_000,
    });
  });
});
