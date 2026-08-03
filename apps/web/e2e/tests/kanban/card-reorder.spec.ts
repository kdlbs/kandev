import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { useRegularMode } from "../../helpers/regular-mode";
import { dragKanbanCard } from "../../helpers/kanban-dnd";

useRegularMode();

async function titlesInColumn(kanban: KanbanPage, stepId: string): Promise<string[]> {
  const titles = kanban.columnByStepId(stepId).locator('[data-testid="task-card-title"]');
  return titles.allTextContents();
}

test("reorders admitted cards within a Kanban step and persists after reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Card Reorder Workflow");
  const step = await apiClient.createWorkflowStep(workflow.id, "Todo", 0, {
    is_start_step: true,
  });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });

  const first = await apiClient.createTask(seedData.workspaceId, "Reorder First", {
    workflow_id: workflow.id,
    workflow_step_id: step.id,
  });
  const second = await apiClient.createTask(seedData.workspaceId, "Reorder Second", {
    workflow_id: workflow.id,
    workflow_step_id: step.id,
  });
  await apiClient.moveTask(first.id, workflow.id, step.id, 0);
  await apiClient.moveTask(second.id, workflow.id, step.id, 1);

  const kanban = new KanbanPage(testPage);
  await kanban.goto();

  await expect
    .poll(() => titlesInColumn(kanban, step.id))
    .toEqual(["Reorder First", "Reorder Second"]);

  await dragKanbanCard(testPage, kanban.taskCard(first.id), kanban.taskCard(second.id), {
    place: "below",
  });

  await expect
    .poll(() => titlesInColumn(kanban, step.id))
    .toEqual(["Reorder Second", "Reorder First"]);

  await testPage.reload();
  await kanban.board.waitFor({ state: "visible" });
  await expect
    .poll(() => titlesInColumn(kanban, step.id))
    .toEqual(["Reorder Second", "Reorder First"]);
});

test("keeps queued overflow after admitted cards and still allows cross-step move", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Reorder WIP Workflow");
  const review = await apiClient.createWorkflowStep(workflow.id, "Review", 0, {
    is_start_step: true,
  });
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 1);
  await apiClient.updateWorkflowStep(review.id, { wip_limit: 1 });
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: workflow.id,
  });

  const admitted = await apiClient.createTask(seedData.workspaceId, "Admitted Card", {
    workflow_id: workflow.id,
    workflow_step_id: review.id,
  });
  await apiClient.createTask(seedData.workspaceId, "Queued Card", {
    workflow_id: workflow.id,
    workflow_step_id: review.id,
  });

  const kanban = new KanbanPage(testPage);
  await kanban.goto();

  const reviewColumn = kanban.columnByStepId(review.id);
  await expect(reviewColumn).toContainText("1/1");
  await expect(reviewColumn).toContainText("Queued for Review");

  const titles = await titlesInColumn(kanban, review.id);
  expect(titles[0]).toBe("Admitted Card");
  expect(titles.at(-1)).toBe("Queued Card");

  await dragKanbanCard(testPage, kanban.taskCard(admitted.id), kanban.columnByStepId(done.id));
  await expect(kanban.taskCardInColumn("Admitted Card", done.id)).toBeVisible({ timeout: 10_000 });
});
