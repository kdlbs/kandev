/**
 * E2E tests for the repo-grouped task sidebar layout.
 *
 * Covers:
 *   - Repo group headers: letter avatar, task count, collapsible
 *   - Subtasks (parent_id): nested under parent with arrow indicator
 *   - Unassigned group: tasks with no repo
 *   - Context menu: Rename, Archive, Delete options via right-click
 *   - Active task highlight: selected task has aria/visual selection state
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar layout — repo groups", () => {
  test("tasks grouped by repository with header showing label and count", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create two tasks in the seeded repo so there is one group
    await apiClient.createTask(seedData.workspaceId, "Sidebar Group Task A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Sidebar Group Task B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // Navigate to any task to open the session page which shows the sidebar
    const task = await apiClient.createTask(seedData.workspaceId, "Sidebar Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Sidebar is visible
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    // At least one repo group header should be visible.
    // The group label for a local path repo will be its local_path — we can
    // match without knowing the exact slug by checking for any group header.
    const anyGroupHeader = session.sidebar.locator("[data-testid^='sidebar-repo-group-']");
    await expect(anyGroupHeader.first()).toBeVisible({ timeout: 10_000 });

    // A group header should contain a task count greater than zero
    const countText = anyGroupHeader.first().locator("span").nth(1);
    const count = await countText.innerText();
    expect(Number(count)).toBeGreaterThan(0);
  });

  test("repo group header is collapsible — clicking hides and restores tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Collapsible Task Alpha", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Collapsible Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // In single-repo workspaces, unassigned tasks merge into the repo group,
    // so both tasks should be in the same group.
    const groupHeader = session.sidebar.locator("[data-testid^='sidebar-repo-group-']").first();
    await expect(groupHeader).toBeVisible({ timeout: 10_000 });

    // Both tasks should be visible before collapsing
    await expect(session.sidebar.getByText("Collapsible Task Alpha")).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.sidebar.getByText("Collapsible Nav Task")).toBeVisible({
      timeout: 10_000,
    });

    // Collapse the group
    await groupHeader.click();

    // After collapsing, both tasks should be hidden
    await expect(session.sidebar.getByText("Collapsible Task Alpha")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(session.sidebar.getByText("Collapsible Nav Task")).not.toBeVisible({
      timeout: 5_000,
    });

    // Expand again
    await groupHeader.click();
    await expect(session.sidebar.getByText("Collapsible Task Alpha")).toBeVisible({
      timeout: 5_000,
    });
  });

  test("tasks without repository merge into single repo group instead of Unassigned", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create a task with the repo and one without
    await apiClient.createTask(seedData.workspaceId, "Repo Task One", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "No Repo Task One", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      // no repository_ids
    });

    const task = await apiClient.createTask(seedData.workspaceId, "No Repo Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // In a single-repo workspace, no "Unassigned" group should appear
    const unassignedGroup = session.sidebar.getByTestId("sidebar-repo-group-Unassigned");
    await expect(unassignedGroup).not.toBeVisible({ timeout: 5_000 });

    // Both tasks should be visible in the sidebar (under the repo group)
    await expect(session.sidebar.getByText("Repo Task One", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.sidebar.getByText("No Repo Task One", { exact: true })).toBeVisible({
      timeout: 5_000,
    });
  });
});

test.describe("Sidebar layout — subtasks", () => {
  test("subtask appears nested under parent with arrow indicator", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create parent task
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Task Sidebar", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // Create subtask with parent_id
    await apiClient.createTask(seedData.workspaceId, "Child Subtask Sidebar", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Both titles are visible in the sidebar
    await expect(session.sidebar.getByText("Parent Task Sidebar")).toBeVisible({ timeout: 10_000 });
    await expect(session.sidebar.getByText("Child Subtask Sidebar")).toBeVisible({
      timeout: 10_000,
    });

    // The subtask row has the isSubTask styling — it uses pl-8 (subtask indent)
    // and renders the ↳ arrow indicator
    const subtaskRow = session.sidebar
      .locator("[data-testid='sidebar-task-item']")
      .filter({ hasText: "Child Subtask Sidebar" });
    await expect(subtaskRow).toBeVisible({ timeout: 5_000 });

    // Arrow indicator ↳ is present inside the subtask row
    await expect(subtaskRow.getByText("↳")).toBeVisible({ timeout: 5_000 });
  });
});

test.describe("Sidebar layout — context menu", () => {
  test("right-clicking a task shows Rename, Archive, and Delete options", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Context Menu Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Context Menu Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Right-click on the task row
    const taskRow = session.sidebar
      .locator("[data-testid='sidebar-task-item']")
      .filter({ hasText: "Context Menu Task" });
    await expect(taskRow).toBeVisible({ timeout: 10_000 });
    await taskRow.click({ button: "right" });

    // Context menu items should be visible
    await expect(testPage.getByRole("menuitem", { name: "Rename" })).toBeVisible({
      timeout: 5_000,
    });
    await expect(testPage.getByRole("menuitem", { name: "Archive" })).toBeVisible({
      timeout: 5_000,
    });
    await expect(testPage.getByRole("menuitem", { name: "Delete" })).toBeVisible({
      timeout: 5_000,
    });

    // Dismiss
    await testPage.keyboard.press("Escape");
  });
});

test.describe("Sidebar layout — active task highlight", () => {
  test("selected task has visual selection state", async ({ testPage, apiClient, seedData }) => {
    const taskA = await apiClient.createTask(seedData.workspaceId, "Highlight Task A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${taskA.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // The task row for the active task should have the selected styling
    // TaskItem applies bg-primary/10 and a left border when isSelected=true
    const activeRow = session.sidebar
      .locator("[data-testid='sidebar-task-item']")
      .filter({ hasText: "Highlight Task A" });
    await expect(activeRow).toBeVisible({ timeout: 10_000 });

    // The selected row has a specific class pattern from isSelected — check for it
    // isSelected applies `bg-primary/10` which becomes part of the element's class
    await expect(activeRow).toHaveClass(/bg-primary/, { timeout: 5_000 });
  });
});
