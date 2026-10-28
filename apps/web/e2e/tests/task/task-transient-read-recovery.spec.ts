import type { Route } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";

function createTemporaryTaskReadFailure(limit: number) {
  let failedReads = 0;
  let markFailuresReturned: () => void = () => {};
  const allFailuresReturned = new Promise<void>((resolve) => {
    markFailuresReturned = resolve;
  });
  const handler = async (route: Route) => {
    if (route.request().method() !== "GET" || failedReads >= limit) {
      await route.continue();
      return;
    }
    failedReads++;
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ code: "persistence_unavailable" }),
    });
    if (failedReads === limit) markFailuresReturned();
  };
  return {
    handler,
    allFailuresReturned,
    getFailedReads: () => failedReads,
  };
}

test.describe("Task transient read recovery", () => {
  test("shows and recovers the initial temporary task-read failure", async ({
    testPage,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const taskId = "task-navigation-initial-read-recovery";
    const taskPath = new RegExp(`/api/v1/tasks/${taskId}$`);
    const sessionPath = new RegExp(`/api/v1/tasks/${taskId}/sessions$`);
    const recoveredTask = {
      id: taskId,
      title: "Recovered initial task",
      description: "",
      workspace_id: seedData.workspaceId,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      primary_session_id: null,
      state: "COMPLETED",
      archived_at: "2026-10-06T00:00:00Z",
      priority: "medium",
      position: 0,
      repositories: [],
    };
    let failedReads = 0;
    let markFailuresReturned: () => void = () => {};
    const allFailuresReturned = new Promise<void>((resolve) => {
      markFailuresReturned = resolve;
    });
    let markRecoveryStarted: () => void = () => {};
    const recoveryStarted = new Promise<void>((resolve) => {
      markRecoveryStarted = resolve;
    });
    let releaseRecovery: () => void = () => {};
    const recoveryGate = new Promise<void>((resolve) => {
      releaseRecovery = resolve;
    });
    await testPage.route(taskPath, async (route: Route) => {
      if (route.request().method() !== "GET") return route.continue();
      if (failedReads < 5) {
        failedReads++;
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ code: "persistence_unavailable" }),
        });
        if (failedReads === 5) markFailuresReturned();
        return;
      }
      markRecoveryStarted();
      await recoveryGate;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(recoveredTask),
      });
    });
    await testPage.route(sessionPath, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ sessions: [], total: 0 }),
      }),
    );

    await testPage.goto(`/t/${taskId}`);
    await allFailuresReturned;
    const errorState = testPage.getByTestId("task-load-error-state");
    await expect(errorState).toBeVisible();
    await expect(errorState).toContainText("Task temporarily unavailable");
    await expect(testPage.getByTestId("task-read-retry")).toBeVisible();
    await expect(testPage.getByTestId("task-unavailable-overview-link")).toBeVisible();
    expect(failedReads).toBe(5);
    await testInfo.attach("desktop-task-initial-temporary-read-failure", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });

    const retry = testPage.getByTestId("task-read-retry");
    const recoveredTaskRead = waitForHttp(testPage, "GET", taskPath);
    await retry.click();
    try {
      await recoveryStarted;
      await expect(retry).toBeDisabled();
      await expect(retry).toHaveAccessibleName("Retry");
      await expect(testPage.getByText("Retrying...", { exact: true })).toBeVisible();
    } finally {
      releaseRecovery();
    }
    expect((await recoveredTaskRead).ok()).toBe(true);
    await expect(errorState).toHaveCount(0);
  });

  test("keeps loaded details and the selected secondary session through a temporary refresh failure", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task navigation retry fixture",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("Task fixture has no primary session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible();
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible();

    let secondarySessionId: string | undefined;
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          secondarySessionId = sessions.find((item) => item.id !== task.session_id)?.id;
          return secondarySessionId ?? null;
        },
        { timeout: 30_000, message: "Waiting for the secondary task session" },
      )
      .toBeTruthy();
    const selectedSessionId = secondarySessionId;
    if (!selectedSessionId) throw new Error("Task fixture has no secondary session");
    await session.sessionTabBySessionId(selectedSessionId).click();
    await expect
      .poll(() =>
        testPage.evaluate(
          () =>
            (
              window as unknown as {
                __KANDEV_E2E_STORE__?: {
                  getState: () => { tasks: { activeSessionId: string | null } };
                };
              }
            ).__KANDEV_E2E_STORE__?.getState().tasks.activeSessionId,
        ),
      )
      .toBe(selectedSessionId);

    const draft = "Keep this unsent task recovery draft";
    const editor = await session.composerReady();
    await editor.fill(draft);
    await expect(editor).toHaveText(draft);

    const workspaceLayout = testPage.getByTestId("dockview-task-layout");
    const beforeRefresh = await workspaceLayout.boundingBox();
    expect(beforeRefresh).not.toBeNull();

    const taskPath = new RegExp(`/api/v1/tasks/${task.id}$`);
    const {
      handler: temporaryFailure,
      allFailuresReturned,
      getFailedReads,
    } = createTemporaryTaskReadFailure(5);
    await testPage.route(taskPath, temporaryFailure);
    try {
      const firstTaskRead = waitForHttp(testPage, "GET", taskPath);
      await testPage.evaluate(() => window.dispatchEvent(new Event("online")));
      const firstFailure = await firstTaskRead;
      expect(firstFailure.status()).toBe(503);
      await allFailuresReturned;
      const retryNotice = testPage.getByTestId("task-read-recovery-notice");
      await expect(retryNotice).toBeVisible();
      await expect(retryNotice).toContainText("Task details could not be refreshed");
      const noticeBox = await retryNotice.boundingBox();
      const noticeLayoutBox = await workspaceLayout.boundingBox();
      expect(noticeBox).not.toBeNull();
      expect(noticeLayoutBox).not.toBeNull();
      if (noticeBox && noticeLayoutBox) {
        expect(noticeLayoutBox.y).toBeGreaterThanOrEqual(noticeBox.y + noticeBox.height - 1);
      }
      await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
      expect(getFailedReads()).toBe(5);
      await testInfo.attach("desktop-task-temporary-refresh-failure", {
        body: await testPage.screenshot(),
        contentType: "image/png",
      });

      const recoveredTaskRead = waitForHttp(testPage, "GET", taskPath);
      await testPage.getByTestId("task-read-retry").click();
      const recoveredTaskResponse = await recoveredTaskRead;
      expect(recoveredTaskResponse.ok()).toBe(true);
      await expect(retryNotice).toHaveCount(0);
      await expect
        .poll(async () => (await workspaceLayout.boundingBox())?.y)
        .toBeCloseTo(beforeRefresh!.y, 1);
      await expect
        .poll(async () => (await workspaceLayout.boundingBox())?.height)
        .toBeCloseTo(beforeRefresh!.height, 1);
      await expect
        .poll(() =>
          testPage.evaluate(
            () =>
              (
                window as unknown as {
                  __KANDEV_E2E_STORE__?: { getState: () => { tasks: { activeSessionId: string } } };
                }
              ).__KANDEV_E2E_STORE__?.getState().tasks.activeSessionId,
          ),
        )
        .toBe(selectedSessionId);
      await expect(editor).toHaveText(draft);
    } finally {
      await testPage.unroute(taskPath, temporaryFailure);
    }
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.8
  test("recovers automatically after a thirteen-second persistence outage", async ({
    testPage,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const taskId = "task-navigation-timed-read-recovery";
    const taskPath = new RegExp(`/api/v1/tasks/${taskId}$`);
    const sessionPath = new RegExp(`/api/v1/tasks/${taskId}/sessions$`);
    const recoveredTask = {
      id: taskId,
      title: "Recovered timed task",
      description: "",
      workspace_id: seedData.workspaceId,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      primary_session_id: null,
      state: "COMPLETED",
      archived_at: "2026-10-06T00:00:00Z",
      priority: "medium",
      position: 0,
      repositories: [],
    };
    let firstReadAt: number | null = null;
    let failedReads = 0;
    await testPage.route(taskPath, async (route: Route) => {
      if (route.request().method() !== "GET") return route.continue();
      firstReadAt ??= Date.now();
      if (Date.now() - firstReadAt < 13_000) {
        failedReads++;
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ code: "persistence_unavailable" }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(recoveredTask),
      });
    });
    await testPage.route(sessionPath, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ sessions: [], total: 0 }),
      }),
    );

    const recoveredRead = waitForHttp(testPage, "GET", taskPath, {
      timeout: 30_000,
      predicate: (response) => response.ok(),
    });
    await testPage.goto(`/t/${taskId}`);

    const errorState = testPage.getByTestId("task-load-error-state");
    await expect(errorState).toBeVisible();
    await expect(testPage.getByTestId("task-unavailable-overview-link")).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Retry" })).toBeDisabled();

    expect((await recoveredRead).ok()).toBe(true);
    await expect(errorState).toHaveCount(0);
    await expect(testPage.getByTestId("task-unavailable-overview-link")).toHaveCount(0);
    expect(failedReads).toBe(3);
    await testInfo.attach("desktop-task-timed-read-recovery", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });
  });

  // @covers AC-PLATFORM-INTERACTIVE-READS-005.7
  test("retains the current task projection through an exhausted temporary read", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(180_000);
    const title = "Projection task read recovery";
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await testPage.goto("/");
    const sidebar = new SidebarTasksPage(testPage);
    await expect(sidebar.row(task.id)).toBeVisible({ timeout: 30_000 });

    const taskPath = new RegExp(`/api/v1/tasks/${task.id}$`);
    const {
      handler: temporaryFailure,
      allFailuresReturned,
      getFailedReads,
    } = createTemporaryTaskReadFailure(5);
    await testPage.route(taskPath, temporaryFailure);
    try {
      const firstRead = waitForHttp(testPage, "GET", taskPath);
      await sidebar.plainClick(task.id);
      await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
      expect((await firstRead).status()).toBe(503);

      const taskTitle = testPage.getByText(title, { exact: true }).first();
      await expect(taskTitle).toBeVisible();
      await allFailuresReturned;

      const retryNotice = testPage.getByTestId("task-read-recovery-notice");
      await expect(retryNotice).toBeVisible();
      await expect(retryNotice).toContainText("Task details could not be refreshed");
      const retry = retryNotice.getByTestId("task-read-retry");
      await expect(retry).toBeEnabled();
      await expect(taskTitle).toBeVisible();
      await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
      expect(getFailedReads()).toBe(5);
      await testInfo.attach("desktop-task-projection-read-recovery", {
        body: await testPage.screenshot(),
        contentType: "image/png",
      });

      const recoveredRead = waitForHttp(testPage, "GET", taskPath);
      await retry.click();
      expect((await recoveredRead).ok()).toBe(true);
      await expect(retryNotice).toHaveCount(0);
      await expect(taskTitle).toBeVisible();
    } finally {
      await testPage.unroute(taskPath, temporaryFailure);
    }
  });
});
