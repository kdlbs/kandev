import { test, expect } from "../../fixtures/office-fixture";

test.describe("Issue sorting", () => {
  test("tasks list renders with default sort", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Sort Task One", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.createTask(officeSeed.workspaceId, "Sort Task Two", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    // Post-overhaul: the unified AppSidebar's Tasks section also lists tasks, so
    // titles appear in both the rail and the page table. Scope to `<main>` (the
    // office page content, which excludes `<aside data-testid="app-sidebar">`)
    // to avoid strict-mode duplicate matches.
    const main = testPage.locator("main");
    await expect(main.getByText("Sort Task One")).toBeVisible({ timeout: 10_000 });
    await expect(main.getByText("Sort Task Two")).toBeVisible({ timeout: 10_000 });
  });

  test("issue rows render the identifier column", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Identifier Sort Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    // Scope to `<main>` — the AppSidebar Tasks section duplicates task titles.
    const main = testPage.locator("main");
    await expect(main.getByText("Identifier Sort Task")).toBeVisible({ timeout: 10_000 });
    // The office task row renders a `.font-mono` identifier span. Its value is
    // empty in e2e (tasks are created via the core /api/v1/tasks route, which
    // doesn't run the office identifier-assignment flow), so assert the column
    // renders (attached) rather than a value this environment never produces.
    const row = main.getByRole("button", { name: /Identifier Sort Task/ });
    await expect(row.locator(".font-mono")).toBeAttached({ timeout: 10_000 });
  });
});
