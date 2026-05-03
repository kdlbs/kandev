import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Kanban topbar utilities", () => {
  test("settings is reachable directly from the topbar (no dropdown)", async ({ testPage }) => {
    // Lock to desktop width — on tablet/mobile Settings lives inside the hamburger sheet.
    await testPage.setViewportSize({ width: 1280, height: 800 });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const settingsButton = testPage.getByRole("link", { name: "Settings" });
    await expect(settingsButton).toBeVisible();

    await settingsButton.click();
    await expect(testPage).toHaveURL(/\/settings/);
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
});
