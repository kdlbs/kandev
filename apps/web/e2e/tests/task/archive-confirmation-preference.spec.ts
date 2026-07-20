import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Archive confirmation preference", () => {
  test("disabling confirmation archives immediately from the desktop sidebar", async ({
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
      "Archive Preference Nav",
      taskOptions,
    );
    await apiClient.seedTask(seedData.workspaceId, "Archive Without Confirmation", taskOptions);

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.taskInSidebar("Archive Without Confirmation")).toBeVisible({
      timeout: 15_000,
    });

    await session.openSidebarMenuAndClick("Archive Without Confirmation", "Archive");

    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(session.taskInSidebar("Archive Without Confirmation")).toHaveCount(0, {
      timeout: 15_000,
    });
  });
});
