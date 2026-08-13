import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

/** Swipe up on a scroll container — the touch equivalent of wheel-down at scroll bottom. */
async function swipeUpOnElement(page: Page, element: Locator): Promise<void> {
  const box = await element.boundingBox();
  if (!box) throw new Error("scroll container has no bounding box");

  const cdp = await page.context().newCDPSession(page);
  const centerX = box.x + box.width / 2;
  const startY = box.y + box.height - 20;
  const endY = box.y + 20;

  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: centerX, y: startY }],
  });
  for (let i = 1; i <= 8; i++) {
    const y = startY + ((endY - startY) * i) / 8;
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: centerX, y }],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchEnd",
    touchPoints: [],
  });
}

test.describe("Automations settings on mobile", () => {
  test("create page does not hand off bottom overscroll to the document", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    const settingsScroller = testPage.getByTestId("settings-scroll-container");
    await expect(settingsScroller).toBeVisible();
    await expect(settingsScroller).toHaveCSS("overscroll-behavior-y", "contain");

    await settingsScroller.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    await swipeUpOnElement(testPage, settingsScroller);

    await expect.poll(() => testPage.evaluate(() => window.scrollY), { timeout: 5_000 }).toBe(0);
  });

  test("delete all in a status view removes only that view's runs on mobile", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // The delete-all control now lives in the table header's rightmost cell;
    // on a phone viewport that position must stay reachable and scoped to
    // the active status filter.
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Mobile Filtered Delete",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedAutomationRun(automation.id, "skipped");
    await apiClient.seedAutomationRun(automation.id, "skipped");
    await apiClient.seedAutomationRun(automation.id, "succeeded");

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    const scrollContainer = testPage.getByTestId("settings-scroll-container");
    await scrollContainer.evaluate((el) => (el.scrollTop = el.scrollHeight));

    const recentRunsButton = testPage.locator("button", { hasText: /Recent Runs/ });
    await recentRunsButton.waitFor({ state: "visible", timeout: 10_000 });
    await recentRunsButton.click();

    const tbody = testPage.locator("table tbody");
    await tbody.waitFor({ state: "visible", timeout: 5_000 });
    await expect(tbody.locator("tr")).toHaveCount(3, { timeout: 10_000 });

    // Filter to Skipped, then delete all from the table header.
    await testPage.getByTestId("run-filter-skipped").click();
    await expect(tbody.locator("tr")).toHaveCount(2, { timeout: 5_000 });

    const deleteAll = testPage.locator("table thead").getByTestId("delete-all-runs");
    await expect(deleteAll).toBeVisible();
    await deleteAll.click();
    await expect(
      testPage.getByText(/permanently remove the Skipped runs shown in this view/),
    ).toBeVisible();
    await testPage.getByTestId("delete-all-runs-confirm").click();

    // Only the skipped runs are gone; the succeeded run survives.
    await expect(tbody.locator("tr")).toHaveCount(1, { timeout: 5_000 });
    await testPage.getByTestId("run-filter-all").click();
    await expect(tbody.locator("tr")).toHaveCount(1, { timeout: 5_000 });
  });
});
