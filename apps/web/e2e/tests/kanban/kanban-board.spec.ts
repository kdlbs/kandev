import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Kanban board", () => {
  test("displays a seeded task card", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "E2E Kanban Test Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);

    await kanban.goto();

    const card = kanban.taskCardByTitle("E2E Kanban Test Task");
    await expect(card).toBeVisible();
    await expect(kanban.taskCard(task.id)).toBeVisible();
  });

  test("shows create task button", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.createTaskButton.first()).toBeVisible();
  });

  test("opens task preview from kanban card", async ({ testPage, apiClient, seedData }) => {
    // Enable preview-on-click so clicking a card opens the preview panel
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Detail View Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);

    await kanban.goto();

    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible();
    await card.click();

    // After clicking a card with preview enabled, URL gets ?taskId=... param.
    // Use toHaveURL (polling assertion) since replaceState doesn't fire navigation events.
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10000 });
  });

  // The desktop kanban header centers the search input absolutely. When the
  // preview panel opens, the kanban area shrinks (`kanbanWidth = container -
  // previewWidth`); the header narrows along with it. Below ~1100px there is
  // no longer room between the left/right action groups for the centered
  // search, so the header hides it (see useIsHeaderNarrow in kanban-header).
  test("hides header search when preview panel narrows the kanban area", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Header Squeeze Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // With no preview open, the kanban area uses the full viewport width
    // (Desktop Chrome = 1280px) so the centered search is visible.
    const search = testPage.getByTestId("kanban-header-search");
    await expect(search).toBeVisible();

    // Open the preview — kanban width drops below 1100px (1280 - 500 default
    // preview width = 780px) and the search must hide.
    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible();
    await card.click();
    await expect(testPage.getByTestId("task-preview-panel")).toBeVisible({ timeout: 10_000 });
    await expect(search).toBeHidden({ timeout: 5_000 });

    // Closing the preview restores the full kanban width and brings the
    // search back.
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByTestId("task-preview-panel")).toBeHidden({ timeout: 5_000 });
    await expect(search).toBeVisible({ timeout: 5_000 });
  });
});
