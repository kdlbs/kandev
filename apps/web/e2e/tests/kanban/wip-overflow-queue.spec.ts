import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { useRegularMode } from "../../helpers/regular-mode";

useRegularMode();

test("dragging into a feeder wakes an open pull target without reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Feeder Move Workflow");
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "C", 0, {
    is_start_step: true,
  });
  const feederStep = await apiClient.createWorkflowStep(workflow.id, "A", 1);
  const pullStep = await apiClient.createWorkflowStep(workflow.id, "B", 2);
  await apiClient.updateWorkflowStep(pullStep.id, {
    wip_limit: 1,
    pull_from_step_id: feederStep.id,
  });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });
  const task = await apiClient.createTask(seedData.workspaceId, "Wake feeder task", {
    workflow_id: workflow.id,
    workflow_step_id: sourceStep.id,
  });

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  const pullColumn = kanban.columnByStepId(pullStep.id);
  await expect(pullColumn).toContainText("0/1");

  const sourceCard = kanban.taskCard(task.id);
  const feederColumn = kanban.columnByStepId(feederStep.id);
  const sourceBox = await sourceCard.boundingBox();
  const feederBox = await feederColumn.boundingBox();
  expect(sourceBox).not.toBeNull();
  expect(feederBox).not.toBeNull();

  await testPage.mouse.move(
    sourceBox!.x + sourceBox!.width / 2,
    sourceBox!.y + sourceBox!.height / 2,
  );
  await testPage.mouse.down();
  await testPage.mouse.move(
    sourceBox!.x + sourceBox!.width / 2 + 20,
    sourceBox!.y + sourceBox!.height / 2,
  );
  await testPage.mouse.move(
    feederBox!.x + feederBox!.width / 2,
    feederBox!.y + feederBox!.height / 2,
    {
      steps: 12,
    },
  );
  await testPage.mouse.up();

  await expect(pullColumn).toContainText("1/1");
  await expect(kanban.taskCardInColumn("Wake feeder task", pullStep.id)).toBeVisible();
  await expect(kanban.taskCardInColumn("Wake feeder task", sourceStep.id)).not.toBeVisible();
});

test("shows same-step WIP overflow and promotes the next queued task", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Visible Queue Workflow");
  const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 0, {
    is_start_step: true,
  });
  const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 1);
  await apiClient.updateWorkflowStep(reviewStep.id, { wip_limit: 2 });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });

  const tasks = [];
  for (let index = 1; index <= 7; index += 1) {
    tasks.push(
      await apiClient.createTask(seedData.workspaceId, `Queue Review ${index}`, {
        workflow_id: workflow.id,
        workflow_step_id: reviewStep.id,
      }),
    );
  }

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  const reviewColumn = kanban.columnByStepId(reviewStep.id);
  await expect(reviewColumn).toContainText("2/2");
  await expect(reviewColumn.getByTestId("task-card-title")).toHaveCount(7);
  await expect(reviewColumn.getByTestId("kanban-queued-section")).toBeVisible();
  await expect(reviewColumn).toContainText("Queued for Review");
  await prCapture.screenshot("desktop-visible-queue", {
    caption:
      "Desktop Kanban showing seven visible tasks with two admitted cards and queued overflow.",
  });

  await apiClient.moveTask(tasks[0].id, workflow.id, doneStep.id);
  await expect(reviewColumn).toContainText("2/2");
  await expect(kanban.taskCardByTitle("Queue Review 3")).toBeVisible();
  await expect(kanban.taskCardByTitle("Queue Review 3")).not.toContainText("Queued for Review");
});

test("moves into a full step, shows sidebar queue position, and promotes through the UI", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Move Queue Workflow");
  const backlogStep = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0, {
    is_start_step: true,
  });
  const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 1);
  const doneStep = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
  await apiClient.updateWorkflowStep(reviewStep.id, { wip_limit: 2 });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });

  const admittedOne = await apiClient.createTask(seedData.workspaceId, "Admitted Review One", {
    workflow_id: workflow.id,
    workflow_step_id: reviewStep.id,
  });
  await apiClient.createTask(seedData.workspaceId, "Admitted Review Two", {
    workflow_id: workflow.id,
    workflow_step_id: reviewStep.id,
  });
  const source = await apiClient.createTask(seedData.workspaceId, "Moved Review Queue", {
    workflow_id: workflow.id,
    workflow_step_id: backlogStep.id,
  });

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  const reviewColumn = kanban.columnByStepId(reviewStep.id);
  await expect(reviewColumn).toContainText("2/2");

  await kanban.moveTaskWithinWorkflow(source.id, reviewStep.id);
  const queuedCard = kanban.taskCardInColumn("Moved Review Queue", reviewStep.id);
  await expect(queuedCard).toBeVisible({ timeout: 10_000 });
  await expect(queuedCard).toContainText("Queued for Review");
  await expect(reviewColumn.getByTestId("kanban-queued-section")).toBeVisible();

  const sidebarRow = testPage
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Moved Review Queue" });
  await expect(sidebarRow).toBeVisible({ timeout: 10_000 });
  const queueStatus = sidebarRow.getByTestId("sidebar-task-wip-queue");
  await expect(queueStatus).toBeVisible();
  await expect(queueStatus.locator("svg")).toBeVisible();
  await expect(queueStatus).toHaveAttribute("aria-label", "Position 1 of 1 in Review queue");
  await queueStatus.hover();
  await expect(testPage.getByRole("tooltip")).toHaveText("Position 1 of 1 in Review queue");
  await testPage.keyboard.press("Escape");

  await kanban.moveTaskWithinWorkflow(admittedOne.id, doneStep.id);
  await expect(queuedCard).not.toContainText("Queued for Review", { timeout: 15_000 });
  await expect(reviewColumn.getByTestId("kanban-queued-section")).toHaveCount(0);
  await expect(sidebarRow.getByTestId("sidebar-task-wip-queue")).toHaveCount(0);
});
