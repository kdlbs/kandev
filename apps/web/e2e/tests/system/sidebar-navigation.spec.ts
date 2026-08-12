import { test, expect } from "../../fixtures/test-base";

const SYSTEM_ENTRIES: Array<{ href: string; label: string; title: string }> = [
  { href: "/settings/system/status", label: "Status", title: "Status" },
  { href: "/settings/system/data-storage", label: "Data & Logs", title: "Data & Logs" },
  { href: "/settings/system/feature-toggles", label: "Feature Toggles", title: "Feature Toggles" },
  { href: "/settings/system/updates", label: "Updates", title: "Updates" },
  { href: "/settings/system/about", label: "About", title: "About" },
];

test.describe("System sidebar navigation", () => {
  test("System section has exactly its page rows, which navigate correctly, and no standalone Changelog entry remains", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);

    // In the unified AppSidebar the settings nav is a gear-gated takeover; the
    // menu is static, so every System row is visible from any settings page.
    await testPage.goto("/settings/system/status");
    await expect(testPage.getByTestId("app-sidebar-settings-mode")).toBeVisible();

    // Each row is present in the settings sidebar.
    for (const entry of SYSTEM_ENTRIES) {
      const link = testPage.locator(`a[href="${entry.href}"]`).first();
      await expect(link).toBeVisible();
    }

    // Standalone Changelog entry is NOT present, and neither are the merged
    // pages' old rows (Database, Backups, Storage, Logs, Licenses).
    await expect(testPage.locator('a[href="/settings/changelog"]')).toHaveCount(0);
    for (const gone of [
      "/settings/system/database",
      "/settings/system/backups",
      "/settings/system/storage",
      "/settings/system/logs",
      "/settings/system/licenses",
    ]) {
      await expect(testPage.locator(`a[href="${gone}"]`)).toHaveCount(0);
    }

    // Click each entry and confirm the URL + page title.
    for (const entry of SYSTEM_ENTRIES) {
      await testPage.locator(`a[href="${entry.href}"]`).first().click();
      await expect(testPage).toHaveURL((url) => new URL(url).pathname === entry.href, {
        timeout: 10_000,
      });
      await expect(testPage.getByTestId("system-page-title")).toHaveText(entry.title, {
        timeout: 10_000,
      });
    }
  });
});
