import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

/**
 * Mobile parity for the desktop sidebar footer's theme toggle. The footer is
 * desktop-only (`hidden md:block`), so on a phone viewport dark mode used to
 * be entirely unreachable — the hamburger sheet's Utilities section had no
 * theme control at all. This proves the toggle is reachable there and that a
 * tap actually flips the app's resolved theme (not just that a row renders).
 */
test.describe("Theme toggle on mobile", () => {
  test("exposes a theme toggle in the hamburger sheet and flips the resolved theme", async ({
    testPage,
  }) => {
    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();

    // Fresh session defaults to the "system" theme, which resolves to light
    // in a headless Chromium profile with no forced color scheme.
    const html = testPage.locator("html");
    await expect(html).toHaveClass(/(^|\s)light(\s|$)/);

    await kanban.mobileMenuButton.click();

    const sheet = testPage.getByRole("dialog");
    const themeToggle = sheet.getByTestId("mobile-theme-toggle-button");
    await expect(themeToggle).toBeVisible({ timeout: 10_000 });

    await themeToggle.click();

    // The real downstream effect: AppThemeProvider re-applies the resolved
    // theme class on <html>, not just local component state.
    await expect(html).toHaveClass(/(^|\s)dark(\s|$)/);
    await expect(html).not.toHaveClass(/(^|\s)light(\s|$)/);

    // The sheet stays open (unlike the navigation rows above it, this is a
    // persistent toggle, not a navigate-and-close action).
    await expect(sheet).toBeVisible();

    await themeToggle.click();
    await expect(html).toHaveClass(/(^|\s)light(\s|$)/);
    await expect(html).not.toHaveClass(/(^|\s)dark(\s|$)/);
  });
});
