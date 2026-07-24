import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 417;
const REVIEWER = "OctoCat";

test.describe("PR re-request review", () => {
  test("re-requests a dismissed review from the PR detail panel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Desktop re-request dismissed review",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
      pr_title: "Re-request dismissed review",
      head_branch: "feat/rerequest-review",
      base_branch: "main",
      author_login: "another-user",
      state: "open",
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      reviews: [
        {
          id: 1,
          author: REVIEWER,
          state: "DISMISSED",
          created_at: "2026-07-23T10:00:00Z",
        },
      ],
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await session.prTopbarButton().click();
    await expect(session.prDetailPanel()).toBeVisible();

    const submitted = session.prSubmittedReview(REVIEWER);
    await expect(submitted).toContainText("Dismissed");
    await session.prReRequestReviewButton(REVIEWER).click();

    await expect(
      testPage.getByText(`Review re-requested from ${REVIEWER}`, { exact: true }),
    ).toBeVisible();
    await expect(session.prPendingReviewer(REVIEWER)).toContainText("Pending review");
    await expect(session.prSubmittedReview(REVIEWER)).toHaveCount(0);
    await expect(session.prReRequestReviewButton(REVIEWER)).toHaveCount(0);
  });
});
