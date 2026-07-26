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

  test("display menu configures the compact list", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Alpha mobile sort", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Zulu mobile sort", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const archivedTask = await apiClient.createTask(
      seedData.workspaceId,
      "Archived mobile sort task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    await apiClient.archiveTask(archivedTask.id);

    await testPage.goto("/tasks?group=none");
    await testPage.waitForLoadState("networkidle");
    await testPage.getByTestId("mobile-search-toggle").click();
    await testPage
      .getByTestId("mobile-search-bar")
      .getByPlaceholder("Search tasks...")
      .fill("mobile sort");

    await expect(testPage.getByTestId("tasks-list-sort")).not.toBeVisible();
    await testPage.getByRole("button", { name: "Open menu" }).tap();
    const menu = testPage.getByRole("dialog", { name: "Menu" });
    await menu.getByTestId("mobile-tasks-list-sort").tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "Title Z-A" }).tap();

    await expect(testPage).toHaveURL((url) => url.searchParams.get("sort") === "title_desc");
    await expect
      .poll(() => testPage.getByTestId("tasks-list-row-title").allTextContents())
      .toEqual(["Zulu mobile sort", "Alpha mobile sort"]);

    await menu.getByTestId("mobile-tasks-list-group").tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "State" }).tap();
    await expect(testPage).toHaveURL((url) => url.searchParams.get("group") === "state");

    await expect(
      testPage.getByTestId("tasks-list").getByText("Archived mobile sort task"),
    ).toHaveCount(0);
    await menu.getByTestId("mobile-tasks-list-show-archived").tap();
    await expect(
      testPage.getByTestId("tasks-list").getByText("Archived mobile sort task"),
    ).toBeVisible();
  });
});
