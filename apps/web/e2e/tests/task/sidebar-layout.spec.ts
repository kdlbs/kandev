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
  test("tasks grouped by repository with header showing letter avatar and count", async ({
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
    const countText = anyGroupHeader.first().locator("span").nth(2);
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

    // Find the first non-Unassigned repo group header (the one for the seeded repo)
    const groupHeader = session.sidebar
      .locator("[data-testid^='sidebar-repo-group-']")
      .filter({ hasNot: session.sidebar.locator("[data-testid='sidebar-repo-group-Unassigned']") })
      .first();
    await expect(groupHeader).toBeVisible({ timeout: 10_000 });

    // Task should be visible before collapsing — scoped to the text of the task
    await expect(session.sidebar.getByText("Collapsible Task Alpha")).toBeVisible({
      timeout: 10_000,
    });

    // Collapse the group
    await groupHeader.click();

    // After collapsing, the task in that repo group should not be visible
    await expect(session.sidebar.getByText("Collapsible Task Alpha")).not.toBeVisible({
      timeout: 5_000,
    });

    // The unassigned task is in a different group and should still be visible
    await expect(session.sidebar.getByText("Collapsible Nav Task")).toBeVisible({ timeout: 5_000 });

    // Expand again
    await groupHeader.click();
    await expect(session.sidebar.getByText("Collapsible Task Alpha")).toBeVisible({
      timeout: 5_000,
    });
  });

  test("unassigned group shown for tasks with no repository", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create a task without a repository
    await apiClient.createTask(seedData.workspaceId, "Unassigned Task One", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      // no repository_ids
    });

    const task = await apiClient.createTask(seedData.workspaceId, "Unassigned Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // The "Unassigned" group header should be visible
    const unassignedGroup = session.sidebar.getByTestId("sidebar-repo-group-Unassigned");
    await expect(unassignedGroup).toBeVisible({ timeout: 10_000 });

    // The unassigned task should appear in that group
    // After the group header, tasks are siblings in the DOM
    await expect(session.sidebar.getByText("Unassigned Task One")).toBeVisible({ timeout: 5_000 });
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
