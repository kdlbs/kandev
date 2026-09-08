import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 719;
const PR_URL = `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`;
const COMMENT_URLS = new Map([
  [101, `${PR_URL}#discussion_r101`],
  [102, `${PR_URL}#discussion_r102`],
  [103, `${PR_URL}#issuecomment-103`],
]);

async function readClipboard(page: Page) {
  return page.evaluate(() => navigator.clipboard.readText());
}

test.describe("PR detail copy links", () => {
  test("copies the PR URL and every visible comment permalink", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Copy links from PR details",
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
      pr_url: PR_URL,
      pr_title: "Copy links from review details",
      head_branch: "feat/copy-pr-links",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      comments: [
        {
          id: 101,
          author: "reviewer",
          body: "Please update this line.",
          path: "main.go",
          line: 7,
          side: "RIGHT",
          comment_type: "review",
          html_url: COMMENT_URLS.get(101),
          created_at: "2026-08-01T09:00:00Z",
        },
        {
          id: 102,
          author: "test-user",
          body: "I will update it.",
          comment_type: "review",
          html_url: COMMENT_URLS.get(102),
          in_reply_to: 101,
          created_at: "2026-08-01T09:01:00Z",
        },
        {
          id: 103,
          author: "maintainer",
          body: "Thanks for the follow-up.",
          comment_type: "issue",
          html_url: COMMENT_URLS.get(103),
          created_at: "2026-08-01T09:02:00Z",
        },
      ],
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await session.prTopbarButton().click();

    const detail = session.prDetailPanel().getByTestId("change-request-detail");
    await expect(detail).toBeVisible();
    const requestCopy = detail.getByTestId("change-request-copy-url");
    await expect(requestCopy).toBeVisible();
    await detail.getByTestId("change-request-comment-copy-101-row").hover();
    await prCapture.screenshot("desktop-pr-detail-copy-links", {
      caption: "Desktop pull request details with copy actions for the PR and comments",
    });
    await requestCopy.click();
    await expect.poll(() => readClipboard(testPage)).toBe(PR_URL);
    await expect(requestCopy).toHaveAttribute("aria-label", "Pull request URL copied");

    for (const id of [101, 102, 103]) {
      const copy = detail.getByTestId(`change-request-comment-copy-${id}`);
      await expect(copy).toBeVisible();
      await detail.getByTestId(`change-request-comment-copy-${id}-row`).hover();
      await copy.click();
      await expect.poll(() => readClipboard(testPage)).toBe(COMMENT_URLS.get(id));
      await expect(copy).toHaveAttribute("aria-label", "Comment URL copied");
    }
  });
});
