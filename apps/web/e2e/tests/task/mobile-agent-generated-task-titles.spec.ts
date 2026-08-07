import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { SessionPage } from "../../pages/session-page";

useRegularMode();

test.describe("Agent-generated task titles on mobile", () => {
  test("keeps the prompt-first task and subtask dialogs touch-safe", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const initial = await apiClient.getUserSettings();
    const initialEnabled = Boolean(initial.settings.agent_generated_task_titles);

    try {
      await apiClient.saveUserSettings({ agent_generated_task_titles: true });
      await testPage.goto("/settings/general/task-actions");
      await expect(
        testPage.getByRole("switch", { name: "Use the agent for new task titles" }),
      ).toBeChecked();

      const parent = await apiClient.seedTask(seedData.workspaceId, "Mobile title parent", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      await testPage.goto(`/t/${parent.task_id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      await testPage.getByTestId("mobile-session-menu").click();
      const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
      const taskRow = taskSheet
        .getByTestId("sidebar-task-item")
        .filter({ hasText: "Mobile title parent" });
      await taskRow.getByRole("button", { name: "Task actions" }).click();
      await testPage.getByRole("menuitem", { name: "Create Subtask", exact: true }).click();

      const subtaskDialog = testPage.getByTestId("new-subtask-dialog");
      await expect(subtaskDialog).toBeVisible();
      await expect(subtaskDialog.getByTestId("subtask-title-input")).toHaveCount(0);
      await expect(subtaskDialog.getByTestId("subtask-prompt-input")).toBeVisible();
      await subtaskDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    } finally {
      await apiClient.saveUserSettings({ agent_generated_task_titles: initialEnabled });
    }
  });
});
