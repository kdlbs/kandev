import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Kanban topbar utilities", () => {
  test("settings is reachable directly from the topbar (no dropdown)", async ({ testPage }) => {
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
});
