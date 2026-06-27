import { test, expect } from "../fixtures/test-base";
import { MobileKanbanPage } from "../pages/mobile-kanban-page";

// Runs on the mobile-chrome Playwright project (Pixel 5 emulation, touch →
// any-pointer: coarse) — see e2e/playwright.config.ts. Guards the iOS focus-zoom
// fix: the 16px coarse-pointer form-control rule (globals.css) that stops Safari
// from auto-zooming when a sub-16px field is focused.
test.describe("Mobile zoom hardening", () => {
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
