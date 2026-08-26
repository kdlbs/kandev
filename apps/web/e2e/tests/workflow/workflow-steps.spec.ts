import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Workflow steps", () => {
  test("task appears in correct column after API move", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Seed a task (lands in the start step by default)
    const task = await apiClient.createTask(seedData.workspaceId, "Workflow Move Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // Find a non-start step to move to
    const targetStep = seedData.steps.find((s) => !s.is_start_step);
    if (!targetStep) {
      test.skip(true, "No non-start step available to test move");
      return;
    }

    // Move the task via API
    await apiClient.moveTask(task.id, seedData.workflowId, targetStep.id);

    // Navigate to kanban and verify the card is visible
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Workflow Move Task");
    await expect(card).toBeVisible();
  });

  test("multiple tasks can be seeded into different steps", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const startStep = seedData.steps.find((s) => s.is_start_step) ?? seedData.steps[0];
    const otherStep = seedData.steps.find((s) => s.id !== startStep.id);
    if (!otherStep) {
      test.skip(true, "Need at least 2 workflow steps");
      return;
    }

    // Create tasks and move one
    await apiClient.createTask(seedData.workspaceId, "Step A Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const taskB = await apiClient.createTask(seedData.workspaceId, "Step B Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.moveTask(taskB.id, seedData.workflowId, otherStep.id);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle("Step A Task")).toBeVisible();
    await expect(kanban.taskCardByTitle("Step B Task")).toBeVisible();
  });

  test("explains a rejected move on the task page", async ({ testPage, apiClient, seedData }) => {
    const targetStep = seedData.steps.find((s) => !s.is_start_step);
    if (!targetStep) {
      test.skip(true, "No non-start step available to test move feedback");
      return;
    }

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Workflow Move Feedback Task",
      seedData.agentProfileId,
      {
        description: "e2e:delay(5000)",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.route(`**/api/v1/tasks/${task.id}/move`, async (route) => {
      await route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({
          error: "task has an active session (RUNNING)",
          code: "task_move_active_session",
        }),
      });
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.stepper).toBeVisible();

    await session.stepper.getByTestId(`workflow-step-${targetStep.name}`).hover();
    await testPage.getByRole("button", { name: "Move here" }).click();

    const moveError = testPage.getByTestId("task-move-error-banner");
    await expect(moveError).toBeVisible();
    await expect(moveError).toContainText("Stop the active session before moving the task.");
    await expect(
      session.stepperStep(seedData.steps.find((s) => s.is_start_step)?.name ?? ""),
    ).toBeVisible();
  });

  test("explains a rejected move from the task sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetStep = seedData.steps.find((s) => !s.is_start_step);
    if (!targetStep) {
      test.skip(true, "No non-start step available to test move feedback");
      return;
    }

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Workflow Sidebar Move Feedback Task",
      seedData.agentProfileId,
      {
        description: "e2e:delay(5000)",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.route(`**/api/v1/tasks/${task.id}/move`, async (route) => {
      await route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify({
          error: "task has an active session (RUNNING)",
          code: "task_move_active_session",
        }),
      });
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.openSidebarTaskContextMenu("Workflow Sidebar Move Feedback Task");
    await testPage.getByTestId("task-context-move-to").hover();
    await testPage.getByTestId(`task-context-step-${targetStep.id}`).click();

    const moveError = testPage.getByTestId("task-move-error-banner");
    await expect(moveError).toBeVisible();
    await expect(moveError).toContainText("Stop the active session before moving the task.");
  });

  test("an unstarted feeder task fills available WIP capacity", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Feeder pull workflow");
    const waiting = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0, {
      is_start_step: true,
    });
    const review = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    await apiClient.updateWorkflowStep(review.id, {
      wip_limit: 2,
      pull_from_step_id: waiting.id,
    });

    await apiClient.createTask(seedData.workspaceId, "Unstarted feeder task", {
      workflow_id: workflow.id,
      workflow_step_id: waiting.id,
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      repository_ids: [],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardInColumn("Unstarted feeder task", review.id)).toBeVisible();
    await expect(kanban.taskCardInColumn("Unstarted feeder task", waiting.id)).toHaveCount(0);
  });
});
