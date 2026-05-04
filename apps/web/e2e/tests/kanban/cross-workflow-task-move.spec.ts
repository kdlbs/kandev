import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Cross-workflow task move from home", () => {
  test("keeps same-workflow Move to as the short path", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetStep = seedData.steps.find((step) => step.id !== seedData.startStepId);
    if (!targetStep) {
      test.skip(true, "seed workflow needs at least two steps");
      return;
    }

    const task = await apiClient.createTask(seedData.workspaceId, "Same Workflow Context Move", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.openTaskContextMenu(task.id);
    await expect(kanban.contextMoveTo()).toBeVisible();
    await expect(kanban.contextSendToWorkflow()).not.toBeVisible();
    await kanban.contextMoveTo().hover();
    await kanban.contextStep(targetStep.id).click();

    await expect(kanban.taskCardInColumn("Same Workflow Context Move", targetStep.id)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("sends one task to another workflow without changing the current view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Target Workflow Context Send",
    );
    const targetStep = await apiClient.createWorkflowStep(targetWorkflow.id, "Incoming", 0);
    const task = await apiClient.createTask(seedData.workspaceId, "Cross Workflow Context Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const beforeUrl = testPage.url();

    await kanban.sendTaskToWorkflow(task.id, targetWorkflow.id, targetStep.id);

    await expect(kanban.taskCard(task.id)).not.toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(/Moved task to/i)).toBeVisible({ timeout: 10_000 });
    expect(testPage.url()).toBe(beforeUrl);

    await testPage.goto(`/?workflowId=${targetWorkflow.id}`);
    await expect(kanban.board).toBeVisible();
    await expect(kanban.taskCardInColumn("Cross Workflow Context Task", targetStep.id)).toBeVisible(
      { timeout: 10_000 },
    );

    await testPage.reload();
    await expect(kanban.taskCardInColumn("Cross Workflow Context Task", targetStep.id)).toBeVisible(
      { timeout: 10_000 },
    );
  });

  test("keeps right-click and three-dot menus aligned for workflow moves", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sameWorkflowStep = seedData.steps.find((step) => step.id !== seedData.startStepId);
    if (!sameWorkflowStep) {
      test.skip(true, "seed workflow needs at least two steps");
      return;
    }

    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Target Workflow Actions Send",
    );
    const targetStep = await apiClient.createWorkflowStep(targetWorkflow.id, "Incoming", 0);
    const task = await apiClient.createTask(seedData.workspaceId, "Cross Workflow Actions Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const expectedLabels = ["Edit", "Move to", "Send to workflow", "Archive", "Delete"];
    await kanban.openTaskContextMenu(task.id);
    for (const label of expectedLabels) {
      await expect(testPage.getByRole("menuitem", { name: label })).toBeVisible();
    }
    await testPage.keyboard.press("Escape");

    await kanban.openTaskActionsMenu(task.id);
    for (const label of expectedLabels) {
      await expect(testPage.getByRole("menuitem", { name: label })).toBeVisible();
    }
    await kanban.contextSendToWorkflow().hover();
    await kanban.contextWorkflow(targetWorkflow.id).hover();
    await kanban.contextStep(targetStep.id).click();

    await expect(kanban.taskCard(task.id)).not.toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(/Moved task to/i)).toBeVisible({ timeout: 10_000 });

    await testPage.goto(`/?workflowId=${targetWorkflow.id}`);
    await expect(kanban.taskCardInColumn("Cross Workflow Actions Task", targetStep.id)).toBeVisible(
      { timeout: 10_000 },
    );
  });

  test("shows no-step workflows as disabled send targets", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const emptyWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Empty Workflow Context Send",
    );
    const task = await apiClient.createTask(seedData.workspaceId, "No Step Target Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.openTaskContextMenu(task.id);
    await kanban.contextSendToWorkflow().hover();

    await expect(kanban.contextWorkflow(emptyWorkflow.id)).toBeVisible();
    await expect(kanban.contextWorkflow(emptyWorkflow.id)).toBeDisabled();
    await expect(testPage.getByTestId("task-context-disabled-reason")).toContainText("No steps");
  });

  test("marks auto-start target steps before moving", async ({ testPage, apiClient, seedData }) => {
    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Auto Start Context Target",
    );
    const targetStep = await apiClient.createWorkflowStep(targetWorkflow.id, "Run agent", 0);
    await apiClient.updateWorkflowStep(targetStep.id, {
      prompt: 'e2e:message("context move auto-started")',
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    const task = await apiClient.createTask(seedData.workspaceId, "Auto Start Marker Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.openTaskContextMenu(task.id);
    await kanban.contextSendToWorkflow().hover();
    await kanban.contextWorkflow(targetWorkflow.id).hover();
    await expect(kanban.contextAutoStartStep(targetStep.id)).toBeVisible();
  });
});
