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

  test("issue rows show identifiers", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Identifier Sort Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    // Scope to `<main>` — the AppSidebar Tasks section duplicates task titles.
    const main = testPage.locator("main");
    await expect(main.getByText("Identifier Sort Task")).toBeVisible({ timeout: 10_000 });
    // Verify identifier elements (font-mono class) are present in issue rows.
    // The span is initially empty until the backend assigns the id — filter to
    // a non-empty one.
    await expect(main.locator(".font-mono").filter({ hasText: /\S/ }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
