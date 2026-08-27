import { test, expect } from "../../fixtures/test-base";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
  assertTextWrapsNaturallyWithoutHorizontalOverflow,
  requireBox,
} from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

const OWNER = "testorg";
const REPO = "testrepo";
const PR_NUMBER = 418;
const REVIEWER = "reviewer-with-a-near-maximum-practical-github-login";
const MOBILE_PR_TITLE =
  "Keep this long pull request title visible while review and merge controls wrap below it at narrow widths";
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
    const mergeReadyPR = {
      task_id: task.id,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
      pr_title: MOBILE_PR_TITLE,
      head_branch: "feat/mobile-rerequest-review",
      base_branch: "main",
      author_login: "another-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      review_count: 1,
      pending_review_count: 0,
      required_reviews: 1,
      checks_total: 1,
      checks_passing: 1,
    };
    await apiClient.mockGitHubAssociateTaskPR(mergeReadyPR);
    await apiClient.mockGitHubSeedPRFeedback({
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      checks: [{ name: "Frontend tests", status: "completed", conclusion: "success" }],
      reviews: [
        {
          id: 2,
          author: "code-owner",
          state: "APPROVED",
          created_at: "2026-07-23T09:00:00Z",
        },
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
    const panel = testPage.getByTestId("mobile-review-panel");
    await expect(panel).toBeVisible({ timeout: 15_000 });
    await assertLocatorWithinViewportX(panel, "mobile PR review panel");

    const detail = panel.getByTestId("change-request-detail");
    const title = detail.getByTestId("change-request-detail-title");
    const actions = detail.getByTestId("change-request-detail-actions");
    const approve = detail.getByTestId("pr-approve-button");
    const merge = detail.getByTestId("pr-merge-button");
    const action = session.prReRequestReviewButton(REVIEWER);
    await expect(title).toBeVisible();
    await expect(approve).toBeVisible();
    await expect(action).toBeVisible();
    // Feedback correctly marks a dismissed reviewer as pending. Re-pin the
    // stored projection so this regression fixture exposes every header action
    // while retaining the dismissed-review flow below it.
    await apiClient.mockGitHubAssociateTaskPR(mergeReadyPR);
    await expect(merge).toBeVisible({ timeout: 15_000 });
    const [detailBox, titleBox, actionsBox, approveBox, mergeBox] = await Promise.all([
      requireBox(detail, "mobile detail"),
      requireBox(title, "mobile title"),
      requireBox(actions, "mobile action cluster"),
      requireBox(approve, "mobile approve action"),
      requireBox(merge, "mobile merge action"),
    ]);
    expect(actionsBox.y, "mobile action cluster should sit below the title").toBeGreaterThanOrEqual(
      titleBox.y + titleBox.height,
    );
    expect(approveBox.y, "mobile approve action should sit below the title").toBeGreaterThanOrEqual(
      titleBox.y + titleBox.height,
    );
    expect(mergeBox.y, "mobile merge action should follow approval").toBeGreaterThanOrEqual(
      approveBox.y,
    );
    expect(
      mergeBox.y > approveBox.y || mergeBox.x > approveBox.x,
      "mobile merge action should preserve visual action order",
    ).toBe(true);
    expect(titleBox.width, "mobile title should own the full padded row").toBeGreaterThanOrEqual(
      detailBox.width - 25,
    );
    expect(approveBox.x, "mobile actions should share the title's leading edge").toBeCloseTo(
      titleBox.x,
      0,
    );
    expect(approveBox.height, "mobile approve action height").toBeGreaterThanOrEqual(44);
    expect(mergeBox.height, "mobile merge action height").toBeGreaterThanOrEqual(44);
    await assertTextWrapsNaturallyWithoutHorizontalOverflow(title, "mobile PR title");
    await assertLocatorWithinViewportX(approve, "mobile approve action");
    await assertLocatorWithinViewportX(merge, "mobile merge action");

    const box = await requireBox(action, "re-request review action");
    expect(box.width, "re-request review action width").toBeGreaterThanOrEqual(44);
    expect(box.height, "re-request review action height").toBeGreaterThanOrEqual(44);
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

    await testPage.getByTestId("review-item-selector-trigger").tap();
    const secondReview = testPage.getByRole("menuitemradio", {
      name: new RegExp(`^PR ${SWITCH_SECOND_PR_NUMBER}\\b`),
    });
    await expect(secondReview).toBeVisible();
    await secondReview.tap();
    await secondFeedbackRequested;

    await expect(action).toHaveCount(0);
    await expect(session.prReRequestReviewButton(REVIEWER)).toHaveCount(0);
    expect(mutationUrls).toEqual([]);

    releaseSecondFeedback();
  });
});
