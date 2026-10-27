import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// @covers AC-TASKS-BOARD-BULK-MOVE-SCOPE-001.1
test("phone bulk move uses the focused workflow", async ({ testPage, apiClient, seedData }) => {
  const target = await apiClient.createWorkflowStep(seedData.workflowId, "Phone destination", 99);
  const task = await apiClient.createTask(seedData.workspaceId, "Phone bulk scope", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const board = new KanbanPage(testPage);
  await board.goto();
  await testPage.getByTestId("mobile-select-tasks").tap();
  await board.taskCard(task.id).tap();
  await expect(board.multiSelectToolbar).toContainText("1 selected");
  await testPage.getByTestId("bulk-actions-button").tap();
  const sheet = testPage.getByRole("dialog", { name: "Actions" });
  await expect(sheet).toBeVisible();
  await sheet.evaluate(async (element) => {
    await Promise.all(
      element
        .getAnimations({ subtree: true })
        .filter((animation) => Number.isFinite(animation.effect?.getComputedTiming().iterations))
        .map((animation) => animation.finished.catch(() => undefined)),
    );
  });
  const requests: unknown[] = [];
  testPage.on("request", (request) => {
    if (
      request.method() === "POST" &&
      new URL(request.url()).pathname === "/api/v1/tasks/bulk-move"
    ) {
      requests.push(request.postDataJSON());
    }
  });
  await board.bulkMoveButton.tap();
  await expect(board.bulkMoveStepOption(target.id)).toBeVisible();
  expect(requests).toEqual([]);
  const response = testPage.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname === "/api/v1/tasks/bulk-move",
  );
  await board.bulkMoveStepOption(target.id).tap();
  expect((await response).ok()).toBe(true);
  expect(requests).toEqual([
    { task_ids: [task.id], target_workflow_id: seedData.workflowId, target_step_id: target.id },
  ]);
  await expect
    .poll(async () => {
      const current = await apiClient.getTask(task.id);
      return { workflow: current.workflow_id, step: current.workflow_step_id };
    })
    .toEqual({ workflow: seedData.workflowId, step: target.id });
});
