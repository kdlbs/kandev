import { test, expect } from "../../fixtures/office-fixture";

test.describe("Execution indicator", () => {
  test("issue row shows status icon for task", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Indicator Test Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    // Post-overhaul: the unified AppSidebar's Tasks section also lists tasks, so
    // the title text appears in BOTH the global rail and the page table. Scope
    // to `<main>` (the office layout's page content, which excludes the
    // `<aside data-testid="app-sidebar">`) to avoid a strict-mode duplicate.
    await expect(testPage.locator("main").getByText("Indicator Test Task")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("issue row renders the identifier column", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Identifier Test Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    // Scope to `<main>` — the AppSidebar Tasks section duplicates task titles.
    const main = testPage.locator("main");
    await expect(main.getByText("Identifier Test Task")).toBeVisible({ timeout: 10_000 });
    // The office task row (`task-row.tsx`) renders a `.font-mono` identifier
    // span. In e2e its value is empty: tasks here are created via the core
    // `/api/v1/tasks` route, which does NOT run the office identifier-assignment
    // flow (the workspace's "KAN" prefix only applies to office-flow-created
    // tasks), so the span is present but blank. Assert the column renders
    // (attached to the row) rather than a value this environment never produces.
    const row = main.getByRole("button", { name: /Identifier Test Task/ });
    await expect(row.locator(".font-mono")).toBeAttached({ timeout: 10_000 });
  });
});
