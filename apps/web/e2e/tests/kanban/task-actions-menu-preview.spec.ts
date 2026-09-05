import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// Covers REQ-TASKS-TASK-ACTIONS-MENU-001/002/003 on the desktop task preview
// panel: the trigger this requirement adds, its parity entries, and the
// preview-scoped Archive and Delete outcomes (remove from board, close the
// panel, no navigation to the task detail route).
test.describe("Task preview panel actions menu", () => {
  test("opens a menu with Edit, Archive, and Delete, and confirming Archive closes the preview without navigating", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await apiClient.createTask(seedData.workspaceId, "Preview Menu Archive Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview Menu Archive Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10_000 });

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible();
    const startUrl = testPage.url();

    const trigger = previewPanel.getByTestId("task-preview-actions-menu");
    await expect(trigger).toHaveAttribute("aria-label", "More options");
    await trigger.click();

    const archiveItem = testPage.getByRole("menuitem", { name: "Archive" });
    await expect(archiveItem).toBeVisible();
    await archiveItem.click();

    const confirmation = testPage.getByTestId("task-archive-confirm-popover");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toContainText("Preview Menu Archive Task");
    await confirmation.getByTestId("archive-task-confirm").click();

    // AC-TASKS-TASK-ACTIONS-MENU-003.3: remove from the board, close the
    // preview panel, no navigation to the task detail route.
    await expect(kanban.taskCardByTitle("Preview Menu Archive Task")).not.toBeVisible({
      timeout: 10_000,
    });
    await expect(previewPanel).not.toBeVisible();
    expect(testPage.url()).not.toBe(startUrl);
    expect(testPage.url()).not.toMatch(/\/t\//);
  });

  test("opens a menu with Edit, Archive, and Delete, and confirming Delete closes the preview without navigating", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await apiClient.createTask(seedData.workspaceId, "Preview Menu Delete Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview Menu Delete Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10_000 });

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible();
    const startUrl = testPage.url();

    const trigger = previewPanel.getByTestId("task-preview-actions-menu");
    await expect(trigger).toHaveAttribute("aria-label", "More options");
    await trigger.click();

    await expect(testPage.getByRole("menuitem", { name: "Edit" })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Archive" })).toBeVisible();
    const deleteItem = testPage.getByRole("menuitem", { name: "Delete" });
    await expect(deleteItem).toBeVisible();
    await deleteItem.click();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Preview Menu Delete Task");
    await dialog.getByRole("button", { name: "Delete" }).click();

    await expect(kanban.taskCardByTitle("Preview Menu Delete Task")).not.toBeVisible({
      timeout: 10_000,
    });
    await expect(previewPanel).not.toBeVisible();
    // No navigation to the task detail route (AC-TASKS-TASK-ACTIONS-MENU-003.3).
    expect(testPage.url()).not.toBe(startUrl);
    expect(testPage.url()).not.toMatch(/\/t\//);
  });

  test("Escape closes only the open menu, keeping the preview open, and returns focus to the trigger; a second Escape closes the preview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    await apiClient.createTask(seedData.workspaceId, "Preview Menu Escape Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Preview Menu Escape Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/taskId=/, { timeout: 10_000 });

    const previewPanel = testPage.getByTestId("task-preview-panel");
    await expect(previewPanel).toBeVisible();

    const trigger = previewPanel.getByTestId("task-preview-actions-menu");
    await trigger.click();
    await expect(testPage.getByRole("menuitem", { name: "Edit" })).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");

    // First Escape (AC-TASKS-TASK-ACTIONS-MENU-001.9/001.11): closes only the
    // menu, leaves the preview open on the same subject, and returns focus to
    // the trigger that opened it.
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByRole("menuitem", { name: "Edit" })).not.toBeVisible();
    await expect(previewPanel).toBeVisible();
    await expect(trigger).toBeFocused();

    // Second Escape, with no menu open, closes the preview panel as it does
    // today.
    await testPage.keyboard.press("Escape");
    await expect(previewPanel).not.toBeVisible();
  });
});
