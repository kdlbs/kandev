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
    await expect(kanban.bulkDeleteConfirm).toBeEnabled();
    await expect(testPage.getByTestId("delete-discard-worktree-checkbox")).toHaveCount(0);
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
    await kanban.bulkArchiveConfirm.click();

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
    await kanban.bulkArchiveConfirm.click();
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
    await expect(kanban.bulkDeleteConfirm).toBeEnabled();
    await expect(testPage.getByTestId("delete-discard-worktree-checkbox")).toHaveCount(0);
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

  // REQ-TASKS-KANBAN-TASK-REORDERING-001.29 (F34): bulk move must land the
  // selected tasks in a deterministic order derived from their source
  // position, not from selection click order or network arrival order. The
  // kanban multi-select bulk-move path previously fanned out one concurrent
  // HTTP call per task with a caller-assigned `position: i`, so the final
  // order depended on which request the server happened to receive first — a
  // race that would make an order assertion here flaky by construction if the
  // underlying fix regressed. Selecting the tasks in reverse order (and
  // asserting against their creation order, not click order) pins that this
  // is not an accident of how the test happens to click.
  test("bulk move lands tasks in deterministic source order (AC.29)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetStep = seedData.steps.find((s) => s.id !== seedData.startStepId);
    if (!targetStep) {
      test.skip(true, "Workflow has only one step — cannot test move");
      return;
    }

    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    // Created sequentially so arrival position (max+1 per create) fixes a
    // known source order: first=0, second=1, third=2, fourth=3.
    const first = await apiClient.createTask(seedData.workspaceId, "MS Order 1", placement);
    const second = await apiClient.createTask(seedData.workspaceId, "MS Order 2", placement);
    const third = await apiClient.createTask(seedData.workspaceId, "MS Order 3", placement);
    const fourth = await apiClient.createTask(seedData.workspaceId, "MS Order 4", placement);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Select in reverse of source order — the resulting order must not
    // reflect this click order.
    await kanban.selectTask(fourth.id);
    await kanban.selectTask(third.id);
    await kanban.selectTask(second.id);
    await kanban.selectTask(first.id);
    await expect(kanban.multiSelectToolbar).toContainText("4 selected");

    await kanban.bulkMoveButton.click();
    const targetOption = kanban.bulkMoveStepOption(targetStep.id);
    await expect(targetOption).toBeVisible();
    await targetOption.click();

    await expect(kanban.taskCardInColumn("MS Order 4", targetStep.id)).toBeVisible({
      timeout: 10000,
    });

    // Positions in the target step are consecutive and match source order,
    // not selection order.
    await expect
      .poll(async () => {
        const [t1, t2, t3, t4] = await Promise.all([
          apiClient.getTask(first.id),
          apiClient.getTask(second.id),
          apiClient.getTask(third.id),
          apiClient.getTask(fourth.id),
        ]);
        return { t1: t1.position, t2: t2.position, t3: t3.position, t4: t4.position };
      })
      .toEqual({ t1: 0, t2: 1, t3: 2, t4: 3 });

    // The column itself renders them in that same order, top to bottom.
    await expect
      .poll(() =>
        kanban
          .columnByStepId(targetStep.id)
          .locator('[data-testid="task-card-title"]')
          .allTextContents(),
      )
      .toEqual(["MS Order 1", "MS Order 2", "MS Order 3", "MS Order 4"]);
  });
});
