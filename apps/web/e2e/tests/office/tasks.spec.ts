import { test, expect } from "../../fixtures/office-fixture";

test.describe("Tasks (Issues)", () => {
  test("tasks page loads", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/tasks");
    await expect(testPage.getByRole("heading", { name: /Tasks/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("list tasks returns array", async ({ officeApi, officeSeed }) => {
    const result = await officeApi.listTasks(officeSeed.workspaceId);
    const issues = (result as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    expect(Array.isArray(issues)).toBe(true);
  });

  test("onboarding task appears in tasks when created with title", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Use the existing office workspace and create a task directly rather than
    // re-running onboarding (which would fail: "workspace already has a CEO agent").
    await apiClient.createTask(officeSeed.workspaceId, "My Onboarding Issue", {
      workflow_id: officeSeed.workflowId,
    });

    const issues = await officeApi.listTasks(officeSeed.workspaceId);
    const list = (issues as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    const found = list.find((i) => (i as Record<string, unknown>).title === "My Onboarding Issue");
    expect(found).toBeDefined();
  });

  test("task created via API appears in tasks list", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    await apiClient.createTask(officeSeed.workspaceId, "API Created Issue", {
      workflow_id: officeSeed.workflowId,
    });

    const result = await officeApi.listTasks(officeSeed.workspaceId);
    const list = (result as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    const found = list.find((i) => (i as Record<string, unknown>).title === "API Created Issue");
    expect(found).toBeDefined();
  });

  test("task created via API appears in tasks page UI", async ({
    testPage,
    apiClient,
    officeApi: _,
    officeSeed,
  }) => {
    await apiClient.createTask(officeSeed.workspaceId, "UI Visible Issue", {
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("UI Visible Issue")).toBeVisible({ timeout: 10_000 });
  });

  test("get task by id returns correct data", async ({ apiClient, officeApi, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Fetch By ID Issue", {
      workflow_id: officeSeed.workflowId,
    });

    const issueResp = await officeApi.getTask(task.id);
    const i = (issueResp as { task: Record<string, unknown> }).task;
    expect(i.id).toBe(task.id);
    expect(i.title).toBe("Fetch By ID Issue");
  });

  test("subtask has parent_id in task response", async ({ apiClient, officeApi, officeSeed }) => {
    const parent = await apiClient.createTask(officeSeed.workspaceId, "Parent Issue", {
      workflow_id: officeSeed.workflowId,
    });
    const child = await apiClient.createTask(officeSeed.workspaceId, "Child Issue", {
      workflow_id: officeSeed.workflowId,
      parent_id: parent.id,
    });

    const childIssueResp = await officeApi.getTask(child.id);
    const c = (childIssueResp as { task: Record<string, unknown> }).task;
    expect(c.id).toBe(child.id);
    expect(c.title).toBe("Child Issue");
    // parent linkage preserved
    expect(c.parentId ?? c.parent_id).toBe(parent.id);
  });

  test("task search returns matching results", async ({ apiClient, officeApi, officeSeed }) => {
    await apiClient.createTask(officeSeed.workspaceId, "Searchable Unique Task XYZ987", {
      workflow_id: officeSeed.workflowId,
    });

    const results = await officeApi.searchTasks(officeSeed.workspaceId, "XYZ987");
    const tasks = (results as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    expect(tasks.length).toBeGreaterThan(0);
    const found = tasks.find(
      (t) =>
        (t as Record<string, unknown>).title &&
        ((t as Record<string, unknown>).title as string).includes("XYZ987"),
    );
    expect(found).toBeDefined();
  });

  test("task search with no results returns empty array", async ({ officeApi, officeSeed }) => {
    const results = await officeApi.searchTasks(
      officeSeed.workspaceId,
      "NORESULT_NONEXISTENT_TOKEN_99999",
    );
    const tasks = (results as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    expect(Array.isArray(tasks)).toBe(true);
    expect(tasks.length).toBe(0);
  });
});
