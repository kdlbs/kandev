import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const TASK_VISIBLE_TIMEOUT = 10_000;
const LONG_TITLE = "T".repeat(60);

/**
 * Regression: typing mid-title in a task title field at the 60-character cap
 * used to rewrite the DOM value and reset the caret to the end of the field on
 * every keystroke. The caret must stay immediately after the inserted text.
 */
test.describe("Task title fields keep the caret at the 60-char cap", () => {
  test("task-edit dialog keeps the caret when typing mid-title at the cap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(LONG_TITLE)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(task.id);
    await testPage.getByRole("menuitem", { name: "Edit" }).click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const input = dialog.getByTestId("task-title-input");
    await expect(input).toHaveValue(LONG_TITLE);

    await input.click();
    await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(6, 6));
    await testPage.keyboard.type("XY");

    // "XY" stays at position 6, the last two characters of the original title
    // are dropped by the cap, and the caret sits right after the insert.
    await expect(input).toHaveValue(`${LONG_TITLE.slice(0, 6)}XY${LONG_TITLE.slice(6, 58)}`);
    expect(await input.evaluate((el) => (el as HTMLInputElement).selectionStart)).toBe(8);
  });

  test("task-edit dialog keeps the caret when a same-char keystroke at the cap leaves the value unchanged", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(LONG_TITLE)).toBeVisible({
      timeout: TASK_VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(task.id);
    await testPage.getByRole("menuitem", { name: "Edit" }).click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const input = dialog.getByTestId("task-title-input");
    await expect(input).toHaveValue(LONG_TITLE);

    await input.click();
    await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(6, 6));
    await testPage.keyboard.type("T");

    // The value cannot change (still 60 T's), but the caret must stay right
    // after the insertion point instead of jumping to the end.
    await expect(input).toHaveValue(LONG_TITLE);
    expect(await input.evaluate((el) => (el as HTMLInputElement).selectionStart)).toBe(7);
  });

  test("task rename dialog keeps the caret when typing mid-title at the cap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const row = session.sidebarTaskItem(LONG_TITLE);
    await expect(row).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await row.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Rename", exact: true }).click();

    const dialog = testPage.getByRole("dialog", { name: "Rename task" });
    await expect(dialog).toBeVisible();
    const input = dialog.getByRole("textbox");
    await expect(input).toHaveValue(LONG_TITLE);

    await input.click();
    await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(6, 6));
    await testPage.keyboard.type("XY");

    await expect(input).toHaveValue(`${LONG_TITLE.slice(0, 6)}XY${LONG_TITLE.slice(6, 58)}`);
    expect(await input.evaluate((el) => (el as HTMLInputElement).selectionStart)).toBe(8);
  });
});
