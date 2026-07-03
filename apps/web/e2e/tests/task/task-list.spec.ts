import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

async function selectListOption(page: Page, testId: string, optionLabel: string) {
  await page.getByTestId(testId).click();
  await page.getByRole("listbox").getByRole("option", { name: optionLabel }).click();
}

async function taskRowTitles(page: Page): Promise<string[]> {
  return page.getByTestId("tasks-list-row-title").allTextContents();
}

test.describe("Task List", () => {
  test("seeded task appears in task list", async ({ testPage, apiClient, seedData }) => {
    await apiClient.createTask(seedData.workspaceId, "Direct Navigate Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // The /tasks page shows all tasks for the workspace in the list view (SSR)
    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");

    // The global AppSidebar also lists tasks (data-testid="sidebar-task-item"),
    // so a bare getByText matches two elements. Scope to the page list.
    await expect(
      testPage.getByTestId("tasks-list").getByText("Direct Navigate Task"),
    ).toBeVisible();
    await expect(testPage.getByTestId("kanban-header-search")).toBeVisible();
    await expect(
      testPage.locator("header").first().getByText("Tasks", { exact: true }),
    ).toHaveCount(0);
    await expect(testPage.locator("main").getByRole("button", { name: "New Task" })).toHaveCount(0);

    const rowBox = await testPage
      .getByTestId("tasks-list-row")
      .filter({ hasText: "Direct Navigate Task" })
      .boundingBox();
    expect(rowBox?.height).toBeLessThan(70);
  });

  test("subtasks render under their parent with indentation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Hierarchy Parent Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Hierarchy Child Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    await testPage.goto("/tasks");
    await testPage.waitForLoadState("networkidle");
    await testPage
      .getByTestId("kanban-header-search")
      .getByPlaceholder("Search tasks...")
      .fill("Hierarchy");

    const taskList = testPage.getByTestId("tasks-list");
    const parentRow = taskList.getByTestId("tasks-list-row").filter({
      hasText: "Hierarchy Parent Task",
    });
    const childRow = taskList.getByTestId("tasks-list-row").filter({
      hasText: "Hierarchy Child Task",
    });

    await expect(parentRow).toHaveAttribute("data-level", "0", { timeout: 5000 });
    await expect(childRow).toHaveAttribute("data-level", "1", { timeout: 5000 });
    await expect(parentRow).toHaveCount(1);
    await expect(childRow).toHaveCount(1);
    await expect(taskList.getByTestId("tasks-list-row-title")).toHaveText([
      "Hierarchy Parent Task",
      "Hierarchy Child Task",
    ]);
  });

  test("sort and group options are deep-linkable and persisted", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Alpha sort task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Zulu sort task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto("/tasks?sort=title_desc&group=none");
    await testPage.waitForLoadState("networkidle");
    await testPage
      .getByTestId("kanban-header-search")
      .getByPlaceholder("Search tasks...")
      .fill("sort task");

    await expect(testPage.getByTestId("tasks-list-sort")).toContainText("Title Z-A");
    await expect(testPage.getByTestId("tasks-list-group")).toContainText("None");
    await expect.poll(() => taskRowTitles(testPage)).toEqual(["Zulu sort task", "Alpha sort task"]);

    await selectListOption(testPage, "tasks-list-sort", "Title A-Z");
    await selectListOption(testPage, "tasks-list-group", "State");

    await expect(testPage).toHaveURL((url) => {
      return (
        url.searchParams.get("sort") === "title_asc" && url.searchParams.get("group") === "state"
      );
    });
    await expect.poll(() => taskRowTitles(testPage)).toEqual(["Alpha sort task", "Zulu sort task"]);

    const settings = await apiClient.getUserSettings();
    expect(settings.settings.tasks_list_sort).toBe("title_asc");
    expect(settings.settings.tasks_list_group).toBe("state");
  });

  test("workflow grouping keeps duplicate workflow names separate", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowA = await apiClient.createWorkflow(seedData.workspaceId, "Duplicate Group");
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Duplicate Group");
    const stepA = await apiClient.createWorkflowStep(workflowA.id, "Start", 0, {
      is_start_step: true,
    });
    const stepB = await apiClient.createWorkflowStep(workflowB.id, "Start", 0, {
      is_start_step: true,
    });
    await apiClient.createTask(seedData.workspaceId, "Duplicate group Alpha", {
      workflow_id: workflowA.id,
      workflow_step_id: stepA.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Duplicate group Beta", {
      workflow_id: workflowB.id,
      workflow_step_id: stepB.id,
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    await testPage.goto("/tasks?sort=title_asc&group=workflow");
    await testPage.waitForLoadState("networkidle");
    await testPage
      .getByTestId("kanban-header-search")
      .getByPlaceholder("Search tasks...")
      .fill("Duplicate group");

    const sections = testPage.getByTestId("tasks-list-section");
    await expect(sections).toHaveCount(2);
    await expect(sections.nth(0).getByTestId("tasks-list-row")).toHaveCount(1);
    await expect(sections.nth(1).getByTestId("tasks-list-row")).toHaveCount(1);
    await expect
      .poll(() => taskRowTitles(testPage))
      .toEqual(["Duplicate group Alpha", "Duplicate group Beta"]);
  });
});
