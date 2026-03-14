import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("First-time setup: timeouts and error handling", () => {
  // Allow one retry for transient cold-start timing issues on first test.
  test.describe.configure({ retries: 1 });
  test("GitHub branch fetch network failure shows error", async ({ testPage, backend }) => {
    // Intercept branch fetch endpoint and abort (simulates network failure / timeout)
    await testPage.route(
      `${backend.baseUrl}/api/v1/github/repos/slow-owner/slow-repo/branches`,
      (route) => route.abort("failed"),
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    // Toggle to GitHub URL mode
    await testPage.getByTestId("toggle-github-url").click();
    await testPage.getByTestId("github-url-input").fill("https://github.com/slow-owner/slow-repo");

    // The error should appear after the fetch fails
    const errorEl = testPage.getByTestId("github-url-error");
    await expect(errorEl).toBeVisible({ timeout: 10_000 });
    await expect(errorEl).toContainText("not found or not accessible");
  });

  test("health indicator shows issues and opens dialog", async ({ testPage, backend }) => {
    // Intercept the health endpoint and return a detection timeout issue
    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          healthy: false,
          issues: [
            {
              id: "agent_detection_failed",
              category: "agents",
              title: "Agent detection timed out",
              message: "Could not verify agent installations. Check Settings > Agents for details.",
              severity: "warning",
              fix_url: "/settings/agents",
              fix_label: "Check Agents",
            },
          ],
        }),
      }),
    );

    await testPage.goto("/");

    // Health indicator should appear with the warning
    const healthBtn = testPage.getByRole("button", { name: "Setup Issues" });
    await expect(healthBtn).toBeVisible({ timeout: 15_000 });

    // Click to open the dialog and verify the issue content
    await healthBtn.click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByText("Agent detection timed out")).toBeVisible();
    await expect(dialog.getByText("Check Agents")).toBeVisible();
  });
});
