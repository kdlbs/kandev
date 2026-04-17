import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Multi-select bulk actions", () => {
  test("toolbar appears when a task is selected via checkbox", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "MS Select Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.multiSelectToolbar).not.toBeVisible();

    await kanban.selectTask(task.id);

    await expect(kanban.multiSelectToolbar).toBeVisible();
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");
  });

  test("selecting multiple tasks shows correct count", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "MS Count Task 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "MS Count Task 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(t1.id);
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");

    await kanban.selectTask(t2.id);
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");
  });

  test("clear selection button hides the toolbar", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "MS Clear Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(task.id);
    await expect(kanban.multiSelectToolbar).toBeVisible();

    await kanban.bulkClearButton.click();
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
  });

  test("bulk delete removes tasks after confirmation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "MS Delete 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "MS Delete 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(t1.id);
    await kanban.selectTask(t2.id);
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    await kanban.bulkDeleteButton.click();
    await expect(kanban.bulkDeleteConfirm).toBeVisible();
    await kanban.bulkDeleteConfirm.click();

    await expect(kanban.taskCard(t1.id)).not.toBeVisible({ timeout: 10000 });
    await expect(kanban.taskCard(t2.id)).not.toBeVisible();
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
  });

  test("bulk archive removes tasks from board", async ({ testPage, apiClient, seedData }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "MS Archive 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "MS Archive 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(t1.id);
    await kanban.selectTask(t2.id);

    await kanban.bulkArchiveButton.click();

    await expect(kanban.taskCard(t1.id)).not.toBeVisible({ timeout: 10000 });
    await expect(kanban.taskCard(t2.id)).not.toBeVisible();
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
  });

  test("multi-select mode is disabled after bulk archive", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "MS Mode Reset 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "MS Mode Reset 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(t1.id);
    await kanban.selectTask(t2.id);

    // Multi-select mode should be active
    await expect(testPage.locator('[data-multi-select-active="true"]').first()).toBeVisible();

    await kanban.bulkArchiveButton.click();
    await expect(kanban.taskCard(t1.id)).not.toBeVisible({ timeout: 10000 });
    await expect(kanban.taskCard(t2.id)).not.toBeVisible();

    // Multi-select mode itself should be disabled, not just the toolbar
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
    await expect(testPage.locator('[data-multi-select-active="true"]')).not.toBeVisible();
  });

  test("multi-select mode is disabled after bulk delete", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "MS Mode Reset Del 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "MS Mode Reset Del 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(t1.id);
    await kanban.selectTask(t2.id);

    await expect(testPage.locator('[data-multi-select-active="true"]').first()).toBeVisible();

    await kanban.bulkDeleteButton.click();
    await expect(kanban.bulkDeleteConfirm).toBeVisible();
    await kanban.bulkDeleteConfirm.click();

    await expect(kanban.taskCard(t1.id)).not.toBeVisible({ timeout: 10000 });
    await expect(kanban.taskCard(t2.id)).not.toBeVisible();

    // Multi-select mode itself should be disabled, not just the toolbar
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
    await expect(testPage.locator('[data-multi-select-active="true"]')).not.toBeVisible();
  });

  test("bulk move moves tasks to target step", async ({ testPage, apiClient, seedData }) => {
    // Pick a step that is not the start step as the move target
    const targetStep = seedData.steps.find((s) => s.id !== seedData.startStepId);
    if (!targetStep) {
      test.skip(true, "Workflow has only one step — cannot test move");
      return;
    }

    const task = await apiClient.createTask(seedData.workspaceId, "MS Move Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.selectTask(task.id);
    await kanban.bulkMoveButton.click();

    const targetOption = kanban.bulkMoveStepOption(targetStep.id);
    await expect(targetOption).toBeVisible();
    await targetOption.click();
    // Toolbar persists after move so follow-up actions can be chained
    await expect(kanban.multiSelectToolbar).toBeVisible({ timeout: 10000 });
    // Task should be visible in the target column
    await expect(kanban.taskCardInColumn("MS Move Task", targetStep.id)).toBeVisible();
  });
});
