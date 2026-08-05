import { test, expect } from "../../fixtures/test-base";

// `/settings` used to render the same card grid as `/settings/general`. On
// desktop it now hands off to the settings page this device was last on, because
// the sidebar already shows the settings tree.
test.describe("Settings index on desktop", () => {
  test("opens the default page on a device with no settings history", async ({ testPage }) => {
    await testPage.goto("/settings");

    await expect(testPage).toHaveURL(/\/settings\/general\/appearance$/);
    await expect(testPage.getByTestId("theme-settings-card")).toBeVisible();
  });

  test("returns to the page you were last on", async ({ testPage }) => {
    await testPage.goto("/settings/general/terminal");
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();

    await testPage.goto("/settings");

    await expect(testPage).toHaveURL(/\/settings\/general\/terminal$/);
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
  });

  test("does not leave /settings in history for Back to land on", async ({ testPage }) => {
    await testPage.goto("/settings/prompts");
    // The visit is recorded once the settings shell mounts; navigating away
    // inside the same frame would race that.
    await expect(testPage.getByTestId("settings-scroll-container")).toBeVisible();

    await testPage.goto("/settings");
    await expect(testPage).toHaveURL(/\/settings\/prompts$/);

    // A `push` here would send Back to /settings, which would redirect forward
    // again and trap the user.
    await testPage.goBack();

    await expect(testPage).not.toHaveURL(/\/settings$/);
  });

  test("redirects the /settings/general prefix to its first page", async ({ testPage }) => {
    await testPage.goto("/settings/general");

    await expect(testPage).toHaveURL(/\/settings\/general\/appearance$/);
  });

  test("keeps a record-scoped page out of the restore slot", async ({ testPage, seedData }) => {
    // Workspace pages carry an id that may not exist next time; restoring one
    // would land on a broken page, so the last id-free page wins.
    await testPage.goto("/settings/prompts");
    await expect(testPage.getByTestId("settings-scroll-container")).toBeVisible();

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);
    await expect(testPage.getByTestId("settings-scroll-container")).toBeVisible();

    await testPage.goto("/settings");

    await expect(testPage).toHaveURL(/\/settings\/prompts$/);
  });
});
