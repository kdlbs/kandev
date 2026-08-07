/**
 * E2E for the sidebar queued prompt count badge.
 *
 * Every task and subtask row in the left task view shows a mail badge with
 * the number of prompts en-queued for that task. Covers:
 *   - Badge appears with the right count after prompts are queued
 *   - Badge disappears live (no reload) when the queue is cleared
 *   - No badge on a task with an empty queue
 *   - Subtask rows get the same badge treatment
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const stepOpts = (seedData: { workflowId: string; startStepId: string }) => ({
  workflow_id: seedData.workflowId,
  workflow_step_id: seedData.startStepId,
});

function queuedBadge(page: SessionPage, taskId: string) {
  return page.sidebar
    .locator(`[data-testid='sortable-task-block'][data-task-id='${taskId}']`)
    .getByTestId("sidebar-task-queued-count");
}

test.describe("Sidebar queued prompt count", () => {
  test("shows a mail badge with the queued count and clears live", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Queued Badge Task", {
      ...stepOpts(seedData),
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
    });
    for (let index = 0; index < 3; index++) {
      await apiClient.queueMessage(task.id, sessionId, `Queued prompt ${index + 1}`);
    }

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const badge = queuedBadge(session, task.id);
    await expect(badge).toBeVisible({ timeout: 10_000 });
    await expect(badge).toHaveText("3");
    await expect(badge.locator("svg")).toHaveCount(1);

    await testPage.screenshot({
      path: "e2e/test-results/screenshots/sidebar-queued-count-desktop.png",
      fullPage: false,
    });

    // Clearing the queue must remove the badge without a reload (live path
    // through the status-summary broadcast).
    await apiClient.clearQueue(sessionId);
    await expect(badge).not.toBeVisible({ timeout: 10_000 });
  });

  test("shows no badge for a task with an empty queue", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Empty Queue Task", {
      ...stepOpts(seedData),
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.sidebar.getByText("Empty Queue Task")).toBeVisible({ timeout: 10_000 });
    await expect(queuedBadge(session, task.id)).toHaveCount(0);
  });

  test("shows the badge on a subtask row", async ({ testPage, apiClient, seedData }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Badge Parent Task", {
      ...stepOpts(seedData),
      repository_ids: [seedData.repositoryId],
    });
    const child = await apiClient.createTask(seedData.workspaceId, "Badge Child Task", {
      ...stepOpts(seedData),
      parent_id: parent.id,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(child.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.queueMessage(child.id, sessionId, "Subtask queued prompt");

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const badge = queuedBadge(session, child.id);
    await expect(badge).toBeVisible({ timeout: 10_000 });
    await expect(badge).toHaveText("1");
  });
});
