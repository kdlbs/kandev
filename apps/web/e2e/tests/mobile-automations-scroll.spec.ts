import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

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
    await testPage.touchscreen.tap(200, 600);

    await expect.poll(() => testPage.evaluate(() => window.scrollY), { timeout: 5_000 }).toBe(0);
  });
});
