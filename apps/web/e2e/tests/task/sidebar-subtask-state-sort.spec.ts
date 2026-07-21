/**
 * E2E test for effective-state sort bubbling in the left sidebar.
 *
 * When the sidebar is sorted by state (the default), a parent task should
 * adopt the highest-priority state bucket among itself and its direct
 * subtasks. So a backlog parent with an in-progress subtask must bubble up
 * above an unrelated backlog peer.
 *
 * This is the end-to-end counterpart to the unit coverage in
 * `lib/sidebar/apply-view.test.ts`: it proves the live store threads the
 * active `state` sort through `applyView` and the sidebar renders the
 * bubbled order in the DOM.
 *
 * Regression value: without the fix both parent and peer share the backlog
 * bucket, where the tiebreak is `createdAt` desc — so the newer peer (created
 * last) would render ABOVE the older parent. The assertion below only holds
 * once the subtask bubbles the parent into the in_progress bucket.
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar subtasks — effective state sort", () => {
  test("a backlog parent with an in-progress subtask bubbles above a backlog peer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Parent task — stays in a backlog-bucket state.
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

    // A second root task created LAST — stays in the backlog bucket. Without
    // bubbling it would sort above the older parent (backlog tiebreak is
    // newest-createdAt-first).
    const peer = await apiClient.createTask(seedData.workspaceId, "Bubble Backlog Peer", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    // Wait for both root rows to render before reading order.
    const parentBlock = session.sidebar.locator(
      `[data-testid='sortable-task-block'][data-task-id='${parent.id}']`,
    );
    const peerBlock = session.sidebar.locator(
      `[data-testid='sortable-task-block'][data-task-id='${peer.id}']`,
    );
    await expect(parentBlock).toBeVisible({ timeout: 10_000 });
    await expect(peerBlock).toBeVisible({ timeout: 10_000 });

    // Read root-task order from the DOM and assert the parent precedes the peer.
    // Comparing indices (not absolute positions) keeps this robust against any
    // other tasks present in the sidebar.
    const rootBlocks = session.sidebar.locator("[data-testid='sortable-task-block']");
    const orderedIds = await rootBlocks.evaluateAll((els) =>
      els.map((el) => el.getAttribute("data-task-id")),
    );
    expect(orderedIds.indexOf(parent.id)).toBeGreaterThanOrEqual(0);
    expect(orderedIds.indexOf(parent.id)).toBeLessThan(orderedIds.indexOf(peer.id));
  });
});
