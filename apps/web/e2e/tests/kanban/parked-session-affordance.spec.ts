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
 * Backend plumbing is not exercised here — deterministically driving a real
 * detached background process through the launch recogniser and liveness
 * probe is out of scope for this fixture. Instead each surface is fed the
 * `parked_on_background_work` projection at the point it actually reads it:
 * the board reads `state.kanbanMulti.snapshots[workflowId].tasks`
 * (components/kanban-board.tsx), so the board test mutates that store slice
 * via the `__KANDEV_E2E_STORE__` bridge. The `/tasks` list has no live store
 * subscription at all — `app/tasks/tasks-page-client.tsx` holds its own
 * `useState` seeded from the `GET /api/v1/workspaces/:id/tasks` response — so
 * the list-row test intercepts that response instead.
 */
type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      kanbanMulti: { snapshots: Record<string, { tasks: Array<Record<string, unknown>> }> };
    };
    setState: (
      updater: (state: {
        kanbanMulti: { snapshots: Record<string, { tasks: Array<Record<string, unknown>> }> };
      }) => void,
    ) => void;
  };
};

async function injectParkedBoardTask(
  page: import("@playwright/test").Page,
  workflowId: string,
  taskId: string,
) {
  await page.evaluate(
    ({ workflowId, taskId }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      store.setState((state) => {
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (!snapshot) throw new Error(`No kanbanMulti snapshot for workflow ${workflowId}`);
        const task = snapshot.tasks.find((t) => t.id === taskId);
        if (!task) throw new Error(`Task ${taskId} not found in kanbanMulti snapshot`);
        task.state = "WAITING_FOR_INPUT";
        task.parkedOnBackgroundWork = true;
      });
    },
    { workflowId, taskId },
  );
}

/**
 * Intercept the tasks-list fetch and mark `taskId` as parked in the response
 * body. Must be registered before navigation: `app/tasks/tasks-page-client.tsx`
 * fetches this endpoint on mount with no Vite-side SSR to bypass, so Playwright
 * observes it on first load like any other client fetch.
 */
async function interceptParkedTaskListResponse(
  page: import("@playwright/test").Page,
  taskId: string,
) {
  await page.route("**/api/v1/workspaces/*/tasks*", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const body = (await response.json()) as {
      tasks: Array<Record<string, unknown>>;
      total: number;
    };
    await route.fulfill({
      response,
      json: {
        ...body,
        tasks: body.tasks.map((task) =>
          task.id === taskId ? { ...task, parked_on_background_work: true } : task,
        ),
      },
    });
  });
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

    await injectParkedBoardTask(testPage, seedData.workflowId, task.id);

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

    await interceptParkedTaskListResponse(testPage, task.id);

    await testPage.goto("/tasks");
    await testPage.waitForSelector('[data-testid="tasks-list-row-title"]', { timeout: 15_000 });

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
