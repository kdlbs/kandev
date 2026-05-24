import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression: deleting a task from the task-details sidebar must show a confirm
 * dialog (mirroring the archive flow). Delete is permanent, so silently firing
 * deleteTaskById on the first menu click is a footgun.
 */
test.describe("Task sidebar — delete shows confirmation", () => {
  test("clicking Delete in sidebar menu shows confirm dialog and does not delete until confirmed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar Delete Confirm Task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    // Second task so the sidebar still has something to switch to after delete.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar Delete Survivor Task",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const target = kanban.taskCardByTitle("Sidebar Delete Confirm Task");
    await expect(target).toBeVisible({ timeout: 30_000 });
    await target.click();

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.taskInSidebar("Sidebar Delete Confirm Task")).toBeVisible({
      timeout: 15_000,
    });

    // Open the sidebar menu and click Delete — dialog should appear, task should still exist.
    await session.openSidebarMenuAndClick("Sidebar Delete Confirm Task", "Delete");

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Sidebar Delete Confirm Task");

    // Cancel — the task must still be there.
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(session.taskInSidebar("Sidebar Delete Confirm Task")).toBeVisible({
      timeout: 5_000,
    });

    // Reopen and confirm — now it should disappear.
    await session.openSidebarMenuAndClick("Sidebar Delete Confirm Task", "Delete");
    await expect(testPage.getByRole("alertdialog")).toBeVisible();
    await testPage.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();

    await expect(session.taskInSidebar("Sidebar Delete Confirm Task")).not.toBeVisible({
      timeout: 15_000,
    });
  });
});
