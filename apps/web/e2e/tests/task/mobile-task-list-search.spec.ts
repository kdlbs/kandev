import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile task list search", () => {
  test("topbar search icon reveals, filters, and clears on collapse", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "List Alpha Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "List Beta Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    const searchBar = testPage.getByTestId("mobile-search-bar");
    const searchToggle = testPage.getByTestId("mobile-search-toggle");

    // Both tasks are listed and search is collapsed behind the topbar icon
    await expect(testPage.getByRole("table").getByText("List Alpha Task")).toBeVisible();
    await expect(testPage.getByRole("table").getByText("List Beta Task")).toBeVisible();
    await expect(searchBar).not.toBeVisible();

    // Tapping the icon reveals the focused search input
    await searchToggle.click();
    await expect(searchBar).toBeVisible();
    await expect(searchBar.getByPlaceholder("Search tasks...")).toBeFocused();

    // Typing filters the list down to the match
    await searchBar.getByPlaceholder("Search tasks...").fill("Alpha");
    await expect(testPage.getByRole("table").getByText("List Alpha Task")).toBeVisible({
      timeout: 5000,
    });
    await expect(testPage.getByRole("table").getByText("List Beta Task")).not.toBeVisible({
      timeout: 5000,
    });

    // Collapsing clears the query so the full list returns
    await searchToggle.click();
    await expect(searchBar).not.toBeVisible();
    await expect(testPage.getByRole("table").getByText("List Alpha Task")).toBeVisible({
      timeout: 5000,
    });
    await expect(testPage.getByRole("table").getByText("List Beta Task")).toBeVisible({
      timeout: 5000,
    });
  });
});
