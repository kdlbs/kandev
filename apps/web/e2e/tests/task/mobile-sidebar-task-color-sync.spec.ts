import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar personal task colors", () => {
  test("displays the server-backed manual color in the drawer after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, "Mobile manual color task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const navTask = await apiClient.seedTask(seedData.workspaceId, "Manual colors mobile nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.saveUserSettings({
      sidebar_task_color_patch: {
        colors: { [task.task_id]: "purple" },
        if_missing: false,
      },
    });

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.mobileSessionMenu.tap();

    const drawer = testPage
      .getByTestId("mobile-task-switcher-list")
      .locator("xpath=ancestor::*[@data-slot='drawer-content'][1]");
    await expect(drawer).toBeVisible();
    const marker = drawer
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile manual color task" })
      .getByTestId("task-item-color-marker");
    await expect(marker).toHaveAttribute("data-color-token", "purple");

    await testPage.reload();
    await session.waitForLoad();
    await session.mobileSessionMenu.tap();
    const reloadedDrawer = testPage
      .getByTestId("mobile-task-switcher-list")
      .locator("xpath=ancestor::*[@data-slot='drawer-content'][1]");
    await expect(reloadedDrawer).toBeVisible();
    await expect(
      reloadedDrawer
        .getByTestId("sidebar-task-item")
        .filter({ hasText: "Mobile manual color task" })
        .getByTestId("task-item-color-marker"),
    ).toHaveAttribute("data-color-token", "purple");
  });
});
