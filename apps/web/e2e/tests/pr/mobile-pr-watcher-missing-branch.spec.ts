// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { restoreSeedRepositoryOrigin, test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile PR watcher missing branch", () => {
  test("keeps the typed recovery card reachable", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);

    restoreSeedRepositoryOrigin(seedData);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const prBranch = "feature/already-merged-and-deleted-mobile";
    await apiClient.mockGitHubAddPRs([
      {
        number: 1000,
        title: "Open mobile feature with deleted branch",
        state: "open",
        head_branch: prBranch,
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "PR #1000: Open mobile feature with deleted branch",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        repositories: [
          {
            repository_id: seedData.repositoryId,
            base_branch: "main",
            checkout_branch: prBranch,
            pr_number: 1000,
          },
        ],
        metadata: {
          pr_number: 1000,
          pr_branch: prBranch,
          pr_repo: "testorg/testrepo",
          pr_author: "test-user",
        },
      },
    );

    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: 1000,
      pr_url: "https://github.com/testorg/testrepo/pull/1000",
      pr_title: "Open mobile feature with deleted branch",
      head_branch: prBranch,
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const chat = session.activeChat();
    const recovery = chat.getByTestId("task-launch-error-entry");

    await expect(recovery).toHaveCount(1, { timeout: 30_000 });
    await expect(recovery).toContainText("The selected base branch is not available.");
    await expect(recovery).not.toContainText(prBranch);
    const actionButtons = recovery.locator("button[data-testid^='task-launch-']");
    await expect(actionButtons).not.toHaveCount(0);
    for (const button of await actionButtons.all()) {
      await expect(button).toBeVisible();
      await expect(button).toBeInViewport();
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    await assertNoDocumentHorizontalOverflow(testPage, "typed launch recovery");
    await testPage.screenshot({
      path: testInfo.outputPath("missing-pr-branch-mobile.png"),
      fullPage: true,
    });
  });
});
