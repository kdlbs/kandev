import { test, expect } from "../../fixtures/test-base";
import { expectMenuBackdropCount, expectMobileMenuBackdrop } from "../../helpers/menu-backdrop";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Menu backdrop viewport boundaries", () => {
  // @covers AC-UI-MOBILE-TASK-NAVIGATION-001.11, AC-UI-MOBILE-TASK-NAVIGATION-001.12
  test("menu backdrop follows the 640px boundary while open", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 639, height: 900 });
    const task = await apiClient.createTask(seedData.workspaceId, "Backdrop boundary task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.taskCard(task.id).getByRole("button", { name: "More options" }).click();
    const menu = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expectMobileMenuBackdrop(menu);
    await expectMenuBackdropCount(testPage, 1);

    await testPage.setViewportSize({ width: 640, height: 900 });
    await expect(menu).toBeVisible();
    await expectMenuBackdropCount(testPage, 0);
    const desktopBox = await menu.boundingBox();
    expect(desktopBox!.width).toBeLessThan(400);
    await testPage.setViewportSize({ width: 639, height: 900 });
    await expectMobileMenuBackdrop(menu);
    await expectMenuBackdropCount(testPage, 1);
    await testPage.keyboard.press("Escape");
    await expectMenuBackdropCount(testPage, 0);
  });

  // @covers AC-UI-MOBILE-TASK-NAVIGATION-001.12
  test("desktop menus keep anchored actions without a backdrop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Desktop backdrop task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCard(task.id);
    await card.hover();
    const trigger = card.getByRole("button", { name: "More options" });
    await trigger.click();
    const dropdown = testPage.locator('[data-slot="dropdown-menu-content"][data-state="open"]');
    await expect(dropdown).toBeVisible();
    await expectMenuBackdropCount(testPage, 0);
    expect((await dropdown.boundingBox())!.width).toBeLessThan(400);
    await testPage.keyboard.press("Escape");
    await expect(trigger).toBeFocused();

    await card.click({ button: "right" });
    const contextMenu = testPage.locator('[data-slot="context-menu-content"][data-state="open"]');
    await expect(contextMenu).toBeVisible();
    await expectMenuBackdropCount(testPage, 0);
    expect((await contextMenu.boundingBox())!.width).toBeLessThan(400);
    await contextMenu.getByTestId("task-context-priority").hover();
    await testPage.getByTestId("task-context-priority-low").click();
    await expect.poll(async () => (await apiClient.getTask(task.id)).priority).toBe("low");
    await expect(contextMenu).toBeHidden();
    await expectMenuBackdropCount(testPage, 0);
  });
});
