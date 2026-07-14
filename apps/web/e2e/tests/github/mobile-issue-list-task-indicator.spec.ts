import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";

test.describe("Mobile GitHub issue task indicator", () => {
  test("keeps the linked task reachable by touch without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const owner = "mobileorg";
    const repo = "mobilerepo";
    const number = 200;
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddIssues([
      {
        number,
        title: "Mobile linked issue",
        state: "open",
        author_login: "test-user",
        repo_owner: owner,
        repo_name: repo,
        assignees: ["test-user"],
      },
    ]);
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile linked task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      metadata: {
        issue_url: `https://github.com/${owner}/${repo}/issues/${number}`,
        issue_number: number,
        issue_repo: `${owner}/${repo}`,
      },
    });

    const github = new MobileGitHubPage(testPage);
    await github.goto();
    await github.mobileMenuButton.tap();
    await github.mobileSidebar.getByRole("button", { name: "Issues", exact: true }).tap();

    const row = testPage.locator(`[data-testid="issue-row"][data-issue-number="${number}"]`);
    const indicator = row.getByTestId("issue-row-task-indicator-single");
    await expect(indicator).toContainText("Mobile linked task", { timeout: 15_000 });
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await indicator.tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}(?:\\?|$)`), {
      timeout: 15_000,
    });
  });
});
