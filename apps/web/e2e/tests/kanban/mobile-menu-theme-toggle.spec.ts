import { devices, type Browser } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

/**
 * Mobile parity for the desktop sidebar footer's theme toggle. The footer is
 * desktop-only (`hidden md:block`), so on a phone viewport there used to be no
 * *quick* way to flip dark mode — Settings still has the theme select, but
 * the hamburger sheet's Utilities section had no one-tap control. This proves
 * the toggle is reachable there and that a tap actually flips the app's
 * resolved theme (not just that a row renders).
 */

/**
 * A second mobile client with the OS/browser color scheme forced to dark, so
 * a fresh session's `theme: "system"` resolves to `resolvedTheme: "dark"`
 * before any tap. This is the state where a toggle keyed off `theme` instead
 * of `resolvedTheme` looks correct while actually being a no-op on first tap.
 */
async function openDarkModeMobileContext(browser: Browser, backend: BackendContext) {
  const context = await browser.newContext({
    ...devices["Pixel 5"],
    baseURL: backend.frontendUrl,
    colorScheme: "dark",
  });
  const page = await context.newPage();
  await page.addInitScript(
    ({ backendPort }: { backendPort: string }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      window.__KANDEV_API_PORT = backendPort;
      window.__KANDEV_E2E_EXPOSE_STORE__ = true;
    },
    { backendPort: String(backend.port) },
  );
  return { page, context };
}

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

    await kanban.mobileMenuButton.tap();

    const sheet = testPage.getByRole("dialog");
    const themeToggle = sheet.getByTestId("mobile-theme-toggle-button");
    await expect(themeToggle).toBeVisible({ timeout: 10_000 });
    await expect(themeToggle).toHaveAttribute("aria-label", "Switch to Dark Mode");
    await expect(themeToggle).toHaveAttribute("aria-pressed", "false");

    await themeToggle.tap();

    // The real downstream effect: AppThemeProvider re-applies the resolved
    // theme class on <html>, not just local component state.
    await expect(html).toHaveClass(/(^|\s)dark(\s|$)/);
    await expect(html).not.toHaveClass(/(^|\s)light(\s|$)/);
    await expect(themeToggle).toHaveAttribute("aria-label", "Switch to Light Mode");
    await expect(themeToggle).toHaveAttribute("aria-pressed", "true");

    // The sheet stays open (unlike the navigation rows above it, this is a
    // persistent toggle, not a navigate-and-close action).
    await expect(sheet).toBeVisible();

    await themeToggle.tap();
    await expect(html).toHaveClass(/(^|\s)light(\s|$)/);
    await expect(html).not.toHaveClass(/(^|\s)dark(\s|$)/);
    await expect(themeToggle).toHaveAttribute("aria-label", "Switch to Dark Mode");
    await expect(themeToggle).toHaveAttribute("aria-pressed", "false");
  });

  test("flips out of dark mode on the first tap when the OS already prefers dark", async ({
    testPage: _testPage,
    browser,
    backend,
  }) => {
    // Depend on testPage so its per-test backend and settings reset runs
    // before this specialized dark-mode context is created.
    const { page, context } = await openDarkModeMobileContext(browser, backend);
    try {
      const kanban = new MobileKanbanPage(page);
      await kanban.goto();

      // theme is still "system", but resolvedTheme is already "dark" because
      // the browser's color scheme is forced dark — exactly the state the
      // buggy `theme === "dark"` comparison could not distinguish from light.
      const html = page.locator("html");
      await expect(html).toHaveClass(/(^|\s)dark(\s|$)/);

      await kanban.mobileMenuButton.tap();
      const sheet = page.getByRole("dialog");
      const themeToggle = sheet.getByTestId("mobile-theme-toggle-button");
      await expect(themeToggle).toBeVisible({ timeout: 10_000 });
      await expect(themeToggle).toHaveAttribute("aria-label", "Switch to Light Mode");
      await expect(themeToggle).toHaveAttribute("aria-pressed", "true");

      await themeToggle.tap();

      // One tap must be enough to leave dark mode. A toggle keyed off the raw
      // `theme` value ("system") instead of `resolvedTheme` ("dark") would
      // call setTheme("dark") here and this assertion would time out.
      await expect(html).toHaveClass(/(^|\s)light(\s|$)/);
      await expect(html).not.toHaveClass(/(^|\s)dark(\s|$)/);
      await expect(themeToggle).toHaveAttribute("aria-label", "Switch to Dark Mode");
      await expect(themeToggle).toHaveAttribute("aria-pressed", "false");
    } finally {
      await context.close();
    }
  });
});
