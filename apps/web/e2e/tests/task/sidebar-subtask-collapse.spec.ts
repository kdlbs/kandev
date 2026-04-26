/**
 * E2E tests for collapsing subtasks in the left sidebar.
 *
 * Covers:
 *   - Chevron toggle appears on parent tasks that have subtasks
 *   - Clicking the chevron hides/shows the subtask rows
 *   - Clicking the chevron does NOT select the parent task
 *   - Collapsed state survives page refresh (sessionStorage)
 *   - Collapsed state survives navigating to another task
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar subtasks — collapse/expand", () => {
  test("chevron hides and restores subtasks, and survives reload + navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Collapse Parent Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await apiClient.createTask(seedData.workspaceId, "Collapse Child One", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Collapse Child Two", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    // A second root task for the navigation-survives check.
    const other = await apiClient.createTask(seedData.workspaceId, "Collapse Unrelated Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    // Subtasks are visible initially.
    await expect(session.sidebar.getByText("Collapse Child One")).toBeVisible({ timeout: 10_000 });
    await expect(session.sidebar.getByText("Collapse Child Two")).toBeVisible({ timeout: 10_000 });

    // Chevron button appears on the parent row.
    const chevron = session.sidebar.locator(
      `[data-testid='sidebar-subtask-toggle'][data-task-id='${parent.id}']`,
    );
    await expect(chevron).toBeVisible({ timeout: 5_000 });
    await expect(chevron).toHaveAttribute("aria-expanded", "true");

    // Clicking the chevron collapses the subtasks without selecting the parent.
    const urlBefore = testPage.url();
    await chevron.click();

    await expect(session.sidebar.getByText("Collapse Child One")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(session.sidebar.getByText("Collapse Child Two")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(chevron).toHaveAttribute("aria-expanded", "false");
    // URL must be unchanged — chevron click must not navigate.
    expect(testPage.url()).toBe(urlBefore);

    // Parent itself is still visible.
    await expect(session.sidebar.getByText("Collapse Parent Task")).toBeVisible({ timeout: 5_000 });

    // Reload — sessionStorage keeps the collapsed state.
    await testPage.reload();
    await session.waitForLoad();
    await expect(
      session.sidebar.locator(
        `[data-testid='sidebar-subtask-toggle'][data-task-id='${parent.id}']`,
      ),
    ).toHaveAttribute("aria-expanded", "false", { timeout: 10_000 });
    await expect(session.sidebar.getByText("Collapse Child One")).not.toBeVisible({
      timeout: 5_000,
    });

    // Navigate to another task — state is still collapsed.
    await testPage.goto(`/t/${other.id}`);
    await session.waitForLoad();
    await expect(
      session.sidebar.locator(
        `[data-testid='sidebar-subtask-toggle'][data-task-id='${parent.id}']`,
      ),
    ).toHaveAttribute("aria-expanded", "false", { timeout: 10_000 });
    await expect(session.sidebar.getByText("Collapse Child One")).not.toBeVisible({
      timeout: 5_000,
    });

    // Expanding restores the subtasks — and at this point the active task is
    // `other`, not `parent`, so a bubbled parent-row click (if any) would
    // change the URL. Capture it and assert it doesn't change after the
    // chevron click, which exercises the no-navigation invariant on an
    // unselected parent row.
    const chevronAgain = session.sidebar.locator(
      `[data-testid='sidebar-subtask-toggle'][data-task-id='${parent.id}']`,
    );
    const urlBeforeExpand = testPage.url();
    await chevronAgain.click();
    expect(testPage.url()).toBe(urlBeforeExpand);
    await expect(chevronAgain).toHaveAttribute("aria-expanded", "true");
    await expect(session.sidebar.getByText("Collapse Child One")).toBeVisible({ timeout: 5_000 });
    await expect(session.sidebar.getByText("Collapse Child Two")).toBeVisible({ timeout: 5_000 });
  });

  test("tasks without subtasks do not render a chevron toggle", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Lonely Task No Subtasks", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.sidebar.getByText("Lonely Task No Subtasks")).toBeVisible({
      timeout: 10_000,
    });
    // No toggle should exist for this task.
    await expect(
      session.sidebar.locator(`[data-testid='sidebar-subtask-toggle'][data-task-id='${task.id}']`),
    ).toHaveCount(0);
  });
});
