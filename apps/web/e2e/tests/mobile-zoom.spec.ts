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
    // Split into tokens so the assertion pins the exact value — a plain
    // substring check would also pass for a typo like `maximum-scale=10`.
    const tokens = content?.split(",").map((token) => token.trim()) ?? [];
    expect(tokens).toContain("maximum-scale=1");
  });

  test("form fields render at >= 16px to prevent iOS focus-zoom", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    // Guard: the 16px rule is gated on `@media (any-pointer: coarse)`. The
    // mobile-chrome project (Pixel 5) emulates touch, so the testPage context
    // exposes a coarse pointer. Assert it here so a future fixture change that
    // drops touch emulation fails loudly instead of silently skipping the rule
    // under test.
    const coarse = await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches);
    expect(coarse).toBe(true);

    await mobile.openSearch();
    const input = mobile.searchInput();
    await input.focus();

    const fontSize = await input.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(fontSize).toBeGreaterThanOrEqual(16);
  });
});
