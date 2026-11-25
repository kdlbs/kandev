import { test, expect } from "../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { AutomationsPage } from "../pages/automations-page";

test.use({ trace: "retain-on-failure" });

async function expectInsideViewport(page: Page, locator: Locator) {
  await locator.scrollIntoViewIfNeeded();
  const viewport = page.viewportSize();
  const box = await locator.boundingBox();
  expect(viewport).not.toBeNull();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport!.width);
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

test.describe("GitHub PR follow-up automation on mobile", () => {
  test("configures follow-up events and per-PR continuation with touch", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const name = "Mobile PR follow-up";
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name,
      workflowId: seedData.workflowId,
      taskMode: "normal_task",
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "github_pr",
      config: { events: ["opened"], repos: [{ owner: "acme", name: "widget" }] },
      enabled: true,
    });
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/automations/${automation.id}`);
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    await testPage
      .getByRole("button", { name: /PR: Opened/ })
      .first()
      .tap();
    const card = testPage.getByTestId("trigger-card-github_pr");
    const newCommits = card.locator("label").filter({ hasText: "New commits" });
    const reviewRequested = card.locator("label").filter({ hasText: "Review requested" });
    await expectInsideViewport(testPage, newCommits);
    await expectInsideViewport(testPage, reviewRequested);
    const includeExisting = card
      .locator("label")
      .filter({ hasText: "Also run on pull requests already open" });
    await expectInsideViewport(testPage, includeExisting);
    await includeExisting.tap();
    await newCommits.tap();
    await reviewRequested.tap();
    const quiet = card.getByLabel(/Wait until pushes stop for/);
    await quiet.fill("5");
    await quiet.blur();

    const perPR = testPage.getByRole("radio", { name: "One task per pull request" });
    await expectInsideViewport(testPage, perPR);
    expect((await perPR.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await perPR.tap();
    await expect(perPR).toBeChecked();
    const followUpLabel = testPage.getByText("Follow-up instructions", { exact: true });
    await followUpLabel.scrollIntoViewIfNeeded();
    await expect(followUpLabel).toBeInViewport();
    const step = testPage.getByTestId("pr-continuation-step");
    const merged = testPage.getByTestId("pr-merged-action");
    await expectInsideViewport(testPage, step);
    await expectInsideViewport(testPage, merged);
    await merged.tap();
    const archiveOption = testPage.getByRole("option", { name: "Archive task" });
    // The menu opens with a zoom-in animation; measure once it settles.
    await expect
      .poll(async () => (await archiveOption.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await archiveOption.tap();

    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.tap();
    await automations.waitForSaved();
    await testPage.reload();
    await expect(automations.editor).toBeVisible({ timeout: 15_000 });

    await expect(testPage.getByTestId("pr-merged-action")).toHaveText("Archive task");
    await testPage
      .getByRole("button", { name: /PR: Opened, New commits, Review requested/ })
      .first()
      .tap();
    await expect(
      testPage.getByTestId("trigger-card-github_pr").getByLabel(/Wait until pushes stop for/),
    ).toHaveValue("5");
    await expect(
      testPage
        .getByTestId("trigger-card-github_pr")
        .getByRole("checkbox", { name: "Also run on pull requests already open" }),
    ).toBeChecked();
  });
});
