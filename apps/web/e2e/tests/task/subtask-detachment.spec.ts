import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DETACH_LABEL = "Detach from parent";

test.describe("Subtask detachment", () => {
  test("promotes an inherited-workspace subtree live from the sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parent = await apiClient.createTask(
      seedData.workspaceId,
      "Detach sidebar parent",
      placement,
    );
    const child = await apiClient.createTask(seedData.workspaceId, "Detach sidebar child", {
      ...placement,
      parent_id: parent.id,
      workspace_mode: "inherit_parent",
    });
    const descendant = await apiClient.seedTask(seedData.workspaceId, "Detach sidebar descendant", {
      ...placement,
      parent_id: child.id,
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const taskBlock = (taskId: string) =>
      session.sidebar.locator(`[data-testid="sortable-task-block"][data-task-id="${taskId}"]`);
    await expect(taskBlock(child.id)).toHaveAttribute("data-depth", "1");
    await expect(taskBlock(descendant.task_id)).toHaveAttribute("data-depth", "2");

    const parentRow = session.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Detach sidebar parent" });
    await parentRow.click({ button: "right" });
    await expect(testPage.getByRole("menuitem", { name: DETACH_LABEL })).toHaveCount(0);
    await testPage.keyboard.press("Escape");

    const childRow = session.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Detach sidebar child" });
    await childRow.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: DETACH_LABEL }).click();

    const dialog = testPage.getByRole("alertdialog", { name: "Detach task from parent?" });
    await expect(dialog).toContainText("shares its parent's workspace");
    await expect(dialog).toContainText("Current and future sessions");
    await dialog.getByTestId("detach-task-confirm").click();

    await expect(taskBlock(child.id)).toHaveAttribute("data-depth", "0", { timeout: 10_000 });
    await expect(taskBlock(descendant.task_id)).toHaveAttribute("data-depth", "1");
    await expect(taskBlock(child.id).locator(`[data-task-id="${descendant.task_id}"]`)).toHaveCount(
      1,
    );

    await expect
      .poll(async () => {
        const detached = await apiClient.getTask(child.id);
        const workspace = detached.metadata?.workspace as { mode?: string } | undefined;
        return { parentId: detached.parent_id ?? "", workspaceMode: workspace?.mode };
      })
      .toEqual({
        parentId: "",
        workspaceMode: "shared_group",
      });
    await expect
      .poll(async () => (await apiClient.getTask(descendant.task_id)).parent_id)
      .toBe(child.id);

    await childRow.click({ button: "right" });
    await expect(testPage.getByRole("menuitem", { name: DETACH_LABEL })).toHaveCount(0);
  });

  test("keeps card menus aligned and preserves kanban placement", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parent = await apiClient.createTask(
      seedData.workspaceId,
      "Detach kanban parent",
      placement,
    );
    const child = await apiClient.createTask(seedData.workspaceId, "Detach kanban child", {
      ...placement,
      parent_id: parent.id,
      workspace_mode: "new_workspace",
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.openTaskActionsMenu(parent.id);
    await expect(testPage.getByRole("menuitem", { name: DETACH_LABEL })).toHaveCount(0);
    await testPage.keyboard.press("Escape");

    await kanban.openTaskContextMenu(child.id);
    await expect(testPage.getByRole("menuitem", { name: DETACH_LABEL })).toBeVisible();
    await testPage.keyboard.press("Escape");

    await kanban.openTaskActionsMenu(child.id);
    await testPage.getByRole("menuitem", { name: DETACH_LABEL }).click();
    await testPage.getByRole("alertdialog").getByTestId("detach-task-confirm").click();

    await expect(kanban.taskCardInColumn("Detach kanban child", seedData.startStepId)).toBeVisible({
      timeout: 10_000,
    });
    await expect
      .poll(async () => {
        const detached = await apiClient.getTask(child.id);
        const workspace = detached.metadata?.workspace as { mode?: string } | undefined;
        return {
          parentId: detached.parent_id ?? "",
          stepId: detached.workflow_step_id,
          workspaceMode: workspace?.mode,
        };
      })
      .toEqual({
        parentId: "",
        stepId: seedData.startStepId,
        workspaceMode: "new_workspace",
      });
  });
});
