import { test, expect } from "../fixtures/test-base";
import { useRegularMode } from "../helpers/regular-mode";

// Exercises the regular task-create dialog (New Task in the sidebar); run with office off.
useRegularMode();

test.describe("First-time setup: timeouts and error handling", () => {
  // Allow one retry for transient cold-start timing issues on first test.
  test.describe.configure({ retries: 1 });

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
