import { expect, test } from "../../fixtures/test-base";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import { KanbanPage } from "../../pages/kanban-page";
import type { ApiClient } from "../../helpers/api-client";
import {
  createPluginTaskInput,
  installAndGrantTaskWrite,
  invokeExactTaskCreate,
  invokePluginTaskTreeDelete,
} from "./managed-task-command-helpers";

async function readTaskStatus(apiClient: ApiClient, taskId: string) {
  return (await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`)).status;
}

test.describe("managed task commands", () => {
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

  test("delegation retries deduplicate and plugin deletion requires the native task confirmation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installAndGrantTaskWrite(testPage, apiClient, seedData.workspaceId);

    const createInput = createPluginTaskInput(
      "Managed coordinator delegated task",
      seedData.workflowId,
      seedData.startStepId,
    );
    const first = await invokeExactTaskCreate(apiClient, seedData.workspaceId, createInput);
    const retry = await invokeExactTaskCreate(apiClient, seedData.workspaceId, createInput);
    expect(first, JSON.stringify(first)).toMatchObject({ status: "APPLIED" });
    expect(retry, JSON.stringify(retry)).toMatchObject({
      status: "APPLIED",
      task_id: first.task_id,
    });
    const taskId = first.task_id;
    if (!taskId) throw new Error("exact task create returned no task id");
    cleanupTaskIds.push(taskId);

    const child = await apiClient.createTask(
      seedData.workspaceId,
      "Managed coordinator child task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        parent_id: taskId,
      },
    );
    cleanupTaskIds.push(child.id);

    const pluginDelete = await invokePluginTaskTreeDelete(apiClient, seedData.workspaceId, taskId);
    expect(pluginDelete.ok).toBe(false);
    expect(await readTaskStatus(apiClient, taskId)).toBe(200);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(createInput.title)).toBeVisible();
    await kanban.openTaskActionsMenu(taskId);
    await testPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
    const staleDialog = testPage.getByRole("alertdialog");
    await expect(staleDialog).toBeVisible();
    const deleteAction = staleDialog.getByRole("button", { name: "Delete", exact: true });
    await expect(deleteAction).toBeEnabled();

    await apiClient.updateTaskTitle(child.id, "Child changed after deletion preview");
    await deleteAction.click();
    await expect.poll(() => readTaskStatus(apiClient, taskId), { timeout: 15_000 }).toBe(200);
    await expect.poll(() => readTaskStatus(apiClient, child.id), { timeout: 15_000 }).toBe(200);
    await expect(staleDialog).toHaveCount(0, { timeout: 5_000 });
    await expect(testPage.locator('[data-slot="alert-dialog-overlay"]')).toHaveCount(0);
    await expect
      .poll(() => testPage.evaluate(() => getComputedStyle(document.body).pointerEvents))
      .not.toBe("none");

    await kanban.openTaskActionsMenu(taskId);
    await testPage.getByRole("menuitem", { name: "Delete", exact: true }).click();
    const currentDialog = testPage.getByRole("alertdialog");
    await expect(currentDialog.getByTestId("delete-cascade-checkbox")).toBeVisible();
    await currentDialog.getByTestId("delete-cascade-checkbox").click();
    const currentDelete = currentDialog.getByRole("button", { name: "Delete", exact: true });
    await expect(currentDelete).toBeEnabled();
    await currentDelete.click();
    await expect.poll(() => readTaskStatus(apiClient, taskId), { timeout: 15_000 }).toBe(404);
    await expect.poll(() => readTaskStatus(apiClient, child.id), { timeout: 15_000 }).toBe(404);
  });
});
