import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

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

    // 1. Create Task A with a completed session and message history.
    const taskA = await apiClient.createTask(seedData.workspaceId, "Task With Messages", {
      description: "Task with seeded messages",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const sessionId = randomUUID();
    const now = Date.now();
    await apiClient.seedTaskSession(taskA.id, {
      sessionId,
      state: "COMPLETED",
      startedAt: new Date(now).toISOString(),
      completedAt: new Date(now + 1_000).toISOString(),
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "simple mock response from stale-session source",
    });
    await apiClient.setPrimarySession(sessionId);

    // 2. Create Task B with no session — simulates a PR watcher or new task.
    const taskB = await apiClient.createTask(seedData.workspaceId, "Sessionless Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // 3. Open Task A in the kanban preview and verify its message is visible.
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCard(taskA.id);
    await expect(cardA).toBeVisible({ timeout: 15_000 });
    await cardA.click();

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible({ timeout: 10_000 });
    await expect(previewPanel.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // 4. Close the floating preview, then click Task B from kanban. The
    // floating layout uses a backdrop that intentionally blocks board clicks.
    await testPage.getByLabel("Close preview").click();
    await expect(previewPanel).not.toBeVisible({ timeout: 5_000 });

    const cardB = kanban.taskCard(taskB.id);
    await expect(cardB).toBeVisible({ timeout: 10_000 });
    await cardB.click();

    // 5. Verify Task B's preview shows the correct title.
    await expect(previewPanel.getByText("Sessionless Task", { exact: true })).toBeVisible({
      timeout: 10_000,
    });

    // 6. Task A's messages must NOT appear on Task B's preview.
    await expect(previewPanel.getByText("simple mock response", { exact: false })).not.toBeVisible({
      timeout: 5_000,
    });
  });
});
