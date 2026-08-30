import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  resetForkPRComparisonRepository,
  seedForkPRComparisonTask,
} from "./fork-pr-comparison-target-helpers";

test.describe("Fork pull-request comparison target", () => {
  test.describe.configure({ timeout: 120_000 });

  test.afterEach(async ({ seedData, backend }) => {
    resetForkPRComparisonRepository(seedData, backend);
  });

  test("uses the upstream target for one fork commit and three files", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const { task } = await seedForkPRComparisonTask(apiClient, seedData, backend);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 15_000 });

    await session.expandCommitsSection();
    await expect(session.commitsSection().locator('[data-testid^="commit-row-"]')).toHaveCount(1);
    await expect(session.commitsSection()).toContainText("Add three fork contribution files");

    await session.expandPRChangesSection();
    await expect(
      session.prFilesSection().locator('[data-changes-file="fork-one.txt"]'),
    ).toBeVisible();
    await expect(
      session.prFilesSection().locator('[data-changes-file="fork-two.txt"]'),
    ).toBeVisible();
    await expect(
      session.prFilesSection().locator('[data-changes-file="fork-three.txt"]'),
    ).toBeVisible();

    const branchDetails = testPage.getByRole("button", {
      name: "Show branch and Git credential details",
    });
    await branchDetails.hover();
    await expect(testPage.getByTestId("changes-comparison-target")).toContainText(
      "upstream/widget:main",
    );
    await expect(testPage.getByTestId("comparison-target-notice")).toHaveCount(0);
  });

  test("shows an unavailable state when the upstream target cannot be fetched", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const { task } = await seedForkPRComparisonTask(apiClient, seedData, backend, {
      comparisonTargetAvailable: false,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("comparison-target-notice")).toContainText(
      "upstream/widget:main",
    );
  });
});
