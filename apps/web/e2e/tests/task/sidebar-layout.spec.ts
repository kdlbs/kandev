/**
 * E2E tests for the repo-grouped task sidebar layout.
 *
 * Covers:
 *   - Repo group headers: letter avatar, task count, collapsible
 *   - Subtasks (parent_id): nested under parent with arrow indicator
 *   - Unassigned group: tasks with no repo
 *   - Context menu: Edit, Create Subtask, Rename, Archive, Delete options via right-click
 *   - Active task highlight: selected task has aria/visual selection state
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";
import {
  repositoryName,
  seedSidebarCombinationRepository,
  SIDEBAR_COMBINATION_REPOSITORY_NAME,
} from "./sidebar-repository-grouping-helpers";

test.describe("Sidebar layout — repo groups", () => {
  test("multi-repository group shows the complete ordered label", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const secondRepository = await seedSidebarCombinationRepository(
      apiClient,
      seedData.workspaceId,
      backend.tmpDir,
    );
    const primaryRepositoryName = await repositoryName(
      apiClient,
      seedData.workspaceId,
      seedData.repositoryId,
    );
    const expectedLabel = `${primaryRepositoryName}, ${SIDEBAR_COMBINATION_REPOSITORY_NAME}`;
    const multiRepositoryTask = await apiClient.createTask(
      seedData.workspaceId,
      "Sidebar multi-repository task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [
          { repository_id: seedData.repositoryId },
          { repository_id: secondRepository.id },
        ],
      },
    );
    await apiClient.createTask(seedData.workspaceId, "Sidebar primary repository task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const navigationTask = await apiClient.createTask(
      seedData.workspaceId,
      "Sidebar grouping navigation task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    await testPage.goto(`/t/${navigationTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const combinationHeader = session.sidebar.locator(
      `[data-testid="sidebar-group-header"][data-group-label="${expectedLabel}"]`,
    );
    await expect(combinationHeader).toHaveCount(1);
    const combinationGroup = combinationHeader.locator("..");
    await expect(
      combinationGroup.locator(`[data-task-row-id="${multiRepositoryTask.id}"]`),
    ).toBeVisible();

    const primaryGroup = session.sidebar
      .locator(`[data-testid="sidebar-group-header"][data-group-key="${primaryRepositoryName}"]`)
      .locator("..");
    await expect(primaryGroup).toHaveCount(1);
    await expect(
      primaryGroup.locator(`[data-task-row-id="${multiRepositoryTask.id}"]`),
    ).toHaveCount(0);

    await prCapture.screenshot("multi-repository-group-desktop", {
      caption: "Desktop sidebar groups the task under its complete repository combination.",
    });
  });

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
    const anyGroupHeader = session.sidebar.locator("[data-testid='sidebar-group-header']");
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
    const groupHeader = session.sidebar.locator("[data-testid='sidebar-group-header']").first();
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

    // In a single-repo workspace, no "Unassigned" group should appear.
    // Keyed on `data-group-key`, the identity, not `data-group-label` — the
    // label is catalog-backed now, so a label selector would silently stop
    // matching under any non-English locale.
    const unassignedGroup = session.sidebar.locator(
      "[data-testid='sidebar-group-header'][data-group-key='__unassigned__']",
    );
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
    prCapture,
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
    await expect(testPage.getByRole("menuitem", { name: "Edit", exact: true })).toBeVisible({
      timeout: 5_000,
    });
    await expect(testPage.getByRole("menuitem", { name: "Rename" })).toBeVisible({
      timeout: 5_000,
    });
    await expect(testPage.getByRole("menuitem", { name: "Create Subtask" })).toBeVisible({
      timeout: 5_000,
    });
    await expect(testPage.getByRole("menuitem", { name: "Archive" })).toBeVisible({
      timeout: 5_000,
    });
    await expect(testPage.getByRole("menuitem", { name: "Delete" })).toBeVisible({
      timeout: 5_000,
    });
    await prCapture.screenshot("desktop-create-subtask-context-menu", {
      caption: "Desktop task context menu with Create Subtask",
    });

    // Dismiss
    await testPage.keyboard.press("Escape");
  });

  test("opens the existing editor and persists sidebar task edits", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const originalTitle = "Sidebar edit target";
    const updatedTitle = "Sidebar edit target updated";
    const task = await apiClient.createTask(seedData.workspaceId, originalTitle, {
      description: "Sidebar edit description",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const navigationTask = await apiClient.createTask(
      seedData.workspaceId,
      "Sidebar edit navigation task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    await apiClient.updateTaskState(task.id, "IN_PROGRESS");
    await testPage.goto(`/t/${navigationTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const initialUrl = testPage.url();

    const taskRow = session.sidebar
      .locator("[data-testid='sidebar-task-item']")
      .filter({ hasText: originalTitle });
    await expect(taskRow).toBeVisible({ timeout: 10_000 });
    await taskRow.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Edit", exact: true }).click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog.getByTestId("task-title-input")).toHaveValue(originalTitle);
    await expect(dialog.getByTestId("task-description-input")).toHaveValue(
      "Sidebar edit description",
    );
    await prCapture.screenshot("desktop-sidebar-task-edit-dialog", {
      caption: "Desktop sidebar task editor",
    });
    await dialog.getByTestId("task-title-input").fill(updatedTitle);
    await dialog.getByRole("button", { name: "Update", exact: true }).click();

    await expect(dialog).toHaveCount(0);
    await expect(testPage).toHaveURL(initialUrl);
    await expect.poll(async () => (await apiClient.getTask(task.id)).title).toBe(updatedTitle);
  });
});

test.describe("Sidebar layout — task timestamps and sorting", () => {
  test("every task row shows a relative timestamp on second line", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create three tasks
    await apiClient.createTask(seedData.workspaceId, "Timestamp Task A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Timestamp Task B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const navTask = await apiClient.createTask(seedData.workspaceId, "Timestamp Task C", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${navTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Every sidebar task row should contain a relative time string ("now", "ago", etc.)
    const taskRows = session.sidebar.locator("[data-testid='sidebar-task-item']");
    await expect(taskRows).toHaveCount(3, { timeout: 10_000 });

    for (let i = 0; i < 3; i++) {
      const text = await taskRows.nth(i).innerText();
      // Should contain "now" or "ago" — formatRelativeTime always produces one of these
      expect(text, `task row ${i} text=${JSON.stringify(text)}`).toMatch(/now|ago/);
    }
  });

  test("tasks are sorted by createdAt descending (newest first) within state bucket", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Create three tasks in order; each should appear above the previous
    const taskOldest = await apiClient.createTask(seedData.workspaceId, "Sort Task Oldest", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await dwell(
      testPage,
      50,
      "clock-separation",
      "this test asserts sidebar ordering by creation time, so consecutive creations need distinguishable created_at values; the thing being waited for is the clock, not an app event",
    );
    await apiClient.createTask(seedData.workspaceId, "Sort Task Middle", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await dwell(
      testPage,
      50,
      "clock-separation",
      "this test asserts sidebar ordering by creation time, so consecutive creations need distinguishable created_at values; the thing being waited for is the clock, not an app event",
    );
    await apiClient.createTask(seedData.workspaceId, "Sort Task Newest", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${taskOldest.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskRows = session.sidebar.locator("[data-testid='sidebar-task-item']");
    await expect(taskRows).toHaveCount(3, { timeout: 10_000 });

    // Within the same state bucket (all backlog), newest should be first
    const titles = await Promise.all(
      [0, 1, 2].map(async (i) => (await taskRows.nth(i).innerText()).split("\n")[0]),
    );
    const newestIdx = titles.findIndex((t) => t.includes("Newest"));
    const middleIdx = titles.findIndex((t) => t.includes("Middle"));
    const oldestIdx = titles.findIndex((t) => t.includes("Oldest"));
    expect(newestIdx).toBeLessThan(middleIdx);
    expect(middleIdx).toBeLessThan(oldestIdx);
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
