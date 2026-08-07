import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

/**
 * Mobile reachability for `/stats`. The route's only entry points used to be the
 * desktop-only sidebar footer (`hidden md:block`) and a keyboard-only command
 * palette entry, so on a phone the page could only be reached by typing the URL.
 * It is now a navigation-manifest destination (`lib/navigation/core-destinations.ts`),
 * which puts it in the hamburger sheet's utility group.
 */
test.describe("Stats on mobile", () => {
  test("exposes Stats in the hamburger sheet and navigates", async ({ testPage }) => {
    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();

    await kanban.mobileMenuButton.click();

    // Scope to the open menu sheet so we never match the display:none desktop
    // sidebar, which carries the same "Stats" accessible name.
    const sheet = testPage.getByRole("dialog");
    const statsLink = sheet.getByRole("link", { name: "Stats" });
    await expect(statsLink).toBeVisible({ timeout: 10_000 });
    expect(await statsLink.getAttribute("href")).toBe("/stats");

    await statsLink.click();

    await expect(testPage).toHaveURL(/\/stats$/);
    await expect(sheet).toBeHidden();
    // Assert content unique to the rendered stats page. The "Statistics" topbar
    // title also renders for the no-workspace and error states, so it would pass
    // on a page that never loaded.
    await expect(testPage.getByRole("button", { name: "Copy Stats" })).toBeVisible({
      timeout: 15_000,
    });
  });
});
