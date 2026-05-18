import { test, expect } from "../../fixtures/test-base";

const SYSTEM_ENTRIES: Array<{ href: string; label: string; title: string }> = [
  { href: "/settings/system/status", label: "Status", title: "Status" },
  { href: "/settings/system/database", label: "Database", title: "Database" },
  { href: "/settings/system/backups", label: "Backups", title: "Backups" },
  { href: "/settings/system/logs", label: "Logs", title: "Logs" },
  { href: "/settings/system/updates", label: "Updates", title: "Updates" },
  { href: "/settings/system/licenses", label: "Licenses", title: "Licenses" },
  { href: "/settings/system/about", label: "About", title: "About" },
];

test.describe("System sidebar navigation", () => {
  test("System group has 7 sub-entries that navigate correctly and no standalone Changelog entry remains", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/settings/general/notifications");

    // Each sub-entry is present in the settings sidebar.
    for (const entry of SYSTEM_ENTRIES) {
      const link = testPage.locator(`a[href="${entry.href}"]`).first();
      await expect(link).toBeVisible();
    }

    // Standalone Changelog entry is NOT present.
    await expect(testPage.locator('a[href="/settings/changelog"]')).toHaveCount(0);

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
