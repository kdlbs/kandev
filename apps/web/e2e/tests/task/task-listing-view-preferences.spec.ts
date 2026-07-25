import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const RICH_TASK_TITLE = "Portable rich task";
const RICH_TASK_DESCRIPTION = "Rich rows retain this useful task context.";
const SEEDED_REPOSITORY_LABEL = "E2E Repo";

test.describe("Task listing display preferences", () => {
  test("restores Pipeline and List from the device-local view preference", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "View restoration task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto("/");
    const kanban = new KanbanPage(testPage);
    await expect(kanban.board).toBeVisible();

    await kanban.viewTogglePipeline.click();
    await expect(kanban.viewTogglePipeline).toHaveAttribute("data-state", "on");
    await testPage.reload();
    await expect(kanban.viewTogglePipeline).toHaveAttribute("data-state", "on");

    await kanban.viewToggleKanban.click();
    await expect(kanban.viewToggleKanban).toHaveAttribute("data-state", "on");
    await testPage.getByTestId("view-toggle-list").click();
    await expect(testPage).toHaveURL(/\/tasks/);
    await expect(testPage.getByTestId("tasks-list")).toBeVisible();

    await testPage.goto("/");
    await expect(testPage).toHaveURL(/\/tasks/);
    await expect(testPage.getByTestId("tasks-list")).toBeVisible();
  });

  test("persists rich List rows through the backend setting", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, RICH_TASK_TITLE, {
      description: RICH_TASK_DESCRIPTION,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "kandev",
      repo: "e2e-repo",
      pr_number: 42,
      pr_url: "https://github.com/kandev/e2e-repo/pull/42",
      pr_title: "Portable details",
      head_branch: "details",
      base_branch: "main",
      author_login: "e2e",
      state: "open",
      checks_state: "success",
      mergeable_state: "clean",
    });

    await testPage.goto("/tasks");
    const row = testPage.getByTestId("tasks-list-row").filter({ hasText: RICH_TASK_TITLE });
    await expect(row).toBeVisible();
    await expect(row).not.toContainText(RICH_TASK_DESCRIPTION);

    await testPage.getByTestId("display-button").click();
    await testPage.getByText("Show task details", { exact: true }).click();
    await expect(row).toContainText(SEEDED_REPOSITORY_LABEL);
    await expect(row).toContainText(RICH_TASK_DESCRIPTION);
    await expect(row.getByTestId(`pr-task-icon-${task.id}`)).toBeVisible();

    await testPage.reload();
    await expect(row).toContainText(SEEDED_REPOSITORY_LABEL);
    await expect(row).toContainText(RICH_TASK_DESCRIPTION);
    await expect(row.getByTestId(`pr-task-icon-${task.id}`)).toBeVisible();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.tasks_list_show_details)
      .toBe(true);
  });
});
