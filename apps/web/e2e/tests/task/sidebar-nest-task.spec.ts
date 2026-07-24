/**
 * E2E tests for the "Nest" feature: making a task a sub-task of another from
 * the sidebar dots (⋯) context menu, and un-nesting it again.
 *
 * Signal used: once a child is nested under a parent, the parent row grows a
 * subtask toggle (`sidebar-subtask-toggle`). Un-nesting removes it.
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar — nest / un-nest a task", () => {
  test("nests a task under another via the dots menu, then un-nests it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Nest Parent Alpha", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Nest Child Beta", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    await expect(session.sidebar.getByText("Nest Child Beta")).toBeVisible({ timeout: 10_000 });

    const parentToggle = session.sidebar.locator(
      `[data-testid='sidebar-subtask-toggle'][data-task-id='${parent.id}']`,
    );
    // Initially both are root tasks — the parent has no subtask toggle.
    await expect(parentToggle).toHaveCount(0);

    const childRow = session.sidebar
      .locator("[data-testid='sidebar-task-item']")
      .filter({ hasText: "Nest Child Beta" });

    // Open the dots menu on the child and nest it under the parent.
    await childRow.hover();
    await childRow.getByRole("button", { name: "Task actions" }).click();
    await testPage.getByRole("menuitem", { name: "Nest under" }).hover();
    await testPage.getByRole("menuitem", { name: "Nest Parent Alpha" }).click();

    // The parent now shows the subtask toggle (child nested underneath).
    await expect(parentToggle).toBeVisible({ timeout: 10_000 });
    await expect(parentToggle).toHaveAttribute("aria-expanded", "true");

    // Un-nest: reopen the child's dots menu and remove its parent.
    await childRow.hover();
    await childRow.getByRole("button", { name: "Task actions" }).click();
    await testPage.getByRole("menuitem", { name: "Nest under" }).hover();
    await testPage.getByRole("menuitem", { name: "Un-nest" }).click();

    // The parent's subtask toggle disappears — the child is a root task again.
    await expect(parentToggle).toHaveCount(0, { timeout: 10_000 });
    await expect(session.sidebar.getByText("Nest Child Beta")).toBeVisible();
  });
});
