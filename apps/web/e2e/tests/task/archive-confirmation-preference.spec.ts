import { test, expect } from "../../fixtures/test-base";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";

test.describe("Archive confirmation preference", () => {
  test("keeps the full archive warning subordinate to dialog copy", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const title = "Full archive warning geometry target";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/background 60s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await apiClient.seedTask(seedData.workspaceId, "Full archive warning child", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: task.id,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await prCapture.screenshot("before-full-archive-dialog", {
      caption: "In-flight task before opening full archive confirmation",
    });

    const taskRow = session.sidebarTaskItem(title);
    await expect(taskRow).toBeVisible({ timeout: 15_000 });
    const taskActions = taskRow.getByRole("button", { name: "Task actions" });
    await expect(taskActions).toBeVisible();
    await taskActions.click({ force: true });
    const archiveMenuItem = testPage.getByRole("menuitem", { name: "Archive", exact: true });
    await expect(archiveMenuItem).toBeVisible();
    await archiveMenuItem.click();
    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("Clean Git worktrees are removed.");
    await expect(dialog).toContainText("Unpublished local branches remain available.");
    const warning = dialog.getByTestId("still-working-warning");
    await expect(warning).toBeVisible();
    await prCapture.screenshot("full-archive-dialog-warning", {
      caption: "Compact still-working warning in full archive dialog",
    });

    const hierarchy = await dialog.evaluate((dialogElement) => {
      const warning = dialogElement.querySelector<HTMLElement>(
        '[data-testid="still-working-warning"]',
      );
      const description = dialogElement.querySelector<HTMLElement>(
        '[data-slot="alert-dialog-description"]',
      );
      if (!warning || !description) throw new Error("dialog hierarchy targets missing");
      const warningStyle = getComputedStyle(warning);
      const descriptionStyle = getComputedStyle(description);
      const warningBox = warning.getBoundingClientRect();
      const dialogBox = dialogElement.getBoundingClientRect();
      return {
        warningFontSize: warningStyle.fontSize,
        warningLineHeight: warningStyle.lineHeight,
        descriptionFontSize: descriptionStyle.fontSize,
        warning: { left: warningBox.left, right: warningBox.right },
        dialog: { left: dialogBox.left, right: dialogBox.right },
        documentWidth: document.documentElement.scrollWidth,
        viewportWidth: document.documentElement.clientWidth,
      };
    });
    expect(Number.parseFloat(hierarchy.warningFontSize)).toBeLessThan(
      Number.parseFloat(hierarchy.descriptionFontSize),
    );
    expect(hierarchy.warningLineHeight).toBe("20px");
    expect(hierarchy.warning.left).toBeGreaterThanOrEqual(hierarchy.dialog.left);
    expect(hierarchy.warning.right).toBeLessThanOrEqual(hierarchy.dialog.right);
    expect(hierarchy.documentWidth).toBeLessThanOrEqual(hierarchy.viewportWidth);

    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toBeHidden();
  });

  test("disabling confirmation archives immediately from the desktop sidebar", async ({
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
