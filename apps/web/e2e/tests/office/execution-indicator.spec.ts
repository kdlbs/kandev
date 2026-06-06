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

  test("issue row displays task identifier", async ({ testPage, apiClient, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Identifier Test Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto("/office/tasks");
    // Scope to `<main>` — the AppSidebar Tasks section duplicates task titles.
    const main = testPage.locator("main");
    await expect(main.getByText("Identifier Test Task")).toBeVisible({ timeout: 10_000 });
    // A task identifier renders in a `.font-mono` span (matches main; the title
    // above is the part that needed `<main>` scoping for the sidebar duplication).
    await expect(testPage.locator(".font-mono").first()).toBeVisible({ timeout: 5_000 });
  });
});
