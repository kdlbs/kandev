import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Pipeline view", () => {
  test.beforeEach(async ({ testPage }) => {
    // The view toggle and multi-select toggle are only rendered on desktop layouts.
    await testPage.setViewportSize({ width: 1280, height: 800 });
  });

  test("shows the linked repository name beneath the task title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Repo Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const repoName = kanban.pipelineTaskRepoName(task.id);
    await expect(repoName).toBeVisible();
    // seedData seeds the repository with the default name from createRepository().
    await expect(repoName).toHaveText("E2E Repo");
  });

  test("does not show a repo name when the task has no linked repository", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline No Repo Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await expect(kanban.pipelineTask(task.id)).toBeVisible();
    await expect(kanban.pipelineTaskRepoName(task.id)).toHaveCount(0);
  });

  test("multi-select checkbox appears and toolbar reflects the selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Pipeline MS 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Pipeline MS 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    // Checkbox is hidden until multi-select mode is enabled.
    await expect(kanban.taskSelectCheckbox(t1.id)).toHaveCount(0);
    await expect(kanban.multiSelectToolbar).not.toBeVisible();

    await kanban.selectPipelineTask(t1.id);
    await expect(kanban.taskSelectCheckbox(t1.id)).toBeVisible();
    await expect(kanban.multiSelectToolbar).toBeVisible();
    await expect(kanban.multiSelectToolbar).toContainText("1 selected");

    // Once multi-select mode is on, every pipeline task exposes a checkbox.
    await kanban.taskSelectCheckbox(t2.id).click();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");
  });

  test("bulk delete from pipeline view removes the selected tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [t1, t2] = await Promise.all([
      apiClient.createTask(seedData.workspaceId, "Pipeline Delete 1", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
      apiClient.createTask(seedData.workspaceId, "Pipeline Delete 2", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ]);
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban.selectPipelineTask(t1.id);
    await kanban.taskSelectCheckbox(t2.id).click();
    await expect(kanban.multiSelectToolbar).toContainText("2 selected");

    await kanban.bulkDeleteButton.click();
    await expect(kanban.bulkDeleteConfirm).toBeVisible();
    await kanban.bulkDeleteConfirm.click();

    await expect(kanban.pipelineTask(t1.id)).toHaveCount(0, { timeout: 10000 });
    await expect(kanban.pipelineTask(t2.id)).toHaveCount(0);
    await expect(kanban.multiSelectToolbar).not.toBeVisible();
  });

  test("title click opens the preview panel when 'Open preview on click' is on", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Preview Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    await kanban
      .pipelineTask(task.id)
      .getByRole("button", { name: "Pipeline Preview Task" })
      .click();

    // Preview panel opens in place (URL carries taskId=), not a full-page navigation to /t/:id.
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10_000 });
    await expect(testPage.getByTestId("task-preview-panel")).toBeVisible();
  });

  test("the 3-dots actions trigger stays reachable and reachable without scrolling on a long pipeline", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1600, height: 900 });

    // Grow the workflow to 9 steps so the row overflows a real desktop viewport,
    // matching the geometry measured for defect 2 (130px pill + 25px connector each).
    const { steps: existingSteps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    const targetStepCount = 9;
    for (let position = existingSteps.length; position < targetStepCount; position++) {
      await apiClient.createWorkflowStep(seedData.workflowId, `Extra Step ${position}`, position);
    }

    const task = await apiClient.createTask(seedData.workspaceId, "Pipeline Overflow Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.switchToPipelineView();

    const trigger = kanban.pipelineTaskActionsTrigger(task.id);
    await expect(trigger).toBeVisible();

    const box = await trigger.boundingBox();
    const viewportSize = testPage.viewportSize();
    expect(box).not.toBeNull();
    expect(viewportSize).not.toBeNull();
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewportSize!.width);

    // Reachable without scrolling: click opens the dropdown menu directly.
    await trigger.click();
    await expect(testPage.getByRole("menuitem", { name: "Delete task" })).toBeVisible();
  });
});
