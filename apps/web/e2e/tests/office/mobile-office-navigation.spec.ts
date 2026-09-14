import { test, expect } from "../../fixtures/office-fixture";
import type { ApiClient } from "../../helpers/api-client";

// Regression coverage for the PageShell migration: Office used to render a
// title-only topbar with no menu and no home link, so on a phone the entire
// Office section was unreachable and un-exitable (browser back excepted).
test.describe("Office mobile navigation", () => {
  let baseline: Awaited<ReturnType<ApiClient["getUserSettings"]>>["settings"];
  let createdWorkspace: { id: string; name: string } | undefined;
  test.beforeEach(async ({ testPage, apiClient }) => {
    void testPage;
    createdWorkspace = undefined;
    baseline = (await apiClient.getUserSettings()).settings;
    await apiClient.saveUserSettings({ startup_page: "threads" });
  });
  test.afterEach(async ({ apiClient }) => {
    if (baseline)
      await apiClient.saveUserSettings({
        startup_page: baseline.startup_page ?? "task_overview",
        workspace_id: baseline.workspace_id,
      });
    if (createdWorkspace)
      await apiClient.deleteWorkspace(createdWorkspace.id, createdWorkspace.name);
  });
  test("offers office sections and a home row in the shared nav sheet", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/office/tasks");

    const trigger = testPage.getByTestId("app-nav-trigger");
    await expect(trigger).toBeVisible();
    await trigger.tap();

    const sheet = testPage.getByTestId("app-nav-sheet");
    await expect(sheet).toBeVisible();

    // Office's own sections render above the global destinations.
    await expect(sheet.getByRole("link", { name: /Routines/ })).toBeVisible();
    await expect(sheet.getByRole("link", { name: /Skills/ })).toBeVisible();
    // Global destinations come from the navigation manifest.
    await expect(sheet.getByRole("link", { name: "Stats" })).toBeVisible();
    await expect(sheet.getByRole("link", { name: "Settings" })).toBeVisible();

    // Home from inside Office lands on the office dashboard, not kanban.
    await sheet.getByRole("link", { name: "Home", exact: true }).tap();
    await expect(sheet).not.toBeVisible();
    await expect(testPage).toHaveURL(/\/office(?:\?.*)?$/);

    await testPage.goto("/settings/preferences/appearance");
    await expect(testPage.getByRole("radio", { name: "Threads", exact: true })).toBeChecked();
    await testPage.getByRole("link", { name: "Home", exact: true }).tap();
    await expect(testPage).toHaveURL(/\/office(?:\?.*)?$/);
  });

  test("navigates between office sections from the sheet", async ({ testPage, officeSeed: _ }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/office/tasks");

    await testPage.getByTestId("app-nav-trigger").click();
    const sheet = testPage.getByTestId("app-nav-sheet");
    await sheet.getByRole("link", { name: /Routines/ }).click();

    await expect(sheet).not.toBeVisible();
    await expect(testPage).toHaveURL(/\/office\/routines$/);
  });

  test("switches workspaces from the Office mobile menu", async ({ testPage, apiClient }) => {
    const kanbanWorkspace = await apiClient.createWorkspace("Mobile Office Kanban Workspace");
    createdWorkspace = kanbanWorkspace;

    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/office/tasks");
    await testPage.getByTestId("app-nav-trigger").click();

    const sheet = testPage.getByTestId("app-nav-sheet");
    const workspaceTrigger = sheet.getByTestId("mobile-office-workspace-trigger");
    await expect(workspaceTrigger).toContainText("E2E Workspace");
    await workspaceTrigger.click();
    await testPage.getByTestId(`mobile-office-workspace-item-${kanbanWorkspace.id}`).click();

    await expect(sheet).not.toBeVisible();
    await expect(testPage).toHaveURL(
      (url) =>
        url.pathname === "/threads" && url.searchParams.get("workspace") === kanbanWorkspace.id,
      { timeout: 10_000 },
    );
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.6
  test("keeps phone Home in the workspace's Threads when Office is disabled", async ({
    testPage,
    backend,
    officeSeed,
  }, testInfo) => {
    try {
      await backend.restart({ KANDEV_FEATURES_OFFICE: "false" });
      await testPage.goto(`/?home=overview&workspaceId=${officeSeed.workspaceId}`);
      await testPage.getByTestId("mobile-topbar-menu").tap();
      const home = testPage
        .getByRole("dialog", { name: "Menu" })
        .getByRole("link", { name: "Home", exact: true });
      await expect(home).toHaveAttribute("href", `/threads?workspace=${officeSeed.workspaceId}`);
      await home.tap();
      await expect(testPage).toHaveURL(
        (url) =>
          url.pathname === "/threads" &&
          url.searchParams.get("workspace") === officeSeed.workspaceId,
      );
      const threads = testPage
        .getByTestId("threads-board")
        .or(testPage.getByTestId("threads-empty-state"));
      await expect(threads).toBeVisible();
      await testPage.reload();
      await expect(threads).toBeVisible();
      await testPage.screenshot({ path: testInfo.outputPath("office-disabled-threads-phone.png") });
    } finally {
      await backend.restart();
    }
  });
});
