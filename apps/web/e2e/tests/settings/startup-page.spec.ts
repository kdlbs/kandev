import { test, expect } from "../../fixtures/test-base";

const APPEARANCE_PATH = "/settings/general/appearance";

test.describe("Startup page", () => {
  test("saves the last-task choice, resumes it at bare home, and keeps Home on the overview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await apiClient.createTask(seedData.workspaceId, "Startup Page Resume Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    try {
      await testPage.goto(`/t/${task.id}`);
      await expect(
        testPage.getByText("Startup Page Resume Task", { exact: true }).first(),
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
      const lastTaskRadio = testPage.getByRole("radio", { name: "Last visited task" });
      await expect(lastTaskRadio).toBeVisible({ timeout: 15_000 });
      await lastTaskRadio.click();

      const card = testPage.getByTestId("startup-page-settings-card");
      await expect(card).toHaveAttribute("data-settings-dirty", "true");
      expect((await apiClient.getUserSettings()).settings.startup_page).toBe("task_overview");

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
      expect((await apiClient.getUserSettings()).settings.startup_page).toBe("last_task");

      await testPage.reload();
      await expect(lastTaskRadio).toBeChecked({ timeout: 15_000 });

      await testPage.goto("/");
      await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`), { timeout: 15_000 });

      await testPage.getByRole("link", { name: "Home", exact: true }).click();
      await expect(testPage).toHaveURL(
        (url) => url.pathname === "/" && url.searchParams.get("home") === "overview",
      );
      await expect(testPage.getByTestId("kanban-board")).toBeVisible({ timeout: 15_000 });
    } finally {
      await apiClient.saveUserSettings({ startup_page: "task_overview" });
    }
  });

  test("falls back to the task overview when this device has no task in the active workspace", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);
    try {
      await apiClient.saveUserSettings({ startup_page: "last_task" });
      await testPage.goto("/?home=overview");
      await testPage.evaluate(() => {
        window.localStorage.setItem(
          "kandev.recentTasks.v1",
          JSON.stringify([
            {
              taskId: "other-workspace-task",
              title: "Other workspace task",
              visitedAt: "2026-07-31T12:00:00.000Z",
              workspaceId: "other-workspace",
            },
          ]),
        );
      });

      await testPage.goto("/");
      await expect(testPage).not.toHaveURL(/\/t\//);
      await expect(testPage.getByTestId("kanban-board")).toBeVisible({ timeout: 15_000 });
    } finally {
      await apiClient.saveUserSettings({ startup_page: "task_overview" });
      await testPage.evaluate(() => window.localStorage.removeItem("kandev.recentTasks.v1"));
    }
  });
});
