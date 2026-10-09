import type { Response } from "@playwright/test";
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

test.describe("mobile session refresh efficiency", () => {
  test("revalidates an unchanged session without a response body", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await createLongRunningSession(
      apiClient,
      seedData,
      `Mobile conditional session read ${Date.now()}`,
    );
    expect(task.session_id).toBeTruthy();
    const sessionId = task.session_id!;
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "RUNNING",
      message: "Waiting for the mobile conditional-read fixture to run",
    });

    const fullReadEtags = new Set<string>();
    const trackFullRead = (response: Response) => {
      if (matchesTaskSessionRead(response.url(), sessionId) && response.status() === 200) {
        fullReadEtags.add(response.headers()["etag"]);
      }
    };
    testPage.on("response", trackFullRead);
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
      const validator = unchanged.request().headers()["if-none-match"];
      expect(validator).toMatch(/^"[a-f0-9]{64}"$/);
      expect(fullReadEtags.has(validator)).toBe(true);
      expect(unchanged.headers()["etag"]).toBe(validator);
    } finally {
      testPage.off("response", trackFullRead);
      await apiClient
        .stopSession({
          session_id: sessionId,
          reason: "mobile session refresh E2E cleanup",
          force: true,
        })
        .catch(() => undefined);
    }
  });

  test("keeps the newer session state when an older full response completes late", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await createLongRunningSession(
      apiClient,
      seedData,
      `Mobile stale session read ${Date.now()}`,
    );
    expect(task.session_id).toBeTruthy();
    const sessionId = task.session_id!;
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "RUNNING",
      message: "Waiting for the mobile stale-read fixture to run",
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
            { timeout: 30_000, message: "The mobile page should subscribe to the running session" },
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
          reason: "mobile session refresh stale-response E2E",
          force: true,
        });
        expect(stopped.success).toBe(true);
        await waitForSessionState(apiClient, {
          taskId: task.id,
          sessionId,
          expectedState: "CANCELLED",
          message: "Waiting for the authoritative mobile stop event",
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
        .stopSession({
          session_id: sessionId,
          reason: "mobile session refresh E2E cleanup",
          force: true,
        })
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
        touch: true,
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

      const trigger = testPage.getByTestId("executor-settings-button");
      await expectTouchTarget(trigger, "Environment drawer trigger");
      await trigger.tap();
      const drawer = testPage.getByTestId("executor-settings-drawer");
      await expect(drawer).toBeVisible();
      await expect(drawer.getByTestId("plugin-executor-status-message")).toBeVisible();

      const beforeRefresh = environmentReadCount;
      const refresh = drawer.getByTestId("executor-settings-refresh");
      await expectTouchTarget(refresh, "Environment refresh action");
      await refresh.tap();
      await expect.poll(() => environmentReadCount).toBeGreaterThan(beforeRefresh);

      const beforeReset = environmentReadCount;
      const reset = drawer.getByTestId("executor-settings-reset");
      await expectTouchTarget(reset, "Environment reset action");
      await reset.tap();
      const confirm = testPage.getByTestId("reset-env-confirm");
      await expect(confirm).toBeDisabled();
      await testPage.getByText("I understand any uncommitted changes will be lost.").tap();
      await expect(confirm).toBeEnabled();
      await confirm.tap();
      await expect.poll(() => resetRequested).toBe(true);
      await expect.poll(() => environmentReadCount).toBeGreaterThan(beforeReset);
    } finally {
      releaseEnvironmentReads();
      if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
      await uninstallFixturePlugin(apiClient);
    }
  });
});

async function expectTouchTarget(locator: import("@playwright/test").Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} must have geometry`).not.toBeNull();
  const height = Math.round(box!.height * 100) / 100;
  const width = Math.round(box!.width * 100) / 100;
  expect(height, `${label} must be at least 44px tall`).toBeGreaterThanOrEqual(44);
  expect(width, `${label} must be at least 44px wide`).toBeGreaterThanOrEqual(44);
}
