import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionState } from "../../helpers/session";
import { seedPluginExecutorStatusTask } from "../../helpers/plugin-executor-status";
import { uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  createLongRunningSession,
  holdNextRunningTaskSessionRead,
  matchesTaskSessionRead,
} from "../../helpers/session-refresh-efficiency";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";

test.describe("session refresh efficiency", () => {
  test("revalidates an unchanged session without a response body", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await createLongRunningSession(
      apiClient,
      seedData,
      `Conditional session read ${Date.now()}`,
    );
    expect(task.session_id).toBeTruthy();
    const sessionId = task.session_id!;
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "RUNNING",
      message: "Waiting for the conditional-read fixture to run",
    });

    const initialRead = testPage.waitForResponse(
      (response) => matchesTaskSessionRead(response.url(), sessionId) && response.status() === 200,
      { timeout: 60_000 },
    );
    const unchangedRead = testPage.waitForResponse(
      (response) => matchesTaskSessionRead(response.url(), sessionId) && response.status() === 304,
      { timeout: 60_000 },
    );
    try {
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const [initial, unchanged] = await Promise.all([initialRead, unchangedRead]);

      expect(initial.headers()["etag"]).toMatch(/^"[a-f0-9]{64}"$/);
      expect(unchanged.headers()["etag"]).toMatch(/^"[a-f0-9]{64}"$/);
      expect(unchanged.request().headers()["if-none-match"]).toBe(unchanged.headers()["etag"]);
    } finally {
      await apiClient
        .stopSession({ session_id: sessionId, reason: "session refresh E2E cleanup", force: true })
        .catch(() => undefined);
    }
  });

  test("does not apply an older full response after a newer session event", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await createLongRunningSession(
      apiClient,
      seedData,
      `Stale session read ${Date.now()}`,
    );
    expect(task.session_id).toBeTruthy();
    const sessionId = task.session_id!;
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "RUNNING",
      message: "Waiting for the stale-read fixture to run",
    });

    const traffic = attachGatewayTrafficCapture(testPage);
    const session = new SessionPage(testPage);
    let heldRead: Awaited<ReturnType<typeof holdNextRunningTaskSessionRead>> | undefined;
    try {
      await test.step("subscribe and observe unchanged revalidation", async () => {
        await testPage.goto(`/t/${task.id}`);
        await session.waitForLoad();
        await expect
          .poll(
            () =>
              traffic.frames.some(
                (frame) =>
                  frame.direction === "sent" &&
                  frame.action === "session.subscribe" &&
                  frame.sessionId === sessionId,
              ),
            { timeout: 30_000, message: "The page should subscribe to the running session" },
          )
          .toBe(true);

        const stableResponse = testPage.waitForResponse(
          (response) =>
            matchesTaskSessionRead(response.url(), sessionId) && response.status() === 304,
          { timeout: 30_000 },
        );
        await stableResponse;
      });

      await test.step("hold a running full response", async () => {
        heldRead = await holdNextRunningTaskSessionRead(testPage, sessionId);
        const oldRead = await heldRead.captured;
        expect(oldRead).toMatchObject({ status: 200, state: "RUNNING" });
        expect(oldRead.etag).toMatch(/^"[a-f0-9]{64}"$/);
      });

      await test.step("apply the newer stop state before releasing the stale read", async () => {
        const stopped = await apiClient.stopSession({
          session_id: sessionId,
          reason: "session refresh stale-response E2E",
          force: true,
        });
        expect(stopped.success).toBe(true);
        await waitForSessionState(apiClient, {
          taskId: task.id,
          sessionId,
          expectedState: "CANCELLED",
          message: "Waiting for the authoritative stop event",
        });
        await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });

        heldRead!.release();
        heldRead = undefined;
        await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
        await expect(session.cancelAgentButton()).toHaveCount(0);
      });
    } finally {
      heldRead?.release();
      await testPage.unroute(`**/api/v1/task-sessions/${sessionId}`).catch(() => undefined);
      await apiClient
        .stopSession({ session_id: sessionId, reason: "session refresh E2E cleanup", force: true })
        .catch(() => undefined);
    }
  });

  test("shares environment reads and refreshes after manual refresh and reset", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    let taskId = "";
    let environmentReadCount = 0;
    let releaseEnvironmentReads!: () => void;
    let notifyFirstEnvironmentRead!: () => void;
    const environmentReadsReleased = new Promise<void>((resolve) => {
      releaseEnvironmentReads = resolve;
    });
    const firstEnvironmentRead = new Promise<void>((resolve) => {
      notifyFirstEnvironmentRead = resolve;
    });
    let resetRequested = false;

    try {
      const seeded = await seedPluginExecutorStatusTask(testPage, {
        backend,
        apiClient,
        seedData,
        state: "cleanup_pending",
        touch: false,
        onEnvironmentLiveRequest: () => {
          environmentReadCount += 1;
          notifyFirstEnvironmentRead();
          return environmentReadsReleased;
        },
      });
      taskId = seeded.taskId;
      await testPage.route(`**/api/v1/tasks/${taskId}/environment/reset`, async (route) => {
        resetRequested = true;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: '{"success":true}',
        });
      });

      await testPage.goto(`/t/${taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
      await firstEnvironmentRead;
      expect(environmentReadCount).toBe(1);
      releaseEnvironmentReads();

      await testPage.getByTestId("executor-settings-button").click();
      const disclosure = testPage.getByTestId("executor-settings-popover");
      await expect(disclosure).toBeVisible();
      await expect(disclosure.getByTestId("plugin-executor-status-message")).toBeVisible();

      const beforeRefresh = environmentReadCount;
      await disclosure.getByTestId("executor-settings-refresh").click();
      await expect.poll(() => environmentReadCount).toBeGreaterThan(beforeRefresh);

      const beforeReset = environmentReadCount;
      await disclosure.getByTestId("executor-settings-reset").click();
      const confirm = testPage.getByTestId("reset-env-confirm");
      await expect(confirm).toBeDisabled();
      await testPage.getByText("I understand any uncommitted changes will be lost.").click();
      await expect(confirm).toBeEnabled();
      await confirm.click();
      await expect.poll(() => resetRequested).toBe(true);
      await expect.poll(() => environmentReadCount).toBeGreaterThan(beforeReset);
    } finally {
      releaseEnvironmentReads();
      if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
      await uninstallFixturePlugin(apiClient);
    }
  });
});
