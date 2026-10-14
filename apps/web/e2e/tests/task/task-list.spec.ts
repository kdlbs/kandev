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
  test("Portuguese workflow step choice fits the closed grouping control", async ({
    testPage,
    backend,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Prepare release checklist", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage
      .context()
      .addCookies([{ name: "kandev_locale", value: "pt-pt", url: backend.frontendUrl }]);
    await testPage.goto("/tasks");
    await expect(testPage.locator("html")).toHaveAttribute("lang", "pt-pt");
    const group = testPage.getByTestId("tasks-list-group");
    await expect(group).toContainText("Etapa do fluxo de trabalho");
    const size = await group.locator('[data-slot="select-value"]').evaluate((element) => ({
      content: element.scrollWidth,
      visible: element.clientWidth,
    }));
    expect(size.content).toBeLessThanOrEqual(size.visible);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await expect(
      testPage.getByTestId("tasks-list").getByText("Prepare release checklist"),
    ).toBeVisible();
    const { steps } = await apiClient.listWorkflowSteps(seedData.workflowId);
    await expect(testPage.getByTestId("tasks-list-section")).toContainText(
      steps.find((step) => step.id === seedData.startStepId)!.name,
    );
  });
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
    // The bar names the page through the shared title crumb.
    await expect(testPage.locator('[data-slot="breadcrumb-page"]')).toHaveText("Tasks");
    await expect(testPage.locator("main").getByRole("button", { name: "New Task" })).toHaveCount(0);

    const rowBox = await testPage
      .getByTestId("tasks-list-row")
      .filter({ hasText: "Direct Navigate Task" })
      .boundingBox();
    expect(rowBox?.height).toBeLessThan(70);
  });

  test("rows show non-medium priority on first render", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Urgent list task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "critical",
    });
    await apiClient.createTask(seedData.workspaceId, "Ordinary list task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "medium",
    });

    await testPage.goto("/tasks");

    const list = testPage.getByTestId("tasks-list");
    const urgentRow = list.getByTestId("tasks-list-row").filter({ hasText: "Urgent list task" });
    const ordinaryRow = list
      .getByTestId("tasks-list-row")
      .filter({ hasText: "Ordinary list task" });
    await expect(urgentRow.getByRole("img", { name: /Critical/ })).toBeVisible();
    await expect(ordinaryRow).toBeVisible();
    await expect(ordinaryRow.getByTestId("tasks-list-row-priority")).toHaveCount(0);
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
    const parentRow = taskList
      .getByTestId("tasks-list-row-title")
      .filter({ hasText: /^Hierarchy Parent Task$/ })
      .locator("xpath=ancestor::*[@data-testid='tasks-list-row']");
    const childRow = taskList
      .getByTestId("tasks-list-row-title")
      .filter({ hasText: /^Hierarchy Child Task$/ })
      .locator("xpath=ancestor::*[@data-testid='tasks-list-row']");

    // Wait for the filtered list to settle before reading row attributes. SSR
    // and the client list can briefly overlap while the search result mounts.
    await expect(parentRow).toHaveCount(1, { timeout: 5000 });
    await expect(childRow).toHaveCount(1, { timeout: 5000 });
    await expect(parentRow).toHaveAttribute("data-level", "0", { timeout: 5000 });
    await expect(childRow).toHaveAttribute("data-level", "1", { timeout: 5000 });
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
    await selectListOption(testPage, "tasks-list-group", "Workflow step");

    await expect(testPage).toHaveURL((url) => {
      return (
        url.searchParams.get("sort") === "title_asc" &&
        url.searchParams.get("group") === "workflow_step"
      );
    });
    await expect.poll(() => taskRowTitles(testPage)).toEqual(["Alpha sort task", "Zulu sort task"]);

    const settings = await apiClient.getUserSettings();
    expect(settings.settings.tasks_list_sort).toBe("title_asc");
    expect(settings.settings.tasks_list_group).toBe("workflow_step");
    await testPage.reload();
    await expect(testPage.getByTestId("tasks-list-group")).toContainText("Workflow step");
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

  test("cold list groups by configured steps across workflows", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const delivery = await apiClient.createWorkflow(seedData.workspaceId, "Delivery");
    const support = await apiClient.createWorkflow(seedData.workspaceId, "Support");
    const planning = await apiClient.createWorkflowStep(delivery.id, "Planning", 0, {
      is_start_step: true,
    });
    const implementation = await apiClient.createWorkflowStep(delivery.id, "Implementation", 1);
    const supportImplementation = await apiClient.createWorkflowStep(
      support.id,
      "Implementation",
      0,
      {
        is_start_step: true,
      },
    );
    await apiClient.createTask(seedData.workspaceId, "Draft release checklist", {
      workflow_id: delivery.id,
      workflow_step_id: planning.id,
    });
    const completed = await apiClient.createTask(seedData.workspaceId, "Improve task navigation", {
      workflow_id: delivery.id,
      workflow_step_id: implementation.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Add workflow filters", {
      workflow_id: delivery.id,
      workflow_step_id: implementation.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Resolve notification issue", {
      workflow_id: support.id,
      workflow_step_id: supportImplementation.id,
    });
    await apiClient.updateTaskState(completed.id, "COMPLETED");
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
    });

    await testPage.goto("/tasks?sort=title_asc&group=state");
    await expect(testPage.getByTestId("tasks-list-group")).toContainText("Workflow step");
    const sections = testPage.getByTestId("tasks-list-section");
    await expect(sections).toHaveCount(3);
    await expect(sections.nth(0)).toContainText("Delivery / Planning");
    await expect(sections.nth(1)).toContainText("Delivery / Implementation");
    await expect(sections.nth(1).getByTestId("tasks-list-row-title")).toHaveText([
      "Add workflow filters",
      "Improve task navigation",
    ]);
    await expect(sections.nth(2)).toContainText("Support / Implementation");
    const rename = await apiClient.rawRequest(
      "PUT",
      `/api/v1/workflow/steps/${implementation.id}`,
      {
        name: "Build",
      },
    );
    expect(rename.ok, await rename.text()).toBe(true);
    await expect(sections.nth(1)).toContainText("Delivery / Build");
    const restore = await apiClient.rawRequest(
      "PUT",
      `/api/v1/workflow/steps/${implementation.id}`,
      {
        name: "Implementation",
      },
    );
    expect(restore.ok, await restore.text()).toBe(true);
    await expect(sections.nth(1)).toContainText("Delivery / Implementation");
    await expect(testPage.getByTestId("sidebar-task-item").first()).toBeVisible();
    await expect(testPage.getByText("Updating tasks...", { exact: true })).not.toBeVisible();
    await prCapture.screenshot("desktop-workflow-step-groups", {
      caption: "Desktop task list groups by configured steps and distinguishes workflows.",
    });
    const created = await apiClient.createWorkflow(seedData.workspaceId, "Operations");
    const createdStep = await apiClient.createWorkflowStep(created.id, "Documentation", 0, {
      is_start_step: true,
    });
    await apiClient.createTask(seedData.workspaceId, "Document workflow steps", {
      workflow_id: created.id,
      workflow_step_id: createdStep.id,
    });
    await testPage.evaluate(() => window.dispatchEvent(new Event("focus")));
    const newSection = sections.filter({ hasText: "Operations / Documentation" });
    await expect(newSection.getByTestId("tasks-list-row-title")).toHaveText(
      "Document workflow steps",
    );
    await testPage
      .context()
      .addCookies([{ name: "kandev_locale", value: "pt-pt", url: new URL(testPage.url()).origin }]);
    await testPage.reload();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "pt-pt");
    await expect(testPage.getByTestId("tasks-list-group")).toContainText(
      "Etapa do fluxo de trabalho",
    );
    await expect(newSection).toContainText("Operations / Documentation");
    await expect(sections.nth(1)).toContainText("Delivery / Implementation");
    await expect(testPage.getByTestId("sidebar-task-item").first()).toBeVisible();
    await prCapture.screenshot("desktop-portuguese-workflow-step", {
      caption: "The Portuguese Workflow step label fits the desktop grouping selector.",
    });
  });

  test("pagination shows totals, rows per page, and page numbers", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await Promise.all(
      Array.from({ length: 26 }, (_, index) =>
        apiClient.createTask(
          seedData.workspaceId,
          `Paged list task ${String(index + 1).padStart(2, "0")}`,
          {
            workflow_id: seedData.workflowId,
            workflow_step_id: seedData.startStepId,
          },
        ),
      ),
    );

    await testPage.goto("/tasks?sort=title_asc&group=none");
    await testPage.waitForLoadState("networkidle");
    await testPage
      .getByTestId("kanban-header-search")
      .getByPlaceholder("Search tasks...")
      .fill("Paged list task");

    const pagination = testPage.getByTestId("tasks-pagination");
    await expect(pagination).toContainText("Showing 1 to 25 of 26 results");
    await expect(testPage.getByTestId("tasks-pagination-page-size")).toContainText("25");
    await expect(pagination.getByRole("button", { name: "1", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );

    await pagination.getByRole("button", { name: "Go to next page" }).click();
    await expect(pagination).toContainText("Showing 26 to 26 of 26 results");
    await expect.poll(() => taskRowTitles(testPage)).toEqual(["Paged list task 26"]);

    await testPage.getByTestId("tasks-pagination-page-size").click();
    await testPage.getByRole("listbox").getByRole("option", { name: "10" }).click();
    await expect(pagination).toContainText("Showing 1 to 10 of 26 results");
    await expect(testPage.getByTestId("tasks-pagination-page-size")).toContainText("10");
  });
});
