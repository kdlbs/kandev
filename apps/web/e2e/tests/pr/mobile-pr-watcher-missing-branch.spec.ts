// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { restoreSeedRepositoryOrigin, test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile PR watcher missing branch", () => {
  test("keeps the recovery summary and task actions reachable", async ({
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
        title: "Already merged mobile feature",
        state: "closed",
        head_branch: prBranch,
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
      },
    ]);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "PR #1000: Already merged mobile feature",
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
      pr_title: "Already merged mobile feature",
      head_branch: prBranch,
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    const chat = session.activeChat();
    const recovery = chat.getByTestId("missing-branch-recovery");

    await expect(recovery).toHaveCount(1, { timeout: 30_000 });
    await expect(
      recovery.getByRole("heading", { name: "Branch is no longer available" }),
    ).toBeVisible();
    await expect(recovery).toContainText(prBranch);
    await expect(
      chat.getByRole("status", { name: /Session failed|Environment setup failed/i }),
    ).toHaveCount(0);

    const archiveButton = recovery.getByTestId("missing-branch-archive-button");
    const deleteButton = recovery.getByTestId("missing-branch-delete-button");
    for (const button of [archiveButton, deleteButton]) {
      await expect(button).toBeVisible();
      await expect(button).toBeInViewport();
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    const technicalDetails = recovery.locator("details");
    const technicalOutput = technicalDetails.locator("pre");
    await expect(technicalDetails).not.toHaveAttribute("open");
    await expect(technicalOutput).not.toBeVisible();
    const disclosure = recovery.getByText("Technical details", { exact: true });
    await disclosure.tap();
    await expect(technicalOutput).toBeVisible();
    await expect(technicalOutput).toContainText("couldn't find remote ref pull/1000/head");
    const disclosureBox = await disclosure.boundingBox();
    expect(disclosureBox).not.toBeNull();
    expect(disclosureBox!.height).toBeGreaterThanOrEqual(44);

    await assertNoDocumentHorizontalOverflow(testPage, "missing branch recovery");
    await testPage.screenshot({
      path: testInfo.outputPath("missing-pr-branch-mobile.png"),
      fullPage: true,
    });
  });
});
