import { test, expect } from "../fixtures/test-base";
import { MobileKanbanPage } from "../pages/mobile-kanban-page";

// Runs on the mobile-chrome Playwright project (Pixel 5 emulation, touch →
// pointer: coarse) — see e2e/playwright.config.ts. Guards the two iOS-zoom
// fixes: the 16px coarse-pointer form-control rule (globals.css) that stops
// focus-zoom, and the maximum-scale=1 viewport meta (index.html) that blocks
// casual page pinch-zoom.
test.describe("Mobile zoom hardening", () => {
  test("viewport meta pins maximum-scale=1", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const content = await testPage.locator('meta[name="viewport"]').getAttribute("content");
    expect(content).toContain("maximum-scale=1");
  });

  test("form fields render at >= 16px to prevent iOS focus-zoom", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    await mobile.openSearch();
    const input = mobile.searchInput();
    await input.focus();

    const fontSize = await input.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(fontSize).toBeGreaterThanOrEqual(16);
  });
});
