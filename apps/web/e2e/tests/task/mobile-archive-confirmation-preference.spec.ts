import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Archive confirmation preference on mobile", () => {
  test("disabling confirmation archives immediately from the task switcher", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.goto("/settings/general/task-actions");
    const toggle = testPage.getByRole("switch", { name: "Confirm before archiving tasks" });
    await expect(toggle).toBeChecked();
    await toggle.click();
    await expect(toggle).not.toBeChecked();
    expect((await apiClient.getUserSettings()).settings.confirm_task_archive).toBe(true);
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.confirm_task_archive)
      .toBe(false);

    const taskOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const navTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile Archive Preference Nav",
      taskOptions,
    );
    await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile Archive Without Confirmation",
      taskOptions,
    );

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();

    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = sheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile Archive Without Confirmation" });
    await expect(taskRow).toBeVisible({ timeout: 15_000 });

    const actions = taskRow.getByRole("button", { name: "Task actions" });
    await expect(actions).toBeVisible();
    await actions.click();
    await testPage.getByRole("menuitem", { name: "Archive" }).click();

    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(taskRow).toHaveCount(0, { timeout: 15_000 });
  });
});
