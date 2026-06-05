import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Chat status bar", () => {
  test("shows todo indicator alone when no PR is associated", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // e2e:plan emits an ACP Plan update with todo entries
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Todo Only Task",
      seedData.agentProfileId,
      {
        description: [
          'e2e:plan([{"content":"Review code","status":"in_progress"},{"content":"Run tests","status":"pending"}])',
          'e2e:message("todo only response")',
        ].join("\n"),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Todo Only Task");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("todo only response").last()).toBeVisible({
      timeout: 30_000,
    });

    // Todo indicator should be visible, PR banner should not
    const statusBar = session.chatStatusBar();
    await expect(statusBar).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("todo-indicator")).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("pr-merged-banner")).not.toBeVisible();
  });

  test("shows PR merged banner alone when no todos exist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "PR Banner Only Task",
      seedData.agentProfileId,
      {
        description: 'e2e:message("pr banner only response")',
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

    const card = kanban.taskCardByTitle("PR Banner Only Task");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("pr banner only response").last()).toBeVisible({
      timeout: 30_000,
    });

    // PR merged banner should be visible, todo indicator should not
    const statusBar = session.chatStatusBar();
    await expect(statusBar).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("pr-merged-banner")).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("pr-merged-banner")).toContainText(
      "PR #101 has been merged",
    );
    await expect(statusBar.getByTestId("todo-indicator")).not.toBeVisible();
  });

  test("shows both todo indicator and PR banner on the same row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Both Indicators Task",
      seedData.agentProfileId,
      {
        description: [
          'e2e:plan([{"content":"Review code","status":"completed"},{"content":"Run tests","status":"in_progress"}])',
          'e2e:message("both indicators response")',
        ].join("\n"),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 202,
      pr_url: "https://github.com/test-org/test-repo/pull/202",
      pr_title: "Both Test PR",
      head_branch: "feature/both",
      base_branch: "main",
      author_login: "test-user",
      state: "merged",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Both Indicators Task");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("both indicators response").last()).toBeVisible({
      timeout: 30_000,
    });

    // Both should be visible inside the same status bar container (same row)
    const statusBar = session.chatStatusBar();
    await expect(statusBar).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("todo-indicator")).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("pr-merged-banner")).toBeVisible({ timeout: 10_000 });
    await expect(statusBar.getByTestId("pr-merged-banner")).toContainText(
      "PR #202 has been merged",
    );
  });

  test("dismissed PR merged banner stays hidden across reload and task switch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Dismiss Banner Task A",
      seedData.agentProfileId,
      {
        description: 'e2e:message("dismiss banner alpha response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Dismiss Banner Task B",
      seedData.agentProfileId,
      {
        description: 'e2e:message("dismiss banner beta response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 404,
      pr_url: "https://github.com/test-org/test-repo/pull/404",
      pr_title: "Dismiss Test PR",
      head_branch: "feature/dismiss",
      base_branch: "main",
      author_login: "test-user",
      state: "merged",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCardByTitle("Dismiss Banner Task A");
    await expect(cardA).toBeVisible({ timeout: 30_000 });
    await cardA.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("dismiss banner alpha response").last()).toBeVisible({
      timeout: 30_000,
    });

    // Banner appears, then user dismisses it.
    await expect(session.prMergedBanner()).toBeVisible({ timeout: 10_000 });
    await session.prMergedDismissButton().click();
    await expect(session.prMergedBanner()).not.toBeVisible();

    // Persists across reload.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.chat.getByText("dismiss banner alpha response").last()).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.prMergedBanner()).not.toBeVisible();

    // Switch to task B and back. Task B has no merged PR so it never shows
    // the banner — the real proof of cross-task-switch persistence is that
    // the banner is still hidden on the return trip to task A.
    await session.taskInSidebar("Dismiss Banner Task B").first().click();
    await expect(session.chat.getByText("dismiss banner beta response").last()).toBeVisible({
      timeout: 30_000,
    });

    await session.taskInSidebar("Dismiss Banner Task A").first().click();
    await expect(session.chat.getByText("dismiss banner alpha response").last()).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.prMergedBanner()).not.toBeVisible();
  });

  test("shows PR closed banner and hides the CI chip when the PR is closed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "PR Closed Banner Task",
      seedData.agentProfileId,
      {
        description: 'e2e:message("pr closed banner response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 505,
      pr_url: "https://github.com/test-org/test-repo/pull/505",
      pr_title: "Closed Test PR",
      head_branch: "feature/closed",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("PR Closed Banner Task");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("pr closed banner response").last()).toBeVisible({
      timeout: 30_000,
    });

    const statusBar = session.chatStatusBar();
    await expect(statusBar).toBeVisible({ timeout: 10_000 });
    await expect(session.prClosedBanner()).toBeVisible({ timeout: 10_000 });
    await expect(session.prClosedBanner()).toContainText("PR #505 was closed without merging");
    // The CI chip is redundant once the PR is terminal — the banner conveys it.
    await expect(session.prStatusChip()).not.toBeVisible();
    await expect(session.prMergedBanner()).not.toBeVisible();
  });

  test("dismissed PR closed banner stays hidden across reload and task switch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Dismiss Closed Banner Task A",
      seedData.agentProfileId,
      {
        description: 'e2e:message("dismiss closed alpha response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Dismiss Closed Banner Task B",
      seedData.agentProfileId,
      {
        description: 'e2e:message("dismiss closed beta response")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: taskA.id,
      owner: "test-org",
      repo: "test-repo",
      pr_number: 606,
      pr_url: "https://github.com/test-org/test-repo/pull/606",
      pr_title: "Dismiss Closed Test PR",
      head_branch: "feature/dismiss-closed",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const cardA = kanban.taskCardByTitle("Dismiss Closed Banner Task A");
    await expect(cardA).toBeVisible({ timeout: 30_000 });
    await cardA.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText("dismiss closed alpha response").last()).toBeVisible({
      timeout: 30_000,
    });

    // Banner appears, then user dismisses it.
    await expect(session.prClosedBanner()).toBeVisible({ timeout: 10_000 });
    await session.prClosedDismissButton().click();
    await expect(session.prClosedBanner()).not.toBeVisible();

    // Persists across reload.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.chat.getByText("dismiss closed alpha response").last()).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.prClosedBanner()).not.toBeVisible();

    // Switch to task B (no closed PR, so never shows the banner) and back. The
    // proof of cross-task-switch persistence is the banner still hidden on the
    // return trip to task A.
    await session.taskInSidebar("Dismiss Closed Banner Task B").first().click();
    await expect(session.chat.getByText("dismiss closed beta response").last()).toBeVisible({
      timeout: 30_000,
    });

    await session.taskInSidebar("Dismiss Closed Banner Task A").first().click();
    await expect(session.chat.getByText("dismiss closed alpha response").last()).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.prClosedBanner()).not.toBeVisible();
  });

  test("archive via PR banner switches to next task", async ({ testPage, apiClient, seedData }) => {
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
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

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

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 10_000 });
    expect(testPage.url()).not.toBe(urlBeforeArchive);
  });
});
