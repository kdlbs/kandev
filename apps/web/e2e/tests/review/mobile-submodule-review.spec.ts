import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createSubmoduleReviewFixture,
  expectStickyReviewHeaderClearance,
} from "./submodule-review-helpers";

test.describe("Nested submodule Review on mobile", () => {
  test.describe.configure({ timeout: 180_000 });

  test("keeps nested scope and diff context touch-reachable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = await createSubmoduleReviewFixture(
      apiClient,
      seedData,
      backend.tmpDir,
      "Mobile nested submodule Review E2E",
    );

    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 45_000 });

      const worktreePath = await fixture.waitForWorktree(apiClient);
      await fixture.applyNestedChanges(worktreePath);

      await testPage.getByRole("button", { name: "Changes" }).tap();
      const changesPanel = testPage.getByTestId("mobile-changes-panel");
      await expect(changesPanel).toBeVisible({ timeout: 15_000 });
      await expect(changesPanel.getByText("README.md").first()).toBeVisible({
        timeout: 45_000,
      });
      await changesPanel.getByRole("button", { name: "Review", exact: true }).tap();

      const review = session.reviewDialog();
      await expect(review).toBeVisible({ timeout: 15_000 });
      const repositoryLabels = review.getByTestId("review-file-repository");
      await expect(repositoryLabels).toHaveCount(2);
      await expect(repositoryLabels.filter({ hasText: /^vendor\/outer$/ })).toBeVisible();
      const innerLabel = repositoryLabels.filter({
        hasText: /^vendor\/outer\/vendor\/inner$/,
      });
      await expect(innerLabel).toBeVisible({ timeout: 15_000 });
      await expectStickyReviewHeaderClearance(review, "touch");

      const innerHeader = review
        .getByTestId("review-file-header")
        .filter({ hasText: "vendor/outer/vendor/inner" });
      await expect(innerHeader).toBeVisible({ timeout: 15_000 });
      await innerHeader.scrollIntoViewIfNeeded();
      await expect
        .poll(() => session.reviewDiffText(), { timeout: 45_000 })
        .toContain("inner committed change");

      const nestedScopeHeaders = review.locator(
        '[data-testid="changes-repo-header"][data-submodule-scope="true"]',
      );
      await expect(nestedScopeHeaders).toHaveCount(2);
      await expect(
        nestedScopeHeaders.filter({ hasText: "vendor/outer/vendor/inner" }),
      ).toBeVisible();

      const [dialogBox, innerLabelBox, viewport] = await Promise.all([
        review.boundingBox(),
        innerLabel.boundingBox(),
        testPage.evaluate(() => ({ width: window.innerWidth, height: window.innerHeight })),
      ]);
      if (!dialogBox || !innerLabelBox) {
        throw new Error("Nested submodule Review geometry is unavailable");
      }
      expect(dialogBox.x).toBeGreaterThanOrEqual(0);
      expect(dialogBox.y).toBeGreaterThanOrEqual(0);
      expect(dialogBox.x + dialogBox.width).toBeLessThanOrEqual(viewport.width + 1);
      expect(dialogBox.y + dialogBox.height).toBeLessThanOrEqual(viewport.height + 1);
      expect(innerLabelBox.height).toBeGreaterThan(0);

      const overflow = await testPage.evaluate(() => ({
        viewport: document.documentElement.clientWidth,
        document: document.documentElement.scrollWidth,
      }));
      expect(overflow.document).toBeLessThanOrEqual(overflow.viewport + 1);
    } finally {
      fixture.cleanup();
    }
  });
});
