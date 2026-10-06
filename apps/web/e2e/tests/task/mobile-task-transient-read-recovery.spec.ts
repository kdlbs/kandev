import type { Route } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Mobile task transient read recovery", () => {
  test("shows and recovers the initial temporary task-read failure", async ({
    testPage,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const taskId = "mobile-task-navigation-initial-read-recovery";
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
    const retry = testPage.getByTestId("task-read-retry");
    const overview = testPage.getByTestId("task-unavailable-overview-link");
    await expect(overview).toBeVisible();
    const retryBox = await retry.boundingBox();
    const overviewBox = await overview.boundingBox();
    const viewport = testPage.viewportSize();
    expect(failedReads).toBe(3);
    expect(retryBox).not.toBeNull();
    expect(overviewBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    if (retryBox && viewport) {
      expect(retryBox.height).toBeGreaterThanOrEqual(44);
      expect(retryBox.x).toBeGreaterThanOrEqual(0);
      expect(retryBox.x + retryBox.width).toBeLessThanOrEqual(viewport.width + 1);
    }
    if (overviewBox && viewport) {
      expect(overviewBox.height).toBeGreaterThanOrEqual(44);
      expect(overviewBox.x).toBeGreaterThanOrEqual(0);
      expect(overviewBox.x + overviewBox.width).toBeLessThanOrEqual(viewport.width + 1);
    }
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await testInfo.attach("phone-task-initial-temporary-read-failure", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });

    const recoveredTaskRead = waitForHttp(testPage, "GET", taskPath);
    await retry.tap();
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
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });

  test("offers reachable retry and overview actions after temporary route failures", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile task navigation retry fixture",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("Task fixture has no session");

    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();

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
      const recoveryNotice = testPage.getByTestId("task-read-recovery-notice");
      await expect(recoveryNotice).toBeVisible();
      await expect(recoveryNotice).toContainText("Task details could not be refreshed");
      await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
      const retry = testPage.getByTestId("task-read-retry");
      const retryBox = await retry.boundingBox();
      const viewport = testPage.viewportSize();
      expect(failedReads).toBe(3);
      expect(retryBox).not.toBeNull();
      expect(viewport).not.toBeNull();
      if (retryBox && viewport) {
        expect(retryBox.height).toBeGreaterThanOrEqual(44);
        expect(retryBox.x).toBeGreaterThanOrEqual(0);
        expect(retryBox.x + retryBox.width).toBeLessThanOrEqual(viewport.width + 1);
      }
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      await testInfo.attach("phone-task-temporary-read-refresh-failure", {
        body: await testPage.screenshot(),
        contentType: "image/png",
      });

      const recoveredTaskRead = waitForHttp(testPage, "GET", taskPath);
      await retry.tap();
      const recoveredTaskResponse = await recoveredTaskRead;
      expect(recoveredTaskResponse.ok()).toBe(true);
      await expect(recoveryNotice).toHaveCount(0);
      await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      await testPage.unroute(taskPath, temporaryFailure);
    }
  });
});
