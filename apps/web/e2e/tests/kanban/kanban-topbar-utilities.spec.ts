import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { missingGitHealth } from "./health-fixtures";

test.describe("Kanban topbar utilities", () => {
  // Post-overhaul: Settings is no longer a topbar link/dropdown. It lives behind
  // the AppSidebar footer gear, which toggles a full-height settings takeover
  // (the settings tree). From there each leaf navigates to a /settings/... page.
  test("settings is reachable from the AppSidebar footer gear", async ({ testPage }) => {
    // Lock to desktop width — the AppSidebar is desktop-only (`hidden md:flex`).
    await testPage.setViewportSize({ width: 1280, height: 800 });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const sidebar = testPage.getByTestId("app-sidebar");
    await expect(sidebar).toBeVisible();

    // The footer gear toggles the settings takeover (the settings tree).
    await sidebar.getByTestId("sidebar-settings-gear").click();
    const settingsMode = testPage.getByTestId("app-sidebar-settings-mode");
    await expect(settingsMode).toBeVisible();

    // Clicking a settings leaf navigates to its /settings/... page.
    await settingsMode.getByRole("link", { name: "Automations" }).click();
    await expect(testPage).toHaveURL(/\/settings\//);
  });

  test("system health button is hidden when there are no issues", async ({ testPage, backend }) => {
    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ healthy: true, issues: [] }),
      }),
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // The button only renders when there are issues; assert it stays hidden.
    await expect(testPage.getByRole("button", { name: "Setup Issues" })).toHaveCount(0);
  });

  test("system health button is visible when there are issues", async ({ testPage, backend }) => {
    // Lock to desktop width — on mobile the health button lives inside the closed hamburger sheet.
    await testPage.setViewportSize({ width: 1280, height: 800 });

    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          healthy: false,
          issues: [{ title: "DB offline", severity: "error" }],
        }),
      }),
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(testPage.getByRole("button", { name: "Setup Issues" })).toBeVisible();
  });

  test("inotify health issue appears in the kanban health dialog", async ({
    testPage,
    backend,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 800 });

    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          healthy: false,
          issues: [
            {
              id: "os_inotify_instances_high",
              category: "system_resources",
              title: "Inotify instances limit nearly exhausted",
              message:
                "123/128 instances in use (96%). Exhaustion causes new terminals, dev servers, and agent CLIs to fail or hang. To increase: sudo sysctl -w fs.inotify.max_user_instances=1024",
              severity: "error",
              fix_url: "/settings/system/status",
              fix_label: "View system status",
            },
          ],
          checks: [],
        }),
      }),
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const issueButton = testPage.getByRole("button", { name: "Setup Issues" });
    await expect(issueButton).toBeVisible();
    await issueButton.click();

    await expect(testPage.getByText("Inotify instances limit nearly exhausted")).toBeVisible();
    await expect(testPage.getByRole("button", { name: "View system status" })).toBeVisible();
  });

  test("missing git executable appears in the homepage health dialog", async ({
    testPage,
    backend,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 800 });

    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(missingGitHealth),
      }),
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const issueButton = testPage.getByRole("button", { name: "Setup Issues" });
    await expect(issueButton).toBeVisible();
    await issueButton.click();

    const dialog = testPage.getByRole("dialog", { name: "Setup Issues" });
    await expect(dialog.getByText("Git executable is required")).toBeVisible();
    await expect(
      dialog.getByText("Install Git and ensure the git executable is available on PATH."),
    ).toBeVisible();
    await expect(dialog.getByRole("button", { name: "View system status" })).toBeVisible();
  });
});
