import { test, expect } from "../../fixtures/test-base";

// Below `md` there is no sidebar, so `/settings` is the settings list itself —
// the same tree the nav sheet shows, rendered as a real route rather than an
// overlay the app has to open for you.
test.describe("Settings index on a phone", () => {
  test("renders the settings tree as the page and navigates from it", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings");

    const index = testPage.getByTestId("settings-index");
    await expect(index).toBeVisible();
    // Stays on /settings: no desktop-style handoff, and nothing to go Back past.
    await expect(testPage).toHaveURL(/\/settings$/);

    await index.getByRole("link", { name: /Terminal/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/terminal$/);
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
  });

  test("keeps the nav sheet available on a leaf for sideways jumps", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/general/terminal");

    await testPage.getByTestId("app-nav-trigger").click();
    const sheet = testPage.getByTestId("app-nav-sheet");
    await expect(sheet).toBeVisible();

    await sheet.getByRole("link", { name: /Notifications/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/notifications$/);
  });
});
