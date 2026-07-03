import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

async function selectListOption(page: Page, testId: string, optionLabel: string) {
  await page.getByTestId(testId).click();
  await page.getByRole("listbox").getByRole("option", { name: optionLabel }).click();
}

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

    const taskList = testPage.getByTestId("tasks-list");
    const searchBar = testPage.getByTestId("mobile-search-bar");
    const searchToggle = testPage.getByTestId("mobile-search-toggle");

    await expect(taskList.getByText("List Alpha Task")).toBeVisible();
    await expect(taskList.getByText("List Beta Task")).toBeVisible();
    await expect(searchBar).not.toBeVisible();

    await searchToggle.click();
    await expect(searchBar).toBeVisible();
    await expect(searchBar.getByPlaceholder("Search tasks...")).toBeFocused();

    await searchBar.getByPlaceholder("Search tasks...").fill("Alpha");
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).not.toBeVisible({ timeout: 5000 });

    await searchToggle.click();
    await expect(searchBar).not.toBeVisible();
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).toBeVisible({ timeout: 5000 });
  });

  test("sort control reorders the compact list", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Alpha mobile sort", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Zulu mobile sort", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/tasks?group=none");
    await testPage.waitForLoadState("networkidle");
    await testPage.getByTestId("mobile-search-toggle").click();
    await testPage
      .getByTestId("mobile-search-bar")
      .getByPlaceholder("Search tasks...")
      .fill("mobile sort");

    await selectListOption(testPage, "tasks-list-sort", "Title Z-A");

    await expect(testPage).toHaveURL((url) => url.searchParams.get("sort") === "title_desc");
    await expect
      .poll(() => testPage.getByTestId("tasks-list-row-title").allTextContents())
      .toEqual(["Zulu mobile sort", "Alpha mobile sort"]);
  });
});
