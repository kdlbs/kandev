/**
 * E2E for multi-level (depth >= 2) subtask nesting in the left sidebar.
 *
 * The production CreateTask path caps kanban nesting at depth 1, so this builds
 * the root → child → grandchild chain through the test harness (`seedTask`),
 * which writes directly via the repository and bypasses that guard — the same
 * way real depth-2 trees arise (office tasks / pre-guard data).
 *
 * Covers:
 *   - All three levels render, each tagged with its tree depth (0 / 1 / 2)
 *   - A grandchild nests inside its child, which nests inside the root
 *   - Collapsing a MID-level node hides its whole subtree (the grandchild),
 *     while the root and the collapsed node stay visible
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar subtasks — multi-level nesting", () => {
  test("renders and collapses a three-level tree", async ({ testPage, apiClient, seedData }) => {
    const stepOpts = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const root = await apiClient.seedTask(seedData.workspaceId, "Tree Root Task", stepOpts);
    const child = await apiClient.seedTask(seedData.workspaceId, "Tree Child Task", {
      ...stepOpts,
      parent_id: root.task_id,
    });
    const grandchild = await apiClient.seedTask(seedData.workspaceId, "Tree Grandchild Task", {
      ...stepOpts,
      parent_id: child.task_id,
    });

    await testPage.goto(`/t/${root.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    // All three levels are visible (expanded by default).
    await expect(session.sidebar.getByText("Tree Root Task")).toBeVisible({ timeout: 10_000 });
    await expect(session.sidebar.getByText("Tree Child Task")).toBeVisible({ timeout: 10_000 });
    await expect(session.sidebar.getByText("Tree Grandchild Task")).toBeVisible({
      timeout: 10_000,
    });

    // Each row carries its depth, and deeper blocks are DOM-nested in shallower
    // ones — proving the tree renders past the old two-level cap.
    const block = (id: string) =>
      session.sidebar.locator(`[data-testid='sortable-task-block'][data-task-id='${id}']`);
    await expect(block(root.task_id)).toHaveAttribute("data-depth", "0");
    await expect(block(child.task_id)).toHaveAttribute("data-depth", "1");
    await expect(block(grandchild.task_id)).toHaveAttribute("data-depth", "2");
    // grandchild block lives inside the child block, which lives inside the root.
    await expect(block(root.task_id).locator(`[data-task-id='${grandchild.task_id}']`)).toHaveCount(
      1,
    );

    // Collapse the MID-level child: its subtree (the grandchild) disappears,
    // while the root and the child itself stay visible.
    const childChevron = session.sidebar.locator(
      `[data-testid='sidebar-subtask-toggle'][data-task-id='${child.task_id}']`,
    );
    await expect(childChevron).toHaveAttribute("aria-expanded", "true");
    await childChevron.click();

    await expect(session.sidebar.getByText("Tree Grandchild Task")).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(session.sidebar.getByText("Tree Root Task")).toBeVisible();
    await expect(session.sidebar.getByText("Tree Child Task")).toBeVisible();
    await expect(childChevron).toHaveAttribute("aria-expanded", "false");

    // Expanding restores the grandchild.
    await childChevron.click();
    await expect(session.sidebar.getByText("Tree Grandchild Task")).toBeVisible({ timeout: 5_000 });
  });
});
