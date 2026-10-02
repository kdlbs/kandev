import { test, expect } from "../../fixtures/test-base";
import { expectNoPageHorizontalOverflow } from "./large-changes-helpers";
import { refreshSpacingAndExpectAnchor } from "./changes-commit-spacing-helpers";
import {
  openHistoryRegression,
  seedHistoryRelation,
  expectStaleHistory,
  expectDivergedHistory,
  expectHeaderGeometry,
  expectExpandedPRContiguous,
} from "./changes-history-regression-helpers";

test.describe("Mobile Changes history regression", () => {
  test.describe.configure({ retries: 0, timeout: 120_000 });

  // @covers AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4
  test("stale upstream keeps phone history unified", async ({ testPage, apiClient, seedData }) => {
    await openHistoryRegression(testPage, apiClient, seedData, true);
    await seedHistoryRelation(testPage, "stale");
    await expectStaleHistory(testPage);
    await seedHistoryRelation(testPage, "local_ahead");
    await expectStaleHistory(testPage);
  });

  test("confirmed divergence keeps phone comparison available", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openHistoryRegression(testPage, apiClient, seedData, true);
    await seedHistoryRelation(testPage, "diverged");
    await expectDivergedHistory(testPage);
    await testPage.getByTestId("local-checkout-commits-section-collapse-toggle").tap();
    await expect(testPage.getByTestId("commit-row-ccccccc")).toBeVisible();
  });

  // @covers AC-UI-BOUNDED-CHANGES-001.6 and AC-UI-BOUNDED-CHANGES-001.8
  test("keeps collapsed history headers compact on phones", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await openHistoryRegression(testPage, apiClient, seedData, true);
    await seedHistoryRelation(testPage, "diverged");
    await expectDivergedHistory(testPage);
    for (const width of [393, 767]) {
      await testPage.setViewportSize({ width, height: 851 });
      await expectHeaderGeometry(testPage, 44);
      await refreshSpacingAndExpectAnchor(testPage);
      await expectNoPageHorizontalOverflow(testPage);
    }
    await testPage.setViewportSize({ width: 393, height: 851 });
    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    await expectExpandedPRContiguous(testPage);
    await expect(testPage.getByTestId("changes-panel-scroll-owner")).toHaveCount(1);
    await prCapture.screenshot("history-header-spacing-phone", {
      caption: "Touch-sized Changes history headers",
    });
  });
});
