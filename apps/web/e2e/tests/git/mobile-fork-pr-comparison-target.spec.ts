import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedForkPRComparisonTask } from "./fork-pr-comparison-target-helpers";

async function expandSection(toggle: Locator) {
  await expect
    .poll(
      async () => {
        if ((await toggle.getAttribute("aria-expanded")) === "true") return true;
        await toggle.tap();
        return (await toggle.getAttribute("aria-expanded")) === "true";
      },
      { timeout: 15_000 },
    )
    .toBe(true);
}

test.describe("Mobile fork pull-request comparison target", () => {
  test.describe.configure({ timeout: 120_000 });

  test("shows the same upstream target in the touch branch drawer", async ({
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
    await expect(testPage.getByRole("button", { name: "Changes" })).toBeVisible({
      timeout: 15_000,
    });
    await testPage.getByRole("button", { name: "Changes" }).tap();

    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes).toBeVisible({ timeout: 15_000 });
    const commitsToggle = changes.getByTestId("commits-section-collapse-toggle");
    await expect(commitsToggle).toBeVisible({ timeout: 45_000 });
    await expandSection(commitsToggle);
    await expect(changes.locator('[data-testid^="commit-row-"]')).toHaveCount(1);
    const prToggle = changes.getByTestId("pr-changes-section-collapse-toggle");
    await expect(prToggle).toBeVisible();
    await expandSection(prToggle);
    const prFiles = changes.getByTestId("pr-files-list");
    await expect(prFiles).toBeVisible({ timeout: 20_000 });
    await expect(prFiles.locator('[data-changes-file="fork-one.txt"]')).toBeVisible();
    await expect(prFiles.locator('[data-changes-file="fork-two.txt"]')).toBeVisible();
    await expect(prFiles.locator('[data-changes-file="fork-three.txt"]')).toBeVisible();

    await testPage.getByRole("button", { name: "Show branch and Git credential details" }).tap();
    const drawer = testPage.getByRole("dialog", { name: "Branch details" });
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("changes-comparison-target")).toContainText(
      "upstream/widget:main",
    );
    await expect(testPage.getByTestId("comparison-target-notice")).toHaveCount(0);
  });

  test("shows the unavailable target in the touch changes panel", async ({
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
    await expect(testPage.getByRole("button", { name: "Changes" })).toBeVisible({
      timeout: 15_000,
    });
    await testPage.getByRole("button", { name: "Changes" }).tap();

    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes).toBeVisible({ timeout: 15_000 });
    await expect(changes.getByTestId("comparison-target-notice")).toContainText(
      "upstream/widget:main",
    );
  });
});
