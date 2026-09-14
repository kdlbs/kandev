import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";

test.describe("Sidebar personal task colors", () => {
  test("imports legacy colors on direct task startup and preserves existing colors", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const existingTask = await apiClient.seedTask(seedData.workspaceId, "Legacy existing color", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const importedTask = await apiClient.seedTask(seedData.workspaceId, "Legacy imported color", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const navTask = await apiClient.seedTask(seedData.workspaceId, "Legacy colors startup nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await apiClient.saveUserSettings({
      sidebar_task_color_patch: {
        colors: { [existingTask.task_id]: "blue" },
        if_missing: false,
      },
    });
    await testPage.addInitScript(
      ({ existingTaskId, importedTaskId }) => {
        window.localStorage.setItem(
          "kandev.taskColors",
          JSON.stringify({ [existingTaskId]: "pink", [importedTaskId]: "green" }),
        );
      },
      { existingTaskId: existingTask.task_id, importedTaskId: importedTask.task_id },
    );

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const sidebar = new SidebarTasksPage(testPage);
    await expect(sidebar.row(importedTask.task_id)).toBeVisible();
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        const colors = settings.sidebar_task_colors as Record<string, string | null> | undefined;
        return colors?.[importedTask.task_id];
      })
      .toBe("green");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        const colors = settings.sidebar_task_colors as Record<string, string | null> | undefined;
        return colors?.[existingTask.task_id];
      })
      .toBe("blue");
    await expect
      .poll(async () => testPage.evaluate(() => window.localStorage.getItem("kandev.taskColors")))
      .toBeNull();

    await expect(
      sidebar.row(importedTask.task_id).getByTestId("task-item-color-marker"),
    ).toHaveAttribute("data-color-token", "green");
    await expect(
      sidebar.row(existingTask.task_id).getByTestId("task-item-color-marker"),
    ).toHaveAttribute("data-color-token", "blue");
  });

  test("saves a manual color and restores it after a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.seedTask(seedData.workspaceId, "Desktop manual color task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const navTask = await apiClient.seedTask(seedData.workspaceId, "Manual colors desktop nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const sidebar = new SidebarTasksPage(testPage);
    const row = sidebar.row(task.task_id);
    const marker = row.getByTestId("task-item-color-marker");
    await expect(row).toBeVisible();
    await expect(marker).toHaveCount(0);

    await sidebar.rightClick(task.task_id);
    const colorMenu = testPage.getByRole("menuitem", { name: /color/i });
    await expect(colorMenu).toBeVisible();
    await colorMenu.hover();
    const blueMenuItem = testPage.getByRole("menuitem", { name: "Blue", exact: true });
    await expect(blueMenuItem).toBeVisible();
    await blueMenuItem.click();

    await expect(marker).toHaveAttribute("data-color-token", "blue");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        const colors = settings.sidebar_task_colors as Record<string, string | null> | undefined;
        return colors?.[task.task_id];
      })
      .toBe("blue");

    await testPage.reload();
    await session.waitForLoad();
    await expect(sidebar.row(task.task_id).getByTestId("task-item-color-marker")).toHaveAttribute(
      "data-color-token",
      "blue",
    );
    await expect(testPage).toHaveURL(new RegExp(`/t/${navTask.task_id}$`));
    expect(await testPage.evaluate(() => localStorage.getItem("kandev.taskColors"))).toBeNull();
  });
});
