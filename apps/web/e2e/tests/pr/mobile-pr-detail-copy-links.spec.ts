import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 720;
const PR_URL = `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`;
const COMMENT_URLS = new Map([
  [201, `${PR_URL}#discussion_r201`],
  [202, `${PR_URL}#discussion_r202`],
  [203, `${PR_URL}#issuecomment-203`],
]);

async function readClipboard(page: Page) {
  return page.evaluate(() => navigator.clipboard.readText());
}

test.describe("mobile PR detail copy links", () => {
  test("copies PR and comment links with touch-sized controls", async ({
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
      "Copy links from mobile PR details",
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
      pr_title: "Copy links from mobile review details",
      head_branch: "feat/mobile-copy-pr-links",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      comments: [
        {
          id: 201,
          author: "reviewer",
          body: "Please update this line.",
          path: "main.go",
          line: 7,
          side: "RIGHT",
          comment_type: "review",
          html_url: COMMENT_URLS.get(201),
          created_at: "2026-08-01T09:00:00Z",
        },
        {
          id: 202,
          author: "test-user",
          body: "I will update it.",
          comment_type: "review",
          html_url: COMMENT_URLS.get(202),
          in_reply_to: 201,
          created_at: "2026-08-01T09:01:00Z",
        },
        {
          id: 203,
          author: "maintainer",
          body: "Thanks for the follow-up.",
          comment_type: "issue",
          html_url: COMMENT_URLS.get(203),
          created_at: "2026-08-01T09:02:00Z",
        },
      ],
    });

    await testPage.goto(`/t/${task.id}`);
    await testPage.getByRole("button", { name: "Review", exact: true }).tap();

    const panel = testPage.getByTestId("mobile-review-panel");
    await expect(panel).toBeVisible({ timeout: 15_000 });
    const detail = panel.getByTestId("change-request-detail");
    await expect(detail).toBeVisible();
    await prCapture.screenshot("mobile-pr-detail-copy-links", {
      caption: "Mobile pull request details with touch-sized copy actions",
    });

    const requestCopy = detail.getByTestId("change-request-copy-url");
    await expect(requestCopy).toBeVisible();
    expect((await requireBox(requestCopy, "mobile PR copy action")).height).toBeGreaterThanOrEqual(
      44,
    );
    await requestCopy.tap();
    await expect.poll(() => readClipboard(testPage)).toBe(PR_URL);
    await expect(requestCopy).toHaveAttribute("aria-label", "Pull request URL copied");

    for (const id of [201, 202, 203]) {
      const copy = detail.getByTestId(`change-request-comment-copy-${id}`);
      await expect(copy).toBeVisible();
      expect(
        (await requireBox(copy, `mobile comment ${id} copy action`)).height,
      ).toBeGreaterThanOrEqual(44);
      await copy.tap();
      await expect.poll(() => readClipboard(testPage)).toBe(COMMENT_URLS.get(id));
      await expect(copy).toHaveAttribute("aria-label", "Comment URL copied");
    }

    await assertNoDocumentHorizontalOverflow(testPage, "mobile PR detail copy links");
  });
});
