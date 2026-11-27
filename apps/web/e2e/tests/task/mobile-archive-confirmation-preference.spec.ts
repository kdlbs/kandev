import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

test.describe("Archive confirmation preference on mobile", () => {
  let previousConfirmation = true;
  test.beforeEach(async ({ testPage, apiClient }) => {
    await testPage.goto("/");
    previousConfirmation =
      (await apiClient.getUserSettings()).settings.confirm_task_archive !== false;
    await apiClient.saveUserSettings({ confirm_task_archive: true });
  });
  test.afterEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({ confirm_task_archive: previousConfirmation });
  });

  test("confirmation describes local-change deletion before archiving a worktree", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const options = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
    const nav = await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile archive navigation",
      options,
    );
    const title = "Mobile worktree archive consequences";
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      ...options,
      agent_profile_id: seedData.agentProfileId,
      prepare_session: true,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    await testPage.goto(`/t/${nav.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").click();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const row = sheet.getByTestId("sidebar-task-item").filter({ hasText: title });
    await expect(row).toHaveCount(1);
    await row.getByRole("button", { name: "Task actions" }).click();
    await testPage.getByRole("menuitem", { name: "Archive", exact: true }).click();
    const confirmation = testPage.getByRole("dialog", { name: /Archive task/ });
    await expect(confirmation).toContainText(
      "Git worktrees and local changes are deleted on archive.",
    );
    await expect(confirmation).not.toContainText("stays until it is clean");
    await testPage.screenshot({ path: testInfo.outputPath("archive-deletion-phone.png") });
    const archived = waitForHttp(testPage, "POST", new RegExp(`/api/v1/tasks/${task.id}/archive$`));
    await confirmation.getByRole("button", { name: "Archive", exact: true }).click();
    expect((await archived).ok()).toBe(true);
    await expect(confirmation).toBeHidden();
    await expect(row).toHaveCount(0);
    await testPage.reload();
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").click();
    await expect(sheet).toBeVisible();
    await expect(row).toHaveCount(0);
  });

  test("disabling confirmation archives immediately from the task switcher", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.goto("/settings/preferences/task-behavior");
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
    const task = await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile Archive Without Confirmation",
      taskOptions,
    );

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").click();

    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    const taskRow = sheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile Archive Without Confirmation" });
    await expect(taskRow).toBeVisible({ timeout: 15_000 });

    const actions = taskRow.getByRole("button", { name: "Task actions" });
    await expect(actions).toBeVisible();
    await actions.click();
    const archived = waitForHttp(
      testPage,
      "POST",
      new RegExp(`/api/v1/tasks/${task.task_id}/archive$`),
    );
    await testPage.getByRole("menuitem", { name: "Archive" }).click();
    expect((await archived).ok()).toBe(true);

    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(taskRow).toHaveCount(0);
  });
});
