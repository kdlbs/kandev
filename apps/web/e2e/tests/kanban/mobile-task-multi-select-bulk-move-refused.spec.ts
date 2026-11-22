import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { KanbanPage } from "../../pages/kanban-page";

// @covers AC-TASKS-COMPLETION-005.5
test("phone bulk move shows the refusal and keeps every task in its source step", async ({
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
    ["Phone bulk refusal 1", "Phone bulk refusal 2", "Phone bulk refusal 3"].map((title) =>
      apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      }),
    ),
  );
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await testPage.getByTestId("mobile-select-tasks").tap();
  for (const task of tasks) await kanban.taskCard(task.id).tap();

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
            task_title: "Phone bulk refusal 2",
            code: "task_approvals_pending",
            message: "task approvals pending: CEO",
            pending_approvers: [{ agent_profile_id: "agent-ceo", name: "CEO" }],
          },
        ],
      }),
    });
  });

  await testPage.getByTestId("bulk-actions-button").tap();
  const actions = testPage.getByRole("dialog", { name: "Actions" });
  await expect(actions).toBeVisible();
  await actions.getByTestId("bulk-move-button").tap();
  const refusedMove = waitForHttp(testPage, "POST", /\/api\/v1\/tasks\/bulk-move$/);
  await kanban.bulkMoveStepOption(targetStep.id).tap();
  expect((await refusedMove).status()).toBe(409);

  const toast = testPage.getByTestId("toast-message").last();
  await expect(toast).toContainText("No tasks were moved");
  await expect(toast).toContainText("Phone bulk refusal 2: Approval pending (CEO)");
  for (const task of tasks) {
    const current = await apiClient.getTask(task.id);
    expect(current.workflow_step_id).toBe(seedData.startStepId);
    await expect(kanban.taskCard(task.id)).toBeVisible();
  }
});
