import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const LONG_TITLE = "T".repeat(60);

/**
 * Mobile regression: the phone drawer renders the same TaskRenameDialog and
 * task-edit dialog as desktop. Typing mid-title at the 60-character cap must
 * keep the caret immediately after the inserted text on a phone viewport too.
 */
test.describe("Mobile task title fields keep the caret at the 60-char cap", () => {
  async function openSheet(
    testPage: import("@playwright/test").Page,
    taskId: string,
    title: string,
  ) {
    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.evaluate(
      "window.__KANDEV_E2E_STORE__?.getState().setMobileSessionTaskSwitcherOpen(true)",
    );
    const surface = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = surface.getByTestId("sidebar-task-item").filter({ hasText: title });
    await expect(taskRow).toBeVisible();
    return taskRow;
  }

  test("phone drawer rename keeps the caret when typing mid-title at the cap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const taskRow = await openSheet(testPage, task.id, LONG_TITLE);

    await taskRow.click({ button: "right" });
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

  test("phone drawer rename keeps the caret when a same-char keystroke at the cap leaves the value unchanged", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const taskRow = await openSheet(testPage, task.id, LONG_TITLE);

    await taskRow.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Rename", exact: true }).click();
    const dialog = testPage.getByRole("dialog", { name: "Rename task" });
    await expect(dialog).toBeVisible();
    const input = dialog.getByRole("textbox");
    await expect(input).toHaveValue(LONG_TITLE);

    await input.click();
    await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(6, 6));
    await testPage.keyboard.type("T");

    await expect(input).toHaveValue(LONG_TITLE);
    expect(await input.evaluate((el) => (el as HTMLInputElement).selectionStart)).toBe(7);
  });

  test("phone drawer task-edit keeps the caret when typing mid-title at the cap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const taskRow = await openSheet(testPage, task.id, LONG_TITLE);

    await taskRow.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Edit", exact: true }).click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const input = dialog.getByTestId("task-title-input");
    await expect(input).toHaveValue(LONG_TITLE);

    await input.click();
    await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(6, 6));
    await testPage.keyboard.type("XY");

    await expect(input).toHaveValue(`${LONG_TITLE.slice(0, 6)}XY${LONG_TITLE.slice(6, 58)}`);
    expect(await input.evaluate((el) => (el as HTMLInputElement).selectionStart)).toBe(8);
  });
});
