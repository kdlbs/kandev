import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { KanbanPage } from "../../pages/kanban-page";

// @covers AC-TASKS-COMPLETION-005.5
test("desktop bulk move shows every refusal and leaves tasks in their source step", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const targetStep = seedData.steps.find((step) => step.id !== seedData.startStepId);
  if (!targetStep) {
    test.skip(true, "Workflow has no second step for a bulk move");
    return;
  }
  const tasks = await Promise.all(
    ["Bulk refusal 1", "Bulk refusal 2", "Bulk refusal 3"].map((title) =>
      apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ),
  );
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  for (const task of tasks) await kanban.selectTask(task.id);

  await testPage.route("**/api/v1/tasks/bulk-move", async (route) => {
    await route.fulfill({
      status: 409,
      contentType: "application/json",
      body: JSON.stringify({
        code: "bulk_move_refused",
        moved_task_ids: [],
        refusals: [
          {
            task_id: tasks[1].id,
            task_title: "Bulk refusal 2",
            code: "task_approvals_pending",
            message: "task approvals pending: CEO",
            pending_approvers: [{ agent_profile_id: "agent-ceo", name: "CEO" }],
          },
        ],
      }),
    });
  });

  await kanban.bulkMoveButton.click();
  const refusedMove = waitForHttp(testPage, "POST", /\/api\/v1\/tasks\/bulk-move$/);
  await kanban.bulkMoveStepOption(targetStep.id).click();
  expect((await refusedMove).status()).toBe(409);

  const toast = testPage.getByTestId("toast-message").last();
  await expect(toast).toContainText("No tasks were moved");
  await expect(toast).toContainText("Bulk refusal 2: Approval pending (CEO)");
  for (const task of tasks) {
    await expect(kanban.taskCardInColumn(task.title, seedData.startStepId)).toBeVisible();
    const current = await apiClient.getTask(task.id);
    expect(current.workflow_step_id).toBe(seedData.startStepId);
  }
  await expect(kanban.taskCardInColumn("Bulk refusal 1", targetStep.id)).toHaveCount(0);
});
