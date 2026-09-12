import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import {
  expectEmptyList,
  expectThreadsHome,
  saveStartupChoice,
  VIEW_STORAGE_KEY,
} from "./startup-page-helpers";

const APPEARANCE_PATH = "/settings/preferences/appearance";

test.describe("Threads Home default", () => {
  let baseline: Awaited<ReturnType<ApiClient["getUserSettings"]>>["settings"];
  let createdWorkspace: { id: string; name: string } | undefined;
  test.beforeEach(async ({ testPage, apiClient }) => {
    void testPage;
    baseline = (await apiClient.getUserSettings()).settings;
    createdWorkspace = undefined;
  });
  test.afterEach(async ({ apiClient }) => {
    if (baseline)
      await apiClient.saveUserSettings({
        startup_page: baseline.startup_page ?? "task_overview",
        workspace_id: baseline.workspace_id,
        workflow_filter_id: baseline.workflow_filter_id,
      });
    if (createdWorkspace)
      await apiClient.deleteWorkspace(createdWorkspace.id, createdWorkspace.name);
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.1, 003.3, 003.4, 003.5, 003.7
  test("saves Threads as the Home default independently of the last listing", async ({
    testPage,
    apiClient,
    seedData,
    browser,
    backend,
  }) => {
    await testPage.goto(APPEARANCE_PATH);
    const radio = testPage.getByRole("radio", { name: "Threads", exact: true });
    await expect(radio).toBeVisible();
    await testPage.evaluate(
      (key) => localStorage.setItem(key, JSON.stringify("list")),
      VIEW_STORAGE_KEY,
    );
    await radio.click();
    expect((await apiClient.getUserSettings()).settings.startup_page).toBe("task_overview");
    await saveStartupChoice(testPage);
    expect(await testPage.evaluate((key) => localStorage.getItem(key), VIEW_STORAGE_KEY)).toBe(
      '"list"',
    );
    await testPage.reload();
    await expect(radio).toBeChecked();
    await testPage.getByRole("link", { name: "Kandev home", exact: true }).click();
    await expectThreadsHome(testPage, seedData.workspaceId);

    await testPage.getByTestId("view-toggle-list").click();
    await expectEmptyList(testPage);
    await testPage.reload();
    await expectEmptyList(testPage);
    expect((await apiClient.getUserSettings()).settings.startup_page).toBe("threads");
    await testPage.getByRole("link", { name: "Home", exact: true }).click();
    await expectThreadsHome(testPage, seedData.workspaceId);
    await testPage.evaluate((key) => localStorage.removeItem(key), VIEW_STORAGE_KEY);
    await testPage.goto("/");
    await expectThreadsHome(testPage, seedData.workspaceId);

    const fresh = await browser.newContext({ baseURL: backend.frontendUrl });
    try {
      const page = await fresh.newPage();
      await page.addInitScript(() => localStorage.setItem("kandev.onboarding.completed", "true"));
      await page.goto(APPEARANCE_PATH);
      await expect(page.getByRole("radio", { name: "Threads", exact: true })).toBeChecked();
      expect(await page.evaluate((key) => localStorage.getItem(key), VIEW_STORAGE_KEY)).toBeNull();
      await page.goto(`/?workspaceId=${seedData.workspaceId}`);
      await expectThreadsHome(page, seedData.workspaceId);
    } finally {
      await fresh.close();
    }
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.2
  test("keeps Threads draft changes pending until saved", async ({ testPage, apiClient }) => {
    await testPage.goto(APPEARANCE_PATH);
    const threads = testPage.getByRole("radio", { name: "Threads", exact: true });
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await expect(threads).toBeVisible();
    await threads.click();
    await floatingSave.getByRole("button", { name: "Reset", exact: true }).click();
    await expect(testPage.getByRole("radio", { name: "Task overview", exact: true })).toBeChecked();
    await expect(floatingSave).not.toBeVisible();
    expect((await apiClient.getUserSettings()).settings.startup_page).toBe("task_overview");

    await threads.click();
    await testPage.route("**/api/v1/user/settings", async (route) => {
      const request = route.request();
      if (request.method() === "PATCH" && request.postDataJSON()?.startup_page === "threads") {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Test save failure" }),
        });
      } else await route.continue();
    });
    const failedSave = waitForHttp(testPage, "PATCH", /\/api\/v1\/user\/settings$/, {
      predicate: (response) => response.status() === 500,
    });
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await failedSave;
    await expect(floatingSave).toHaveAttribute("data-status", "error");
    await expect(threads).toBeChecked();
    expect((await apiClient.getUserSettings()).settings.startup_page).toBe("task_overview");
    await testPage.unroute("**/api/v1/user/settings");
    await saveStartupChoice(testPage);
    expect((await apiClient.getUserSettings()).settings.startup_page).toBe("threads");
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.4, 003.6, 003.7
  test("keeps every Home entry in the selected workspace", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    createdWorkspace = await apiClient.createWorkspace("Threads Home second workspace");
    const workflow = await apiClient.createWorkflow(
      createdWorkspace.id,
      "Threads second workflow",
      "simple",
    );
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const start = steps.find((step) => step.is_start_step);
    if (!start) throw new Error("Expected second workspace start step");
    await apiClient.seedTask(seedData.workspaceId, "First workspace only", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedTask(createdWorkspace.id, "Second workspace only", {
      workflow_id: workflow.id,
      workflow_step_id: start.id,
    });
    await testPage.goto(APPEARANCE_PATH);
    await testPage.getByRole("radio", { name: "Threads", exact: true }).click();
    await saveStartupChoice(testPage);
    await testPage.getByTestId("sidebar-settings-gear").click();
    await expectThreadsHome(testPage, seedData.workspaceId);
    await testPage.getByTestId("view-toggle-list").click();
    const rows = testPage.getByTestId("tasks-list-row");
    await expect(rows.filter({ hasText: "First workspace only" })).toBeVisible();
    await expect(rows.filter({ hasText: "Second workspace only" })).toHaveCount(0);

    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${createdWorkspace.id}`).click();
    await expectThreadsHome(testPage, createdWorkspace.id);
    await expect(testPage.getByTestId("threads-empty-state")).toBeVisible();
    await testPage.getByTestId("view-toggle-list").click();
    await expect(rows.filter({ hasText: "Second workspace only" })).toBeVisible();
    await expect(rows.filter({ hasText: "First workspace only" })).toHaveCount(0);

    await testPage.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
    const commands = testPage.getByRole("dialog");
    await commands.getByRole("combobox").fill("Go to Home");
    await commands
      .getByRole("option")
      .filter({ has: testPage.getByText("Go to Home", { exact: true }) })
      .click();
    await expectThreadsHome(testPage, createdWorkspace.id);
    await testPage.goto(APPEARANCE_PATH);
    await testPage.getByRole("link", { name: "Kandev home", exact: true }).click();
    await expectThreadsHome(testPage, createdWorkspace.id);
    await testPage.reload();
    await expectThreadsHome(testPage, createdWorkspace.id);
  });

  // @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.5
  test("preserves explicit destinations with a Threads Home default", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Threads explicit focus",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Expected a started session");
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "explicit target session finishes",
    );
    await testPage.goto(APPEARANCE_PATH);
    await testPage.getByRole("radio", { name: "Threads", exact: true }).click();
    await saveStartupChoice(testPage);
    await testPage.getByRole("link", { name: "Kandev home", exact: true }).click();
    await expectThreadsHome(testPage, seedData.workspaceId);
    const kanban = new KanbanPage(testPage);
    await testPage.getByTestId("view-toggle-kanban").click();
    await expect(kanban.board).toBeVisible();
    await testPage.reload();
    await expect(kanban.viewToggleKanban).toHaveAttribute("data-state", "on");
    await kanban.viewTogglePipeline.click();
    await testPage.reload();
    await expect(kanban.viewTogglePipeline).toHaveAttribute("data-state", "on");
    await kanban.viewToggleKanban.click();
    await testPage.getByTestId("view-toggle-list").click();
    await expect(testPage.getByTestId("tasks-list")).toBeVisible();
    await testPage.goto(`/?workspaceId=${seedData.workspaceId}&workflowId=${seedData.workflowId}`);
    await expect(kanban.board).toBeVisible();
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("workflowId") === seedData.workflowId,
    );
    const taskUrl = `/t/${task.id}?sessionId=${task.session_id}`;
    await testPage.goto(taskUrl);
    await expect(testPage.getByTestId("session-chat")).toBeVisible();
    await testPage.reload();
    await expect(testPage).toHaveURL((url) => url.pathname + url.search === taskUrl);
    const focusUrl = `/threads?workspace=${seedData.workspaceId}&taskId=${task.id}&sessionId=${task.session_id}`;
    await testPage.goto(focusUrl);
    await expect(testPage.getByTestId(`thread-column-${task.id}`)).toBeVisible();
    await testPage.reload();
    await expect(testPage).toHaveURL(
      (url) => url.searchParams.get("sessionId") === task.session_id,
    );
    await testPage.goBack();
    await expect(testPage).toHaveURL((url) => url.pathname + url.search === taskUrl);
    await testPage.goForward();
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/threads" && url.searchParams.get("taskId") === task.id,
    );
  });
});

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
