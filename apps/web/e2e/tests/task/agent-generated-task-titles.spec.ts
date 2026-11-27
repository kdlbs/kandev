import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { restoreSidebarLayout } from "../../helpers/sidebar-layout";
import { AppSidebarPage } from "../../pages/app-sidebar-page";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

test.describe("Agent-generated task titles", () => {
  test("hides the new-task title and persists a six-word provisional title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialEnabled = Boolean(initial.settings.agent_generated_task_titles);
    const initialLayout = initial.settings.sidebar_layouts_by_workspace?.[seedData.workspaceId];

    try {
      await testPage.goto("/settings/preferences/task-behavior");
      const toggle = testPage.getByRole("switch", { name: "Use the agent for new task titles" });
      if (!(await toggle.isChecked())) await toggle.click();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();
      await expect
        .poll(async () =>
          Boolean((await apiClient.getUserSettings()).settings.agent_generated_task_titles),
        )
        .toBe(true);

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await new AppSidebarPage(testPage).expandNavigationIfCollapsed();
      await kanban.createTaskButton.first().click();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("task-title-input")).toHaveCount(0);
      await dialog
        .getByTestId("task-description-input")
        .fill("Design a small login recovery flow with tests and docs");
      await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
      await dialog.getByTestId("submit-start-agent").click();

      await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
      const taskID = testPage.url().match(/\/t\/([^/?]+)/)?.[1];
      expect(taskID).toBeTruthy();
      await expect
        .poll(async () => {
          const task = await apiClient.getTask(taskID!);
          return (
            task.title === "Design a small login recovery flow" ||
            task.metadata?.agent_title_pending !== true
          );
        })
        .toBe(true);
      const task = await apiClient.getTask(taskID!);
      expect(task.title).toBeTruthy();
      expect(
        task.title === "Design a small login recovery flow" ||
          task.metadata?.agent_title_pending !== true,
      ).toBe(true);
    } finally {
      await apiClient.saveUserSettings({ agent_generated_task_titles: initialEnabled });
      await restoreSidebarLayout(apiClient, seedData.workspaceId, initialLayout);
    }
  });
});
