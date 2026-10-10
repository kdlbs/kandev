import type { Route } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

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
      if (failedReads < 3) {
        failedReads++;
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ code: "persistence_unavailable" }),
        });
        if (failedReads === 3) markFailuresReturned();
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
    expect(failedReads).toBe(3);
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

    const workspaceLayout = testPage.getByTestId("dockview-task-layout");
    const beforeRefresh = await workspaceLayout.boundingBox();
    expect(beforeRefresh).not.toBeNull();

    const taskPath = new RegExp(`/api/v1/tasks/${task.id}$`);
    let failedReads = 0;
    let markFailuresReturned: () => void = () => {};
    const allFailuresReturned = new Promise<void>((resolve) => {
      markFailuresReturned = resolve;
    });
    const temporaryFailure = async (route: Route) => {
      if (route.request().method() !== "GET" || failedReads >= 3) {
        await route.continue();
        return;
      }
      failedReads++;
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ code: "persistence_unavailable" }),
      });
      if (failedReads === 3) markFailuresReturned();
    };
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
      await expect
        .poll(async () => (await workspaceLayout.boundingBox())?.y)
        .toBeCloseTo(beforeRefresh!.y, 1);
      await expect
        .poll(async () => (await workspaceLayout.boundingBox())?.height)
        .toBeCloseTo(beforeRefresh!.height, 1);
      await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
      expect(failedReads).toBe(3);
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
    } finally {
      await testPage.unroute(taskPath, temporaryFailure);
    }
  });
});
