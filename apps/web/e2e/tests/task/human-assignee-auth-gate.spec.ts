import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

test("auth-disabled installs hide human assignee controls and indicators", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Hidden Human Assignee", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const response = await apiClient.rawRequest("PATCH", `/api/v1/tasks/${task.id}`, {
    assignee_user_id: "default-user",
  });
  expect(response.ok).toBe(true);

  await testPage.goto(`/t/${task.id}`);
  await expect(testPage.getByTestId("task-topbar-title")).toHaveText("Hidden Human Assignee");
  await expect(testPage.getByTestId("task-assignee-control")).toHaveCount(0);

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  const card = kanban.taskCard(task.id);
  await expect(card).toBeVisible();
  await expect(card.getByTestId("kanban-card-assignee")).toHaveCount(0);
});
