import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

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
    const taskA = await apiClient.createTaskWithAgent(
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

    // 2. Wait for Task A's session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskA.id);
          return DONE_STATES.includes(sessions[0]?.state);
        },
        { timeout: 30_000, message: "Waiting for Task A session to finish" },
      )
      .toBe(true);

    // 3. Navigate to Task A's session page to populate store with its messages
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCardByTitle("Task With Messages");
    await expect(cardA).toBeVisible({ timeout: 10_000 });
    await cardA.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Confirm Task A's messages are visible
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Create Task B (no agent session — simulates a PR watcher or new task)
    await apiClient.createTask(seedData.workspaceId, "Sessionless Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // 5. Go back to kanban
    await kanban.goto();
    await expect(kanban.board).toBeVisible({ timeout: 10_000 });

    // 6. Click on Task B from kanban
    const cardB = kanban.taskCardByTitle("Sessionless Task");
    await expect(cardB).toBeVisible({ timeout: 10_000 });
    await cardB.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // 7. Verify Task B's page shows the correct title
    await expect(testPage.getByText("Sessionless Task")).toBeVisible({ timeout: 10_000 });

    // 8. Task A's messages must NOT appear on Task B's page
    await expect(testPage.getByText("simple mock response", { exact: false })).not.toBeVisible({
      timeout: 5_000,
    });
  });
});
