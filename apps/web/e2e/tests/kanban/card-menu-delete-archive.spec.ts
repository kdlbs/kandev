import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// Regression: clicking Delete / Archive must show the confirm dialog, not navigate to the task.
test.describe("Kanban card actions menu — delete/archive does not navigate", () => {
  test("clicking Delete in card dropdown shows confirm dialog and does not navigate", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Card Menu Delete Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const startUrl = testPage.url();

    await kanban.openTaskActionsMenu(task.id);

    const deleteItem = testPage.getByRole("menuitem", { name: "Delete" });
    await expect(deleteItem).toBeVisible();
    await deleteItem.click();

    // Confirm dialog must appear with the task title
    await expect(testPage.getByRole("alertdialog")).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toContainText("Card Menu Delete Task");

    // URL must not have changed (no navigation to /tasks/:id and no ?taskId=)
    expect(testPage.url()).toBe(startUrl);
  });

  test("clicking Archive in card dropdown shows confirm dialog and does not navigate", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Card Menu Archive Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const startUrl = testPage.url();

    await kanban.openTaskActionsMenu(task.id);

    const archiveItem = testPage.getByRole("menuitem", { name: "Archive" });
    await expect(archiveItem).toBeVisible();
    await archiveItem.click();

    await expect(testPage.getByRole("alertdialog")).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toContainText("Card Menu Archive Task");

    expect(testPage.url()).toBe(startUrl);
  });

  test("clicking Delete with preview-on-click enabled does not open preview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const task = await apiClient.createTask(seedData.workspaceId, "Card Menu Preview Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const startUrl = testPage.url();

    await kanban.openTaskActionsMenu(task.id);
    await testPage.getByRole("menuitem", { name: "Delete" }).click();

    await expect(testPage.getByRole("alertdialog")).toBeVisible();
    // Preview-on-click must not have fired — URL should still be the start URL
    expect(testPage.url()).toBe(startUrl);
    await expect(testPage.getByTestId("task-preview-panel")).not.toBeVisible();
  });
});
