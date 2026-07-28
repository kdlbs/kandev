import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { useRegularMode } from "../../helpers/regular-mode";

useRegularMode();

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
  const reviewColumn = kanban.columnByStepId(reviewStep.id).first();
  await expect(reviewColumn).toContainText("2/2");
  await expect(reviewColumn.getByTestId("task-card-title")).toHaveCount(7);
  await expect(reviewColumn).toContainText("Queued for Review");
  await prCapture.screenshot("desktop-visible-queue", {
    caption: "Desktop Kanban showing seven visible tasks with two admitted and queued overflow.",
  });

  await apiClient.moveTask(tasks[0].id, workflow.id, doneStep.id);
  await expect(reviewColumn).toContainText("2/2");
  await expect(kanban.taskCardByTitle("Queue Review 3")).toBeVisible();
  await expect(kanban.taskCardByTitle("Queue Review 3")).not.toContainText("Queued for Review");
});
