import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

// @covers AC-TASKS-TASK-ACTIONS-MENU-004.2
test("phone card stays interactive after a stale deletion confirmation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(90_000);
  const parent = await apiClient.createTask(seedData.workspaceId, "Phone delete recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const child = await apiClient.createTask(seedData.workspaceId, "Phone recovery child", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    parent_id: parent.id,
  });
  try {
    await testPage.goto(`/?workflowId=${encodeURIComponent(seedData.workflowId)}`);
    const mobile = new MobileKanbanPage(testPage);
    await expect(mobile.mobileKanbanLayout()).toBeVisible();
    const card = mobile.taskCard(parent.id);
    await expect(card).toBeVisible();
    const trigger = card.getByRole("button", { name: "More options" });
    await trigger.tap();
    await testPage.getByRole("menuitem", { name: "Delete", exact: true }).tap();
    const dialog = testPage.getByRole("alertdialog");
    const action = dialog.getByRole("button", { name: "Delete", exact: true });
    await expect(action).toBeEnabled();

    await apiClient.updateTaskTitle(child.id, "Changed after phone deletion preview");
    await action.tap();
    await expect(dialog).toHaveCount(0);
    await expect(testPage.getByRole("menu")).toHaveCount(0);
    await expect
      .poll(() => testPage.evaluate(() => getComputedStyle(document.body).pointerEvents))
      .not.toBe("none");
    expect((await apiClient.rawRequest("GET", `/api/v1/tasks/${parent.id}`)).status).toBe(200);
    expect((await apiClient.rawRequest("GET", `/api/v1/tasks/${child.id}`)).status).toBe(200);
    await assertNoDocumentHorizontalOverflow(testPage, "phone deletion recovery");

    await trigger.tap();
    await testPage.getByRole("menuitem", { name: "Delete", exact: true }).tap();
    await dialog.getByTestId("delete-cascade-checkbox").tap();
    await expect(action).toBeEnabled();
    await action.tap();
    for (const taskId of [parent.id, child.id]) {
      await expect
        .poll(() => apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`).then((r) => r.status))
        .toBe(404);
    }
  } finally {
    await apiClient.deleteTask(child.id).catch(() => undefined);
    await apiClient.deleteTask(parent.id).catch(() => undefined);
  }
});
