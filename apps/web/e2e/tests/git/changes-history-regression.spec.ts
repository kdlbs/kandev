import { test, expect } from "../../fixtures/test-base";
import { resizeColumnViaSplitview } from "../../helpers/dockview-resize";
import { expectNoPageHorizontalOverflow } from "./large-changes-helpers";
import { refreshSpacingAndExpectAnchor } from "./changes-commit-spacing-helpers";
import {
  openHistoryRegression,
  seedHistoryRelation,
  expectStaleHistory,
  expectDivergedHistory,
  expectHeaderGeometry,
  expectExpandedPRContiguous,
  measurePRSectionGeometry,
  expectRepositoryToggleTouchTarget,
} from "./changes-history-regression-helpers";

test.describe("Changes history regression", () => {
  test.describe.configure({ retries: 0, timeout: 120_000 });

  // @covers AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4
  test("stale upstream keeps unified history until current evidence arrives", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openHistoryRegression(testPage, apiClient, seedData, false);
    await seedHistoryRelation(testPage, "stale");
    await expectStaleHistory(testPage);
    await expect(testPage.getByTestId("commits-repo-push")).toHaveCount(0);
    await seedHistoryRelation(testPage, "local_ahead");
    await expectStaleHistory(testPage);
    await expect(testPage.getByTestId("commits-repo-push")).toBeEnabled();
  });

  test("confirmed divergence keeps both version histories", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openHistoryRegression(testPage, apiClient, seedData, false);
    await seedHistoryRelation(testPage, "diverged");
    await expectDivergedHistory(testPage);
    await testPage.getByTestId("header-remote-contribution-warning").click();
    await expect(testPage.getByTestId("header-compare-versions")).toBeVisible();
    await expect(testPage.getByTestId("header-replace-pr-branch")).toBeVisible();
    await expect(testPage.getByTestId("header-use-pr-version")).toBeVisible();
  });

  // @covers AC-UI-BOUNDED-CHANGES-001.6
  test("keeps collapsed history headers compact", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const session = await openHistoryRegression(testPage, apiClient, seedData, false);
    await seedHistoryRelation(testPage, "diverged");
    await expectDivergedHistory(testPage);
    // Measure single-line density in a pane wide enough for the full labels.
    // The narrower viewport loop below separately covers dynamic wrapping.
    await resizeColumnViaSplitview(testPage, "right", 400);
    await expectHeaderGeometry(testPage, 28);
    await expectExpandedPRContiguous(testPage);
    for (const width of [1040, 768, 1280]) {
      await testPage.setViewportSize({ width, height: 900 });
      await expectHeaderGeometry(testPage, 28, true);
      await refreshSpacingAndExpectAnchor(testPage);
    }
    await session.clickSessionChatTab();
    await session.clickTab("Changes");
    await resizeColumnViaSplitview(testPage, "right", 400);
    await expectHeaderGeometry(testPage, 28);
    await expectNoPageHorizontalOverflow(testPage);
    await prCapture.screenshot("history-header-spacing-desktop", {
      caption: "Compact Changes history headers",
    });
  });

  test("preserves residual history spacing after compact headers", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await openHistoryRegression(testPage, apiClient, seedData, false);
    await seedHistoryRelation(testPage, "diverged");
    await expectDivergedHistory(testPage);
    const prToggle = testPage.getByTestId("pr-changes-section-collapse-toggle");
    if ((await prToggle.getAttribute("aria-expanded")) === "false") await prToggle.click();
    await expect(
      testPage.locator('[data-testid="pr-files-section"] [data-changes-file]'),
    ).toHaveCount(5);

    await expect
      .poll(() => measurePRSectionGeometry(testPage))
      .toEqual({
        siblingGaps: [2, 2, 2, 2],
        sectionGap: expect.closeTo(10, 0),
        contentOffset: expect.closeTo(-4, 0),
      });
  });

  // @covers AC-UI-BOUNDED-CHANGES-001.8
  test("keeps phone header hit targets with a fine pointer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    const session = await openHistoryRegression(testPage, apiClient, seedData, true);
    await seedHistoryRelation(testPage, "diverged");
    await expectDivergedHistory(testPage);
    await expectHeaderGeometry(testPage, 44);
    await seedHistoryRelation(testPage, "diverged", ["frontend", "backend"]);
    await testPage.getByTestId("local-checkout-commits-section-collapse-toggle").click();
    await expectRepositoryToggleTouchTarget(testPage, "frontend");
    await prCapture.screenshot("history-repository-targets-fine-pointer-phone", {
      caption: "44px repository controls on a fine-pointer phone",
    });
    await testPage.getByTestId("local-checkout-commits-section-collapse-toggle").click();
    await testPage.setViewportSize({ width: 767, height: 851 });
    await expectHeaderGeometry(testPage, 44);
    await testPage.setViewportSize({ width: 768, height: 900 });
    await session.clickTab("Changes");
    await expectHeaderGeometry(testPage, 28);
  });

  // @covers AC-UI-BOUNDED-CHANGES-001.8
  test("keeps touch header targets in the desktop workbench", async ({
    coarseDesktopTestPage: testPage,
    apiClient,
    seedData,
  }) => {
    await openHistoryRegression(testPage, apiClient, seedData, false);
    await seedHistoryRelation(testPage, "diverged");
    await expectDivergedHistory(testPage);
    await expectHeaderGeometry(testPage, 44);
    await testPage.setViewportSize({ width: 1040, height: 900 });
    await expectHeaderGeometry(testPage, 44, true);
    await refreshSpacingAndExpectAnchor(testPage);
  });
});
