import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import {
  expectEmptyList,
  expectThreadsHome,
  saveStartupChoice,
  VIEW_STORAGE_KEY,
} from "./startup-page-helpers";

const APPEARANCE_PATH = "/settings/preferences/appearance";

test.describe("Mobile Threads Home default", () => {
  let baseline: Awaited<ReturnType<ApiClient["getUserSettings"]>>["settings"];
  test.beforeEach(async ({ testPage, apiClient }) => {
    void testPage;
    baseline = (await apiClient.getUserSettings()).settings;
  });
  test.afterEach(async ({ apiClient }) => {
    if (baseline)
      await apiClient.saveUserSettings({ startup_page: baseline.startup_page ?? "task_overview" });
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.1, 003.4, 003.6, 003.8
  test("saves Threads from phone settings and returns Home after using List", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileMenuButton.tap();
    await testPage
      .getByRole("dialog", { name: "Menu" })
      .getByRole("link", { name: "Settings", exact: true })
      .tap();
    await testPage.getByRole("link", { name: "Appearance", exact: true }).tap();
    const row = testPage.locator('label[for="startup-page-threads"]');
    await expect(row).toBeVisible();
    expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await row.tap();
    const radio = testPage.getByRole("radio", { name: "Threads", exact: true });
    await expect(radio).toBeChecked();
    const saveButton = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(saveButton).toBeVisible();
    const box = await saveButton.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.y + box!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
    // A transient update toast may cover the action before auto-dismissal.
    await expect
      .poll(() =>
        saveButton.evaluate((button) => {
          const bounds = button.getBoundingClientRect();
          return button.contains(
            document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2),
          );
        }),
      )
      .toBe(true);
    await testPage.screenshot({ path: testInfo.outputPath("threads-home-settings-phone.png") });
    await saveStartupChoice(testPage, true);
    await testPage.reload();
    await expect(radio).toBeChecked();
    await testPage.getByRole("link", { name: "Home", exact: true }).tap();
    await expectThreadsHome(testPage, seedData.workspaceId);
    await mobile.mobileMenuButton.tap();
    await testPage
      .getByRole("dialog", { name: "Menu" })
      .getByRole("radio", { name: "List", exact: true })
      .tap();
    await expectEmptyList(testPage);
    await testPage.reload();
    await expectEmptyList(testPage);
    await mobile.mobileMenuButton.tap();
    await testPage
      .getByRole("dialog", { name: "Menu" })
      .getByRole("link", { name: "Home", exact: true })
      .tap();
    await expectThreadsHome(testPage, seedData.workspaceId);
    expect((await apiClient.getUserSettings()).settings.startup_page).toBe("threads");
    await testPage.evaluate((key) => localStorage.removeItem(key), VIEW_STORAGE_KEY);
    await testPage.goto("/");
    await expectThreadsHome(testPage, seedData.workspaceId);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await testPage.screenshot({ path: testInfo.outputPath("threads-home-deck-phone.png") });
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.5, 003.7, 003.8
  test("keeps explicit phone destinations with Threads selected", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Phone explicit Threads",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Expected a started session");
    await waitForSessionDone(apiClient, task.id, task.session_id, "phone target finishes");
    await testPage.goto(APPEARANCE_PATH);
    await testPage.locator('label[for="startup-page-threads"]').tap();
    await saveStartupChoice(testPage, true);
    await testPage.goto(`/tasks?workspace=${seedData.workspaceId}`);
    await expect(testPage.getByTestId("tasks-list")).toBeVisible();
    await testPage.goto(`/t/${task.id}?sessionId=${task.session_id}`);
    await expect(testPage.getByTestId("session-chat")).toBeVisible();
    await testPage.getByRole("link", { name: "Task overview", exact: true }).tap();
    await expect(testPage).toHaveURL((url) => url.pathname === "/tasks");
    await expect(testPage.getByTestId("tasks-list")).toBeVisible();
    await testPage.goto(
      `/threads?workspace=${seedData.workspaceId}&taskId=${task.id}&sessionId=${task.session_id}`,
    );
    const column = testPage.getByTestId(`thread-column-${task.id}`);
    await expect(column).toBeVisible();
    const width = (await column.boundingBox())!.width;
    expect(width).toBeGreaterThan(testPage.viewportSize()!.width * 0.7);
    expect(width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
    await testPage.reload();
    await expect(column).toBeVisible();
    await expect(testPage).toHaveURL(
      (url) =>
        url.searchParams.get("taskId") === task.id &&
        url.searchParams.get("sessionId") === task.session_id,
    );
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await testPage.screenshot({ path: testInfo.outputPath("threads-home-focused-phone.png") });
  });
});

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
