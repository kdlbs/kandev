import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  captureGatewayRequests,
  routeRecoveryFailureAndRetry,
  sessionLaunchRequests,
} from "../../helpers/archived-session-recovery";
import { waitForArchiveCancelledSession, waitForSessionDone } from "../../helpers/session";
import {
  prepareArchiveRecoverySession,
  seedWorktreeRecoveryFixture,
} from "../../helpers/session-resume-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile: archived session recovery", () => {
  test("keeps archived history read-only, then resumes the same session after unarchive", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(180_000);

    const fixture = await seedWorktreeRecoveryFixture(
      testPage,
      apiClient,
      seedData,
      `Mobile archived session recovery ${Date.now()}`,
    );
    const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const beforeRepository = beforeEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );

    const sessionId = await prepareArchiveRecoverySession(apiClient, fixture);
    await apiClient.archiveTask(fixture.task.id);
    await waitForArchiveCancelledSession(
      apiClient,
      fixture.task.id,
      sessionId,
      "Waiting for archive cancellation to mark the mobile recovery session",
    );

    const requests = captureGatewayRequests(testPage);
    await testPage.goto(`/t/${fixture.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const unarchiveButton = testPage.getByTestId("task-unarchive-button");
    await expect(unarchiveButton).toBeVisible();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible();
    await expect(testPage.getByTestId("failed-session-banner")).toHaveCount(0);
    expect(sessionLaunchRequests(requests, sessionId)).toHaveLength(0);
    const archivedButtonBox = await unarchiveButton.boundingBox();
    expect(archivedButtonBox).not.toBeNull();
    expect(archivedButtonBox!.height).toBeGreaterThanOrEqual(44);

    requests.length = 0;
    const unarchiveResponse = testPage.waitForResponse((response) =>
      response.url().endsWith(`/api/v1/tasks/${fixture.task.id}/unarchive`),
    );
    await unarchiveButton.tap();
    await unarchiveResponse;
    await expect(unarchiveButton).toHaveCount(0);
    await expect
      .poll(
        () => sessionLaunchRequests(requests, sessionId).map((request) => request.payload.intent),
        {
          timeout: 60_000,
          message: "Mobile unarchive did not trigger automatic same-session resume",
        },
      )
      .toEqual(["resume"]);
    await waitForSessionDone(
      apiClient,
      fixture.task.id,
      sessionId,
      "Waiting for mobile automatic same-session resume to settle",
      60_000,
    );
    expect(
      sessionLaunchRequests(requests, sessionId).map((request) => request.payload.intent),
    ).toEqual(["resume"]);

    const afterEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const afterRepository = afterEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );
    expect(afterEnvironment?.id).toBe(beforeEnvironment?.id);
    expect(afterRepository?.worktree_id).toBe(beforeRepository?.worktree_id);
    expect(afterRepository?.worktree_path).toBe(beforeRepository?.worktree_path);
    expect(afterRepository?.worktree_branch).toBe(beforeRepository?.worktree_branch);

    await session.sendMessageViaButton("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 60_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile archived session recovery");

    await testPage.screenshot({
      path: testInfo.outputPath("archived-session-recovery-mobile.png"),
      fullPage: true,
    });
    await prCapture.screenshot("archived-session-recovery-mobile", {
      caption: "Mobile archived history stays read-only until in-place recovery",
      fullPage: true,
    });
  });

  test("honors prevent-auto-start after a mobile archived task is unarchived", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });

    try {
      const fixture = await seedWorktreeRecoveryFixture(
        testPage,
        apiClient,
        seedData,
        `Mobile archived recovery preference ${Date.now()}`,
      );
      const sessionId = await prepareArchiveRecoverySession(apiClient, fixture);
      await apiClient.archiveTask(fixture.task.id);
      await waitForArchiveCancelledSession(
        apiClient,
        fixture.task.id,
        sessionId,
        "Waiting for archive cancellation to mark the mobile preference session",
      );

      const requests = captureGatewayRequests(testPage);
      await testPage.goto(`/t/${fixture.task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const unarchiveButton = testPage.getByTestId("task-unarchive-button");
      await expect(unarchiveButton).toBeVisible();
      expect(sessionLaunchRequests(requests, sessionId)).toHaveLength(0);

      requests.length = 0;
      const unarchiveResponse = testPage.waitForResponse((response) =>
        response.url().endsWith(`/api/v1/tasks/${fixture.task.id}/unarchive`),
      );
      await unarchiveButton.tap();
      await unarchiveResponse;
      await expect(unarchiveButton).toHaveCount(0);
      await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
      expect(sessionLaunchRequests(requests, sessionId)).toEqual([]);
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
    }
  });

  test("keeps recovery causes accessible and retryable on touch", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(180_000);

    const fixture = await seedWorktreeRecoveryFixture(
      testPage,
      apiClient,
      seedData,
      `Mobile archived recovery feedback ${Date.now()}`,
    );
    const sessionId = await prepareArchiveRecoverySession(apiClient, fixture);
    await apiClient.archiveTask(fixture.task.id);
    await waitForArchiveCancelledSession(
      apiClient,
      fixture.task.id,
      sessionId,
      "Waiting for archive cancellation to mark the mobile feedback session",
    );

    await routeRecoveryFailureAndRetry(testPage, {
      taskId: fixture.task.id,
      sessionId,
      resumeError: "mobile resume attempt failed in bounded e2e",
      restoreError: "mobile restore attempt failed in bounded e2e",
    });
    await testPage.goto(`/t/${fixture.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("task-unarchive-button").tap();
    await expect(testPage.getByTestId("task-unarchive-button")).toHaveCount(0);

    const banner = testPage.getByTestId("session-recovery-error");
    await expect(banner).toBeVisible({ timeout: 30_000 });
    const details = banner.getByTestId("session-recovery-details");
    await expect(details).not.toHaveAttribute("open");
    await prCapture.screenshot("archived-session-recovery-feedback-mobile-collapsed", {
      caption: "Mobile recovery keeps the detailed causes collapsed",
      fullPage: true,
    });
    await details.getByTestId("session-recovery-details-summary").tap();
    await expect(details).toHaveAttribute("open", "");
    await expect(details).toContainText("mobile resume attempt failed in bounded e2e");
    await expect(details).toContainText("mobile restore attempt failed in bounded e2e");
    await prCapture.screenshot("archived-session-recovery-feedback-mobile-expanded", {
      caption: "Mobile recovery exposes both causes with a touch-sized disclosure",
      fullPage: true,
    });
    const summaryBox = await details.getByTestId("session-recovery-details-summary").boundingBox();
    expect(summaryBox).not.toBeNull();
    expect(summaryBox!.height).toBeGreaterThanOrEqual(44);

    await session.recoveryResumeButton().tap();
    await expect(banner).toHaveCount(0, { timeout: 30_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile archived recovery feedback");

    await testPage.screenshot({
      path: testInfo.outputPath("archived-session-recovery-feedback-mobile.png"),
      fullPage: true,
    });
    await prCapture.screenshot("archived-session-recovery-feedback-mobile", {
      caption: "Mobile recovery details remain touch-accessible",
      fullPage: true,
    });
  });
});
