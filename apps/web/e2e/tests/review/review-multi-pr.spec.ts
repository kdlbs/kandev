import { test, expect } from "../../fixtures/test-base";
import {
  REVIEW_OWNER,
  REVIEW_PRS,
  REVIEW_REPO,
  REVIEW_SHARED_FILE,
  seedMultiPRReviewTask,
} from "../../helpers/multi-pr-review";
import { SessionPage } from "../../pages/session-page";

async function openDesktopReview(session: SessionPage) {
  await session.clickTab("Changes");
  const prFiles = session.prFilesSection();
  await expect(prFiles).toBeVisible({ timeout: 20_000 });
  for (const pr of REVIEW_PRS) {
    await expect(
      prFiles.locator(
        `[data-changes-file=${JSON.stringify(REVIEW_SHARED_FILE)}][data-pr-key="${REVIEW_OWNER}/${REVIEW_REPO}/${pr.number}"]`,
      ),
    ).toBeVisible();
  }
  await session.changes.getByRole("button", { name: "Review", exact: true }).click();
  await expect(session.reviewDialog()).toBeVisible({ timeout: 15_000 });
}

test.describe("Review dialog multi-PR selector", () => {
  test.describe.configure({ timeout: 120_000 });

  test("switches between same-repository PRs without stale files or diff content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await seedMultiPRReviewTask(apiClient, seedData, "Multi-PR Review E2E");
    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await openDesktopReview(session);

    const [firstPR, secondPR] = REVIEW_PRS;
    const selector = session.reviewPRSelectorTrigger();
    await expect(selector).toBeVisible();
    await expect(selector).toHaveAttribute("data-pr-number", String(firstPR.number));
    await expect(session.reviewFileHeader(REVIEW_SHARED_FILE)).toBeVisible();
    await expect(session.reviewDialog().getByTestId("changes-repo-group")).toHaveAttribute(
      "data-repository-name",
      firstPR.repositoryName,
    );
    await expect
      .poll(() => session.reviewDiffText(), { timeout: 30_000 })
      .toContain(firstPR.marker);

    await selector.click();
    const menu = session.reviewPRSelectorMenu();
    await expect(menu).toBeVisible();
    await session.reviewPRSelectorItem(REVIEW_OWNER, REVIEW_REPO, secondPR.number).click();

    await expect(menu).toBeHidden();
    await expect(session.reviewDialog()).toBeVisible();
    await expect(selector).toHaveAttribute("data-pr-number", String(secondPR.number));
    await expect(session.reviewFileHeader(REVIEW_SHARED_FILE)).toBeVisible({ timeout: 20_000 });
    await expect(session.reviewDialog().getByTestId("changes-repo-group")).toHaveAttribute(
      "data-repository-name",
      secondPR.repositoryName,
    );
    await expect
      .poll(() => session.reviewDiffText(), { timeout: 30_000 })
      .toContain(secondPR.marker);
    await expect.poll(() => session.reviewDiffText()).not.toContain(firstPR.marker);
  });
});
