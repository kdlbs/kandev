import { test, expect } from "../../fixtures/test-base";

const OWNER = "testorg";
const REPO = "testrepo";

function issueMetadata(number: number) {
  return {
    issue_url: `https://github.com/${OWNER}/${REPO}/issues/${number}`,
    issue_number: number,
    issue_repo: `${OWNER}/${REPO}`,
  };
}

test.describe("GitHub issue list task indicator", () => {
  test("shows linked tasks, leaves unlinked issues unchanged, and navigates", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddIssues(
      [100, 101, 102].map((number) => ({
        number,
        title: `Issue ${number}`,
        state: "open",
        author_login: "test-user",
        repo_owner: OWNER,
        repo_name: REPO,
        assignees: ["test-user"],
      })),
    );

    const linked = await apiClient.createTask(seedData.workspaceId, "Linked issue task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      metadata: issueMetadata(100),
    });
    for (const title of ["First shared task", "Second shared task"]) {
      await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        metadata: issueMetadata(102),
      });
    }

    await testPage.goto("/github");
    const scopeBar = testPage.getByTestId("github-presets-scope-bar");
    await expect(scopeBar).toBeVisible({ timeout: 15_000 });
    await scopeBar.getByRole("button", { name: "Issues", exact: true }).click();

    const linkedRow = testPage.locator('[data-testid="issue-row"][data-issue-number="100"]');
    const unlinkedRow = testPage.locator('[data-testid="issue-row"][data-issue-number="101"]');
    const sharedRow = testPage.locator('[data-testid="issue-row"][data-issue-number="102"]');
    await expect(linkedRow).toBeVisible({ timeout: 15_000 });
    await expect(linkedRow.getByTestId("issue-row-task-indicator-single")).toContainText(
      "Linked issue task",
    );
    await expect(unlinkedRow.getByTestId("issue-row-task-indicator-single")).toHaveCount(0);
    await expect(sharedRow.getByTestId("issue-row-task-indicator-multi")).toContainText("2");

    await linkedRow.getByTestId("issue-row-task-indicator-single").click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${linked.id}(?:\\?|$)`), {
      timeout: 15_000,
    });
  });
});
