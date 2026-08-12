import { test, expect } from "../../fixtures/test-base";

// `/settings` used to render the same card grid as `/settings/general`. On
// desktop it now hands off to the settings page this device was last on, because
// the sidebar already shows the settings tree.
test.describe("Settings index on desktop", () => {
  test("opens the default page on a device with no settings history", async ({ testPage }) => {
    await testPage.goto("/settings");

    await expect(testPage).toHaveURL(/\/settings\/preferences\/appearance$/);
    await expect(testPage.getByTestId("theme-settings-card")).toBeVisible();
  });

  test("returns to the page you were last on", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/terminal-editors");
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();

    await testPage.goto("/settings");

    await expect(testPage).toHaveURL(/\/settings\/preferences\/terminal-editors$/);
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

  test("hands off when a phone-width window grows past the sidebar boundary", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings");
    await expect(testPage.getByTestId("settings-index")).toBeVisible();

    // The sidebar appears at md and brings its own copy of this tree; two
    // identical menus side by side is what this route exists to avoid.
    await testPage.setViewportSize({ width: 1280, height: 860 });

    await expect(testPage).toHaveURL(/\/settings\/preferences\/appearance$/);
    await expect(testPage.getByTestId("settings-index")).toHaveCount(0);
  });

  test("marks only the active row, never the static section header", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/appearance");

    const takeover = testPage.getByTestId("app-sidebar-settings-mode");
    await expect(takeover.getByRole("link", { name: "Appearance" })).toBeVisible();

    // Section headers are static text, not links, so exactly one row can claim
    // the current location.
    const active = takeover.locator("a[data-active='true']");
    await expect(active).toHaveCount(1);
    await expect(active).toHaveAccessibleName(/Appearance/);
  });

  test("owns Voice Mode inside Task Behavior", async ({ testPage, prCapture }) => {
    await testPage.goto("/settings/preferences/task-behavior");

    const settingsNav = testPage.getByTestId("app-sidebar-settings-mode");
    await expect(
      settingsNav.getByRole("link", { name: "Task Behavior", exact: true }),
    ).toBeVisible();
    await expect(settingsNav.getByRole("link", { name: "Voice Mode", exact: true })).toHaveCount(0);
    await expect(testPage.getByRole("heading", { name: "Voice Mode", exact: true })).toBeVisible();
    await expect(testPage.getByTestId("voice-enable-card")).toBeVisible();
    await expect(testPage.getByText("Transcription Engine", { exact: true })).toBeVisible();
    await expect(testPage.getByText("Behavior", { exact: true })).toBeVisible();
    await expect(testPage.getByText("Whisper Web Model", { exact: true })).toBeVisible();
    await expect(testPage.getByText(/Shortcut$/, { exact: true })).toBeVisible();
    await testPage
      .getByRole("heading", { name: "Voice Mode", exact: true })
      .scrollIntoViewIfNeeded();
    await prCapture.screenshot("task-behavior-voice-mode-desktop", {
      caption: "Desktop Task Behavior page with Voice Mode settings",
    });
  });

  test("redirects the /settings/general prefix to the Appearance page", async ({ testPage }) => {
    await testPage.goto("/settings/general");

    await expect(testPage).toHaveURL(/\/settings\/preferences\/appearance$/);
  });

  test("keeps a record-scoped page out of the restore slot", async ({ testPage, seedData }) => {
    // Workspace pages carry an id that may not exist next time; restoring one
    // would land on a broken page, so the last id-free page wins.
    await testPage.goto("/settings/prompts");
    await expect(testPage.getByTestId("settings-scroll-container")).toBeVisible();

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await expect(testPage.getByTestId("settings-scroll-container")).toBeVisible();

    await testPage.goto("/settings");

    await expect(testPage).toHaveURL(/\/settings\/prompts$/);
  });
});
