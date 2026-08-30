import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile subtask detachment", () => {
  test("detaches an inherited-workspace subtask from the task actions sheet", async ({
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
      "Mobile detach parent",
      placement,
    );
    const child = await apiClient.createTask(seedData.workspaceId, "Mobile detach child", {
      ...placement,
      parent_id: parent.id,
      workspace_mode: "inherit_parent",
    });

    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();

    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const childRow = taskSheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile detach child" });
    const actions = childRow.getByRole("button", { name: "Task actions" });
    await expect(actions).toBeVisible({ timeout: 10_000 });
    await actions.click();

    const detachAction = testPage.getByRole("menuitem", { name: "Detach from parent" });
    await detachAction.scrollIntoViewIfNeeded();
    await detachAction.click();

    const dialog = testPage.getByRole("alertdialog", { name: "Detach task from parent?" });
    await expect(dialog).toContainText("shares its parent's workspace");
    await dialog.getByTestId("detach-task-confirm").click();

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

    await testPage.getByTestId("mobile-session-menu").click();
    const promotedBlock = taskSheet.locator(
      `[data-testid="sortable-task-block"][data-task-id="${child.id}"]`,
    );
    await expect(promotedBlock).toHaveAttribute("data-depth", "0", { timeout: 10_000 });
    await promotedBlock
      .getByTestId("sidebar-task-item")
      .getByRole("button", { name: "Task actions" })
      .click();
    await expect(testPage.getByRole("menuitem", { name: "Detach from parent" })).toHaveCount(0);
  });

  test("keeps kanban detachment inline on a phone", async ({ testPage, apiClient, seedData }) => {
    const placement = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const parent = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile card parent",
      placement,
    );
    const child = await apiClient.createTask(seedData.workspaceId, "Mobile card child", {
      ...placement,
      parent_id: parent.id,
      workspace_mode: "new_workspace",
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const card = mobile.taskCard(child.id);
    await card.getByRole("button", { name: "More options" }).click();
    let menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible');
    await menu.getByRole("menuitem", { name: "Detach from parent" }).click();

    let inlineConfirmation = testPage.getByTestId("detach-task-inline-confirmation");
    await expect(inlineConfirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(inlineConfirmation.getByTestId("detach-task-confirm")).toHaveClass(/h-11/);
    await expect(inlineConfirmation.getByTestId("detach-task-confirm")).toHaveClass(/min-w-11/);
    await expect
      .poll(() => testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      .toBe(true);

    await inlineConfirmation.getByRole("button", { name: "Cancel" }).click();
    await expect(inlineConfirmation).toHaveCount(0);
    await expect.poll(async () => (await apiClient.getTask(child.id)).parent_id).toBe(parent.id);

    await card.getByRole("button", { name: "More options" }).click();
    menu = testPage.locator('[data-slot="dropdown-menu-content"]:visible');
    await menu.getByRole("menuitem", { name: "Detach from parent" }).click();
    inlineConfirmation = testPage.getByTestId("detach-task-inline-confirmation");
    await inlineConfirmation.getByTestId("detach-task-confirm").click();

    await expect.poll(async () => (await apiClient.getTask(child.id)).parent_id ?? "").toBe("");
  });
});
