import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Kanban board search matches linked PR/MR numbers", () => {
  test("searching a linked PR number finds the card by number alone, not by title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const stepOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };

    // Title deliberately shares no words with the PR number so a match can
    // only come from the linked PR, proving the number itself is indexed.
    const prTask = await apiClient.createTask(
      seedData.workspaceId,
      "Board search PR-linked card",
      stepOptions,
    );
    const unrelatedTask = await apiClient.createTask(
      seedData.workspaceId,
      "Board search unrelated card",
      stepOptions,
    );

    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: prTask.id,
      owner: "kandev-e2e",
      repo: "board-search-fixtures",
      pr_number: 93315,
      pr_url: "https://github.test/kandev-e2e/board-search-fixtures/pull/93315",
      pr_title: "Totally unrelated PR title",
      head_branch: "feature/board-search",
      base_branch: "main",
      author_login: "board-search-author",
      state: "open",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCard(prTask.id)).toBeVisible();
    await expect(kanban.taskCard(unrelatedTask.id)).toBeVisible();

    const search = testPage.getByTestId("kanban-header-search").getByPlaceholder("Search tasks...");

    await search.fill("93315");
    await expect(kanban.taskCard(prTask.id)).toBeVisible();
    await expect(kanban.taskCard(unrelatedTask.id)).toBeHidden();

    // The leading "#" form must match too.
    await search.fill("#93315");
    await expect(kanban.taskCard(prTask.id)).toBeVisible();
    await expect(kanban.taskCard(unrelatedTask.id)).toBeHidden();

    // A number that matches nothing hides both cards.
    await search.fill("77777");
    await expect(kanban.taskCard(prTask.id)).toBeHidden();
    await expect(kanban.taskCard(unrelatedTask.id)).toBeHidden();

    await search.fill("");
    await expect(kanban.taskCard(prTask.id)).toBeVisible();
    await expect(kanban.taskCard(unrelatedTask.id)).toBeVisible();
  });
});
