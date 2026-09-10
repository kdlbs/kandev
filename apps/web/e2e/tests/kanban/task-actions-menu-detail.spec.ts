import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Covers REQ-TASKS-TASK-ACTIONS-MENU-001/002/003 on the desktop task detail
// top bar: the trigger this requirement adds, its parity entries, and the
// detail-scoped Archive outcome (archive-and-switch away from the archived
// task, matching every other task-scoped archive entry point).
test.describe("Task detail top bar actions menu", () => {
  test("opens a menu with Edit, Archive, and Delete, and confirming Archive switches away from the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Detail Menu Archive Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Detail Menu Archive Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const topBar = testPage.getByTestId("task-topbar");
    const trigger = topBar.getByTestId("task-topbar-actions-menu");
    await expect(trigger).toHaveAttribute("aria-label", "More options");
    await trigger.click();

    await expect(testPage.getByRole("menuitem", { name: "Edit" })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Delete" })).toBeVisible();
    const archiveItem = testPage.getByRole("menuitem", { name: "Archive" });
    await expect(archiveItem).toBeVisible();
    await archiveItem.click();

    const confirmation = testPage.getByTestId("task-archive-confirm-popover");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toContainText("Detail Menu Archive Task");
    await confirmation.getByTestId("archive-task-confirm").click();

    // Archive-and-switch (AC-TASKS-TASK-ACTIONS-MENU-003.4): the detail route
    // no longer points at the archived task.
    await expect
      .poll(() => testPage.url(), {
        timeout: 15_000,
        message: "Waiting for switch-away navigation",
      })
      .not.toMatch(new RegExp(`/t/${task.id}(?:$|[/?])`));
  });
});
