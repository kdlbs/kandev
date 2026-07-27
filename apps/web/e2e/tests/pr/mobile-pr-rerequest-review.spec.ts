import { test, expect } from "../../fixtures/test-base";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 418;
const REVIEWER = "reviewer-with-a-near-maximum-practical-github-login";
const SWITCH_PR_NUMBER = 420;
const SWITCH_SECOND_PR_NUMBER = 421;

test.describe("mobile PR re-request review", () => {
  test("uses bottom-nav Review to re-request a dismissed review without overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 320, height: 720 });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile re-request dismissed review",
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
      pr_title: "Mobile re-request dismissed review",
      head_branch: "feat/mobile-rerequest-review",
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
    // The task-PR association can arrive before this route's WS subscription;
    // reload to hydrate the linked PR from the authoritative boot payload.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.prTopbarButton()).toHaveCount(0);

    await testPage.getByRole("button", { name: "Review", exact: true }).tap();
    const panel = testPage.getByTestId("mobile-pr-review-panel");
    await expect(panel).toBeVisible({ timeout: 15_000 });
    await assertLocatorWithinViewportX(panel, "mobile PR review panel");

    const action = session.prReRequestReviewButton(REVIEWER);
    await expect(action).toBeVisible();
    const box = await action.boundingBox();
    expect(box, "re-request review action has no bounding box").not.toBeNull();
    if (box) {
      expect(box.width, "re-request review action width").toBeGreaterThanOrEqual(44);
      expect(box.height, "re-request review action height").toBeGreaterThanOrEqual(44);
    }
    await assertLocatorWithinViewportX(action, "mobile re-request review action");
    const reRequestResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response
          .url()
          .includes(`/api/v1/github/prs/${OWNER}/${REPO}/${PR_NUMBER}/requested-reviewers`),
    );
    await action.tap();
    await reRequestResponse;

    await expect(session.prPendingReviewer(REVIEWER)).toContainText("Pending review");
    await expect(session.prSubmittedReview(REVIEWER)).toHaveCount(0);
    await expect(session.prReRequestReviewButton(REVIEWER)).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile PR re-request review");
  });

  test("drops PR A feedback before delayed PR B feedback can receive PR A's action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR identity switch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    for (const prNumber of [SWITCH_PR_NUMBER, SWITCH_SECOND_PR_NUMBER]) {
      await apiClient.mockGitHubAssociateTaskPR({
        task_id: task.id,
        owner: OWNER,
        repo: REPO,
        pr_number: prNumber,
        pr_url: `https://github.com/${OWNER}/${REPO}/pull/${prNumber}`,
        pr_title: `PR ${prNumber}`,
        head_branch: `feat/pr-${prNumber}`,
        base_branch: "main",
        author_login: "another-user",
        state: "open",
      });
    }
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: SWITCH_PR_NUMBER,
      reviews: [
        { id: 1, author: REVIEWER, state: "DISMISSED", created_at: "2026-07-23T10:00:00Z" },
      ],
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: SWITCH_SECOND_PR_NUMBER,
      reviews: [],
    });

    let releaseSecondFeedback!: () => void;
    const secondFeedbackHeld = new Promise<void>((resolve) => {
      releaseSecondFeedback = resolve;
    });
    let observeSecondFeedback!: () => void;
    const secondFeedbackRequested = new Promise<void>((resolve) => {
      observeSecondFeedback = resolve;
    });
    await testPage.route(
      (url) => url.pathname === `/api/v1/github/prs/${OWNER}/${REPO}/${SWITCH_SECOND_PR_NUMBER}`,
      async (route) => {
        const response = await route.fetch();
        observeSecondFeedback();
        await secondFeedbackHeld;
        await route.fulfill({ response });
      },
    );
    const mutationUrls: string[] = [];
    testPage.on("request", (request) => {
      if (request.method() === "POST" && request.url().includes("/requested-reviewers")) {
        mutationUrls.push(request.url());
      }
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByRole("button", { name: "Review", exact: true }).tap();
    const action = session.prReRequestReviewButton(REVIEWER);
    await expect(action).toBeVisible({ timeout: 15_000 });

    await testPage.getByTestId("mobile-review-pr-selector-trigger").tap();
    await testPage
      .getByTestId(`mobile-review-pr-selector-item-${OWNER}-${REPO}-${SWITCH_SECOND_PR_NUMBER}`)
      .tap();
    await secondFeedbackRequested;

    await expect(action).toHaveCount(0);
    await expect(session.prReRequestReviewButton(REVIEWER)).toHaveCount(0);
    expect(mutationUrls).toEqual([]);

    releaseSecondFeedback();
  });
});
