import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const TASK_VISIBLE_TIMEOUT = 10_000;

// Regression: clicking Delete / Archive must show the local confirmation, not navigate to the task.
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
    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Card Menu Delete Task");
    const dialogBox = await dialog.boundingBox();
    if (!dialogBox) throw new Error("delete confirmation dialog has no layout box");
    expect(dialogBox.width).toBeGreaterThanOrEqual(480);
    await expect(dialog).toHaveClass(/font-sans/);
    const deleteHeader = dialog.locator('[data-slot="alert-dialog-header"]');
    await expect
      .poll(async () =>
        deleteHeader.evaluate((element) => {
          const style = getComputedStyle(element);
          return { justifyItems: style.justifyItems, textAlign: style.textAlign };
        }),
      )
      .toEqual({ justifyItems: "start", textAlign: "left" });

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

    const confirmation = testPage.getByTestId("task-archive-confirm-popover");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toContainText("Card Menu Archive Task");
    await expect(confirmation.getByTestId("archive-task-confirm")).toBeVisible();

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

// Regression: in "All Workflows" swimlane view, state.kanban.workflowId is null.
// useTaskCRUD's handleDelete/handleArchive used to early-return on that, so the
// dialog closed but no API call ran and the task stayed on the board.
test.describe("Kanban card actions menu — delete/archive in All Workflows view", () => {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- testPage is listed to force its fixture (user-settings reset) to run before this hook's API seeding; without it the reset wipes our workflow_filter_id on first use
  test.beforeEach(async ({ apiClient, seedData, testPage }) => {
    // Need a second workflow so resolveDesiredWorkflowId does not auto-select
    // the only visible workflow when filter is null.
    await apiClient.createWorkflow(seedData.workspaceId, "Secondary Workflow", "simple");
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: "",
      repository_ids: [],
    });
  });

  test("confirming Delete removes the task from the board in All Workflows view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "All-Wf Delete Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle("All-Wf Delete Task")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(task.id);
    await testPage.getByRole("menuitem", { name: "Delete" }).click();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Delete" }).click();

    await expect(kanban.taskCardByTitle("All-Wf Delete Task")).not.toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
  });

  test("confirming Archive removes the task from the board in All Workflows view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "All-Wf Archive Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle("All-Wf Archive Task")).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(task.id);
    await testPage.getByRole("menuitem", { name: "Archive" }).click();

    const confirmation = testPage.getByTestId("task-archive-confirm-popover");
    await expect(confirmation).toBeVisible();
    await confirmation.getByTestId("archive-task-confirm").click();

    await expect(kanban.taskCardByTitle("All-Wf Archive Task")).not.toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });
  });
});
