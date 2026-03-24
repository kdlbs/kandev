import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Chat status bar", () => {
  test("shows PR merged banner inside status bar", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "PR Banner Task",
      seedData.agentProfileId,
      {
        description: 'e2e:message("pr banner response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 101,
      pr_url: "https://github.com/test-org/test-repo/pull/101",
      pr_title: "Test PR",
      head_branch: "feature/test",
      base_branch: "main",
      author_login: "test-user",
      state: "merged",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("PR Banner Task");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("pr banner response").last()).toBeVisible({
      timeout: 30_000,
    });

    // PR merged banner should be visible inside the chat status bar
    const statusBar = session.chatStatusBar();
    await expect(statusBar).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("pr-merged-banner")).toBeVisible();
    await expect(statusBar.getByTestId("pr-merged-banner")).toContainText(
      "PR #101 has been merged",
    );
  });

  test("archive via PR banner switches to next task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Archive Banner Task A",
      seedData.agentProfileId,
      {
        description: 'e2e:message("archive banner alpha response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Archive Banner Task B",
      seedData.agentProfileId,
      {
        description: 'e2e:message("archive banner beta response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 303,
      pr_url: "https://github.com/test-org/test-repo/pull/303",
      pr_title: "Archive Test PR",
      head_branch: "feature/archive",
      base_branch: "main",
      author_login: "test-user",
      state: "merged",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCardByTitle("Archive Banner Task A");
    await expect(cardA).toBeVisible({ timeout: 30_000 });
    await cardA.click();
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("archive banner alpha response").last()).toBeVisible({
      timeout: 30_000,
    });

    await expect(session.prMergedBanner()).toBeVisible({ timeout: 10_000 });

    const urlBeforeArchive = testPage.url();

    // Click archive in the PR merged banner
    await session.prMergedArchiveButton().click();

    // Should switch to task B
    await expect(session.taskInSidebar("Archive Banner Task A")).not.toBeVisible({
      timeout: 15_000,
    });
    await expect(session.taskInSidebar("Archive Banner Task B")).toBeVisible({ timeout: 10_000 });

    await expect(session.chat.getByText("archive banner beta response").last()).toBeVisible({
      timeout: 15_000,
    });

    await expect(testPage).toHaveURL(/\/s\//, { timeout: 10_000 });
    expect(testPage.url()).not.toBe(urlBeforeArchive);
  });
});
