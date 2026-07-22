import { test, expect } from "../../fixtures/test-base";
import {
  REVIEW_OWNER,
  REVIEW_PRS,
  REVIEW_REPO,
  REVIEW_SHARED_FILE,
  seedMultiPRReviewTask,
} from "../../helpers/multi-pr-review";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

async function openMobileReview(testPage: Page, session: SessionPage) {
  await testPage.getByRole("button", { name: "Changes" }).tap();
  const changesPanel = testPage.getByTestId("mobile-changes-panel");
  await expect(changesPanel).toBeVisible({ timeout: 15_000 });
  const prFiles = changesPanel.getByTestId("pr-files-section");
  await expect(prFiles).toBeVisible({ timeout: 20_000 });
  for (const pr of REVIEW_PRS) {
    await expect(
      prFiles.locator(
        `[data-changes-file=${JSON.stringify(REVIEW_SHARED_FILE)}][data-pr-key="${REVIEW_OWNER}/${REVIEW_REPO}/${pr.number}"]`,
      ),
    ).toBeVisible();
  }
  await changesPanel.getByRole("button", { name: "Review", exact: true }).tap();
  await expect(session.reviewDialog()).toBeVisible({ timeout: 15_000 });
}

test.describe("Review dialog multi-PR selector on mobile", () => {
  test.describe.configure({ timeout: 120_000 });

  test("switches PRs from a contained touch menu without viewport overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await seedMultiPRReviewTask(apiClient, seedData, "Mobile Multi-PR Review E2E");
    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await openMobileReview(testPage, session);

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

    await selector.tap();
    const menu = session.reviewPRSelectorMenu();
    const secondItem = session.reviewPRSelectorItem(REVIEW_OWNER, REVIEW_REPO, secondPR.number);
    await expect(menu).toBeVisible();
    await expect(secondItem).toBeVisible();

    const [selectorBox, menuBox, itemBox, viewport] = await Promise.all([
      selector.boundingBox(),
      menu.boundingBox(),
      secondItem.boundingBox(),
      testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
    ]);
    if (!selectorBox || !menuBox || !itemBox) {
      throw new Error("Review PR selector geometry is unavailable");
    }
    expect(selectorBox.height).toBeGreaterThanOrEqual(44);
    expect(itemBox.height).toBeGreaterThanOrEqual(44);
    expect(menuBox.x).toBeGreaterThanOrEqual(0);
    expect(menuBox.y).toBeGreaterThanOrEqual(0);
    expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(viewport.width + 1);
    expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(viewport.height + 1);

    const openMenuOverflow = await testPage.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      document: document.documentElement.scrollWidth,
    }));
    expect(openMenuOverflow.document).toBeLessThanOrEqual(openMenuOverflow.viewport + 1);

    await secondItem.tap();
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

    const switchedOverflow = await testPage.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      document: document.documentElement.scrollWidth,
    }));
    expect(switchedOverflow.document).toBeLessThanOrEqual(switchedOverflow.viewport + 1);
  });

  test("keeps the selector usable in the coarse-pointer tablet layout", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 820, height: 900 });
    const task = await seedMultiPRReviewTask(apiClient, seedData, "Tablet Multi-PR Review E2E");
    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible();

    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    await expect(session.reviewDialog()).toBeVisible();
    const dialogSelector = session.reviewPRSelectorTrigger();
    await expect(dialogSelector).toBeVisible();
    const [dialogSelectorBox, toolbarBox] = await Promise.all([
      dialogSelector.boundingBox(),
      dialogSelector.locator("xpath=../..").boundingBox(),
    ]);
    if (!dialogSelectorBox || !toolbarBox) {
      throw new Error("Tablet Review selector geometry is unavailable");
    }
    expect(Math.round(dialogSelectorBox.height)).toBeGreaterThanOrEqual(44);
    expect(toolbarBox.height).toBeGreaterThanOrEqual(44);

    await dialogSelector.tap();
    await session.reviewPRSelectorItem(REVIEW_OWNER, REVIEW_REPO, REVIEW_PRS[1].number).tap();
    await expect(dialogSelector).toHaveAttribute("data-pr-number", String(REVIEW_PRS[1].number));
    await expect.poll(() => session.reviewDiffText()).toContain(REVIEW_PRS[1].marker);
  });
});
