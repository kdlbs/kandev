import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile task list search", () => {
  test("menu search action reveals, filters, and clears on collapse", async ({
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

    await testPage.getByTestId("mobile-topbar-page-context").tap();
    await searchToggle.click();
    await expect(searchBar).toBeVisible();
    await expect(searchBar.getByPlaceholder("Search tasks...")).toBeFocused();

    await searchBar.getByPlaceholder("Search tasks...").fill("Alpha");
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).not.toBeVisible({ timeout: 5000 });

    await testPage.getByTestId("mobile-topbar-page-context").tap();
    await searchToggle.click();
    await expect(searchBar).not.toBeVisible();
    await expect(taskList.getByText("List Alpha Task")).toBeVisible({ timeout: 5000 });
    await expect(taskList.getByText("List Beta Task")).toBeVisible({ timeout: 5000 });
  });

  test("display menu configures the compact list", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
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
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    await testPage.getByTestId("mobile-search-toggle").click();
    await testPage
      .getByTestId("mobile-search-bar")
      .getByPlaceholder("Search tasks...")
      .fill("mobile sort");

    await expect(testPage.getByTestId("tasks-list-sort")).not.toBeVisible();
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    const menu = testPage.getByRole("dialog", { name: "View options" });
    await menu.getByTestId("mobile-tasks-list-sort").tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "Title Z-A" }).tap();

    await expect(testPage).toHaveURL((url) => url.searchParams.get("sort") === "title_desc");
    await expect
      .poll(() => testPage.getByTestId("tasks-list-row-title").allTextContents())
      .toEqual(["Zulu mobile sort", "Alpha mobile sort"]);

    await menu.getByTestId("mobile-tasks-list-group").tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "Workflow step" }).tap();
    await expect(testPage).toHaveURL((url) => url.searchParams.get("group") === "workflow_step");

    await expect(
      testPage.getByTestId("tasks-list").getByText("Archived mobile sort task"),
    ).toHaveCount(0);
    await menu.getByTestId("mobile-tasks-list-show-archived").tap();
    await expect(
      testPage.getByTestId("tasks-list").getByText("Archived mobile sort task"),
    ).toBeVisible();
    await testPage.reload();
    await expect(testPage.getByTestId("tasks-list-section")).toHaveCount(1);
    const { steps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    const startStep = steps.find((step) => step.id === seedData.startStepId)!;
    await expect(testPage.getByTestId("tasks-list-section")).toContainText(startStep.name);
    await prCapture.screenshot("mobile-workflow-step-groups", {
      caption: "Phone task list uses configured workflow step headings.",
    });
    await testPage.getByTestId("mobile-topbar-page-context").tap();
    await expect(menu.getByTestId("mobile-tasks-list-group")).toContainText("Workflow step");
    const groupBox = await menu.getByTestId("mobile-tasks-list-group").boundingBox();
    expect(groupBox?.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await menu.getByTestId("mobile-tasks-list-group").scrollIntoViewIfNeeded();
    await menu.evaluate(async (element) => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .filter((animation) => animation.effect?.getTiming().iterations !== Infinity)
          .map((animation) => animation.finished),
      );
    });
    await prCapture.screenshot("mobile-workflow-step-options", {
      caption: "Phone View options offers the same Workflow step grouping.",
    });
  });
});
