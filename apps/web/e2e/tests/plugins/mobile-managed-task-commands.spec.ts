import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  createPluginTaskInput,
  installAndGrantTaskWrite,
  invokeExactTaskCreate,
  invokePluginTaskTreeDelete,
} from "./managed-task-command-helpers";

async function openMobileTaskDeleteDialog(page: Page, title: string) {
  const session = new SessionPage(page);
  await session.mobileSessionMenu.tap();
  const drawer = page
    .getByTestId("mobile-task-switcher-list")
    .locator("xpath=ancestor::*[@data-slot='drawer-content'][1]");
  await expect(drawer).toBeVisible();
  const row = drawer.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(row).toBeVisible();
  await row.locator("button.mobile-task-actions-button").tap();
  const menu = page.locator('[data-slot="context-menu-content"]:visible');
  await expect(menu).toHaveCount(1);
  await menu.getByRole("menuitem", { name: "Delete", exact: true }).tap();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

test.describe("managed task commands on mobile", () => {
  let cleanupTaskIds: string[] = [];

  test.beforeEach(() => {
    cleanupTaskIds = [];
  });

  test.afterEach(async ({ apiClient }) => {
    for (const taskId of [...cleanupTaskIds].reverse()) {
      await apiClient.deleteTask(taskId).catch(() => undefined);
    }
    await uninstallFixturePlugin(apiClient);
  });

  test("phone deletion uses touch confirmation and rejects a stale preview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installAndGrantTaskWrite(testPage, apiClient, seedData.workspaceId);
    const createInput = createPluginTaskInput(
      "Mobile managed coordinator task",
      seedData.workflowId,
      seedData.startStepId,
    );
    const created = await invokeExactTaskCreate(apiClient, seedData.workspaceId, createInput);
    expect(created.status).toBe("APPLIED");
    if (!created.task_id) throw new Error("exact task create returned no task id");
    cleanupTaskIds.push(created.task_id);
    const child = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile managed coordinator child",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        parent_id: created.task_id,
      },
    );
    cleanupTaskIds.push(child.id);

    const pluginDelete = await invokePluginTaskTreeDelete(
      apiClient,
      seedData.workspaceId,
      created.task_id,
    );
    expect(pluginDelete.ok).toBe(false);
    expect((await apiClient.rawRequest("GET", `/api/v1/tasks/${created.task_id}`)).status).toBe(
      200,
    );

    await testPage.goto(`/t/${created.task_id}`);
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    const dialog = await openMobileTaskDeleteDialog(testPage, createInput.title);
    const deleteAction = dialog.getByRole("button", { name: "Delete", exact: true });
    await expect(deleteAction).toBeEnabled();
    await expect
      .poll(async () => (await deleteAction.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    const actionBox = await deleteAction.boundingBox();
    if (!actionBox) throw new Error("mobile delete action has no layout box");
    expect(actionBox.y + actionBox.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);

    await apiClient.updateTaskTitle(child.id, "Mobile child changed after deletion preview");
    await deleteAction.tap();
    await expect
      .poll(
        async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${created.task_id}`)).status,
        {
          timeout: 15_000,
        },
      )
      .toBe(200);
    await expect
      .poll(async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${child.id}`)).status, {
        timeout: 15_000,
      })
      .toBe(200);
    await expect(dialog).toHaveCount(0, { timeout: 5_000 });

    const currentDialog = await openMobileTaskDeleteDialog(testPage, createInput.title);
    const cascade = currentDialog.getByTestId("delete-cascade-checkbox");
    await expect(cascade).toBeVisible();
    await cascade.tap();
    const confirm = currentDialog.getByRole("button", { name: "Delete", exact: true });
    await expect(confirm).toBeEnabled();
    await confirm.tap();
    await expect
      .poll(
        async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${created.task_id}`)).status,
        {
          timeout: 15_000,
        },
      )
      .toBe(404);
    await expect
      .poll(async () => (await apiClient.rawRequest("GET", `/api/v1/tasks/${child.id}`)).status, {
        timeout: 15_000,
      })
      .toBe(404);
  });
});
