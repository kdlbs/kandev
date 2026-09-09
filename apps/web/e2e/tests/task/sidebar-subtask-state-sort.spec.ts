/**
 * E2E test for effective-state sort bubbling in the left sidebar.
 *
 * When the sidebar is grouped and sorted by state, a parent task should use
 * the effective state of its complete included tree. A completed parent with
 * an in-progress subtask must therefore render in the in-progress group.
 *
 * This is the end-to-end counterpart to the unit coverage in
 * `lib/sidebar/apply-view-effective-state.test.ts`: it proves the live store
 * threads the effective state through `applyView` and the sidebar renders the
 * parent tree in the matching group and order.
 *
 * Regression value: without the fix the completed parent remains in the
 * completed group, even though its child is running.
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// @covers AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.1, .3, .5
test.describe("Sidebar subtasks — effective state sort", () => {
  let originalSidebarSettings:
    | { sidebar_views?: unknown[]; sidebar_active_view_id?: string }
    | undefined;

  test.beforeEach(async ({ apiClient }) => {
    const { settings } = await apiClient.getUserSettings();
    originalSidebarSettings = {
      sidebar_views: Array.isArray(settings.sidebar_views) ? settings.sidebar_views : [],
      sidebar_active_view_id:
        typeof settings.sidebar_active_view_id === "string" ? settings.sidebar_active_view_id : "",
    };
  });

  test.afterEach(async ({ apiClient }) => {
    if (!originalSidebarSettings) return;
    await apiClient.saveUserSettings(originalSidebarSettings);
    originalSidebarSettings = undefined;
  });

  test("a completed parent with an in-progress subtask uses the active tree state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Parent task — its terminal state must not hide active child work.
    const parent = await apiClient.createTask(seedData.workspaceId, "Bubble Parent Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // Child subtask whose task state we flip to IN_PROGRESS. `classifyTask`
    // buckets a task with no session by its persisted state, so this lands in
    // the in_progress bucket without needing to drive an agent.
    const child = await apiClient.createTask(seedData.workspaceId, "Bubble Child Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskState(child.id, "IN_PROGRESS");
    await apiClient.updateTaskState(parent.id, "COMPLETED");

    // A second active root created LAST. Once the tree is resolved to the same
    // bucket, the existing createdAt tiebreak places this newer peer first.
    const peer = await apiClient.createTask(seedData.workspaceId, "Bubble Active Peer", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskState(peer.id, "IN_PROGRESS");

    // The default E2E sidebar view groups by repository. Use a state-grouped
    // view so the regression asserts the actual group heading as well as the
    // root ordering consumed by the rendered tree.
    await apiClient.saveUserSettings({
      sidebar_views: [
        {
          id: "effective-state-view",
          name: "Effective state",
          filters: [],
          sort: { key: "state", direction: "asc" },
          group: "state",
          collapsed_groups: [],
        },
      ],
      sidebar_active_view_id: "effective-state-view",
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const inProgressHeader = session.sidebar.locator(
      "[data-testid='sidebar-group-header'][data-group-key='IN_PROGRESS']",
    );
    await expect(inProgressHeader).toBeVisible({ timeout: 10_000 });
    const inProgressGroup = session.sidebar.locator(
      "[data-testid='sidebar-group'][data-group-key='IN_PROGRESS']",
    );

    // Both active roots must be rendered inside the same effective group.
    const parentBlock = inProgressGroup.locator(
      `[data-testid='sortable-task-block'][data-task-id='${parent.id}']`,
    );
    const peerBlock = inProgressGroup.locator(
      `[data-testid='sortable-task-block'][data-task-id='${peer.id}']`,
    );
    await expect(parentBlock).toBeVisible({ timeout: 10_000 });
    await expect(peerBlock).toBeVisible({ timeout: 10_000 });

    // Read root-task order from the active group and assert the newer peer
    // keeps the existing state-sort tiebreak after the parent bubbles.
    const rootBlocks = inProgressGroup.locator(
      "[data-testid='sortable-task-block'][data-depth='0']",
    );
    const orderedIds = await rootBlocks.evaluateAll((els) =>
      els.map((el) => el.getAttribute("data-task-id")),
    );
    expect(orderedIds.indexOf(peer.id)).toBeGreaterThanOrEqual(0);
    expect(orderedIds.indexOf(peer.id)).toBeLessThan(orderedIds.indexOf(parent.id));
  });
});
