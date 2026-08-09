import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

/**
 * Parked-session affordance (AC-58, AC-73a, AC-52).
 *
 * A session that settled to WAITING_FOR_INPUT while a background shell
 * workload is still live is "parked". The board card and task-list row must
 * render `data-testid="task-state-background-running"` rather than the plain
 * WAITING_FOR_INPUT question-mark icon.
 *
 * Backend plumbing is not exercised here — the projection is injected via the
 * __KANDEV_E2E_STORE__ bridge so the test stays deterministic and fast.
 */
type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => { kanban: { tasks: Array<Record<string, unknown>> } };
    setState: (
      updater: (state: { kanban: { tasks: Array<Record<string, unknown>> } }) => void,
    ) => void;
  };
};

async function injectParkedTask(page: import("@playwright/test").Page, taskId: string) {
  await page.evaluate((tid) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    store.setState((state) => {
      const task = state.kanban.tasks.find((t) => t.id === tid);
      if (!task) throw new Error(`Task ${tid} not found in kanban store`);
      task.state = "WAITING_FOR_INPUT";
      task.parkedOnBackgroundWork = true;
    });
  }, taskId);
}

test.describe("Parked-session affordance", () => {
  test("board card shows background-running icon when task is parked (AC-58)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Parked Board Card Test", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 10_000 });

    await injectParkedTask(testPage, task.id);

    // The board card must show the violet background-running spinner (AC-58),
    // not the question-mark icon.
    await expect(card.getByTestId("task-state-background-running")).toBeVisible({ timeout: 5_000 });
    await expect(card.getByTestId("task-state-waiting-for-input")).not.toBeVisible();
  });

  test("task list row shows background-running icon when task is parked (AC-73a)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Parked Task List Test", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // Navigate to task list
    await testPage.goto("/tasks");
    await testPage.waitForSelector('[data-testid="tasks-list-row-title"]', { timeout: 15_000 });

    await injectParkedTask(testPage, task.id);

    // Find the row and check for the parked icon (AC-73a).
    const row = testPage.locator(`[data-testid="tasks-list-row-title"]`, {
      hasText: "Parked Task List Test",
    });
    await expect(row).toBeVisible({ timeout: 5_000 });
    // The icon is a sibling of the title, both inside a flex container.
    const rowContainer = row.locator("..");
    await expect(rowContainer.getByTestId("task-state-background-running")).toBeVisible({
      timeout: 5_000,
    });
  });
});
