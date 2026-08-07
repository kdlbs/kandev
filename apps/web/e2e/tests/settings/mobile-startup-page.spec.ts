import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const APPEARANCE_PATH = "/settings/general/appearance";

test.describe("Mobile startup page", () => {
  test("uses a touch-friendly saved preference, resumes the task, and returns to overview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 390, height: 844 });
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Startup Resume Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    try {
      await testPage.goto(`/t/${task.id}`);
      await expect(
        testPage.locator("header").getByText("Mobile Startup Resume Task", { exact: true }),
      ).toBeVisible({
        timeout: 15_000,
      });
      await expect
        .poll(
          () =>
            testPage.evaluate(
              ({ taskId, workspaceId }) => {
                const entries = JSON.parse(
                  window.localStorage.getItem("kandev.recentTasks.v1") ?? "[]",
                ) as Array<{ taskId?: string; workspaceId?: string }>;
                return entries.some(
                  (entry) => entry.taskId === taskId && entry.workspaceId === workspaceId,
                );
              },
              { taskId: task.id, workspaceId: seedData.workspaceId },
            ),
          { timeout: 15_000 },
        )
        .toBe(true);

      await testPage.goto(APPEARANCE_PATH);
      const lastTaskRow = testPage.locator('label[for="startup-page-last_task"]');
      const lastTaskRadio = testPage.getByRole("radio", { name: "Last visited task" });
      await expect(lastTaskRow).toBeVisible({ timeout: 15_000 });
      const rowBox = await lastTaskRow.boundingBox();
      expect(rowBox).not.toBeNull();
      expect(rowBox!.height).toBeGreaterThanOrEqual(44);
      await lastTaskRow.tap();
      await expect(lastTaskRadio).toBeChecked();

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).tap();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
      await testPage.reload();
      await expect(lastTaskRadio).toBeChecked({ timeout: 15_000 });

      await testPage.goto("/");
      await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`), { timeout: 15_000 });

      await testPage.getByRole("link", { name: "Task overview" }).tap();
      const mobile = new MobileKanbanPage(testPage);
      await expect(testPage).toHaveURL(
        (url) => url.pathname === "/" && url.searchParams.get("home") === "overview",
      );
      await expect(mobile.mobileKanbanLayout()).toBeVisible({ timeout: 15_000 });
      expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
        await testPage.evaluate(() => document.documentElement.clientWidth),
      );
    } finally {
      await apiClient.saveUserSettings({ startup_page: "task_overview" });
    }
  });
});
