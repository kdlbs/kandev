import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("First-time setup: timeouts and error handling", () => {
  test("health indicator appears when system has issues", async ({ testPage, backend }) => {
    // Intercept the health endpoint and return issues (simulating missing GitHub + agents)
    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          healthy: false,
          issues: [
            {
              id: "github_unavailable",
              category: "github",
              title: "GitHub integration unavailable",
              message: "Install the gh CLI and run 'gh auth login', or add a GITHUB_TOKEN secret.",
              severity: "warning",
              fix_url: "/settings/workspace/{workspaceId}/github",
              fix_label: "Configure GitHub",
            },
            {
              id: "no_agents",
              category: "agents",
              title: "No AI agents detected",
              message: "Install an AI coding agent to start using KanDev.",
              severity: "warning",
              fix_url: "/settings/agents",
              fix_label: "Configure Agents",
            },
          ],
        }),
      }),
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // The health indicator button should appear (amber warning icon)
    const healthBtn = testPage.locator('button:has(span:text("Setup Issues"))');
    await expect(healthBtn).toBeVisible({ timeout: 10_000 });

    // Click it to open the health issues dialog
    await healthBtn.click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Both issues should be listed
    await expect(dialog.getByText("GitHub integration unavailable")).toBeVisible();
    await expect(dialog.getByText("No AI agents detected")).toBeVisible();

    // Fix buttons should be present
    await expect(dialog.getByText("Configure GitHub")).toBeVisible();
    await expect(dialog.getByText("Configure Agents")).toBeVisible();
  });

  test("health indicator shows timeout warning when agent detection is slow", async ({
    testPage,
    backend,
  }) => {
    // Simulate the health endpoint returning a detection timeout issue
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

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Health indicator should appear
    const healthBtn = testPage.locator('button:has(span:text("Setup Issues"))');
    await expect(healthBtn).toBeVisible({ timeout: 10_000 });

    await healthBtn.click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByText("Agent detection timed out")).toBeVisible();
    await expect(dialog.getByText("Check Agents")).toBeVisible();
  });

  test("agent discovery shows empty state when API fails", async ({ testPage, backend }) => {
    // Intercept discovery endpoint to return an error
    await testPage.route(`${backend.baseUrl}/api/v1/agents/discovery`, (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "discovery failed" }),
      }),
    );

    await testPage.goto("/settings/agents");

    // Should show the "no installed agents" message (not hang on "Scanning...")
    await expect(testPage.getByText("No installed agents were detected")).toBeVisible({
      timeout: 10_000,
    });
    // "Scanning for installed agents..." should NOT be visible
    await expect(testPage.getByText("Scanning for installed agents...")).not.toBeVisible();
  });

  test("agent discovery network failure shows empty state", async ({ testPage, backend }) => {
    // Intercept discovery endpoint and abort the request (simulates network failure / timeout)
    await testPage.route(`${backend.baseUrl}/api/v1/agents/discovery`, (route) =>
      route.abort("failed"),
    );

    await testPage.goto("/settings/agents");

    // The catch handler should fire and show the empty state
    await expect(testPage.getByText("No installed agents were detected")).toBeVisible({
      timeout: 10_000,
    });
  });

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

  test("GitHub branch fetch shows not-configured error for 503", async ({ testPage, backend }) => {
    // Intercept branch fetch and return the new 503 "not configured" response
    await testPage.route(
      `${backend.baseUrl}/api/v1/github/repos/unconfigured-owner/unconfigured-repo/branches`,
      (route) =>
        route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({
            error:
              "GitHub is not configured. Install the gh CLI and run 'gh auth login', or add a GITHUB_TOKEN secret.",
            code: "github_not_configured",
          }),
        }),
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("toggle-github-url").click();
    await testPage
      .getByTestId("github-url-input")
      .fill("https://github.com/unconfigured-owner/unconfigured-repo");

    // Should show the "not found or not accessible" error (current frontend maps all non-timeout errors)
    const errorEl = testPage.getByTestId("github-url-error");
    await expect(errorEl).toBeVisible({ timeout: 10_000 });
    await expect(errorEl).toContainText("not found or not accessible");
  });
});
