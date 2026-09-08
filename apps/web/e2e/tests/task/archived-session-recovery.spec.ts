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

test.describe("archived session recovery", () => {
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
      `Archived session recovery ${Date.now()}`,
    );
    const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const beforeRepository = beforeEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );
    expect(beforeEnvironment?.id).toBe(fixture.environment.id);

    const sessionId = await prepareArchiveRecoverySession(apiClient, fixture);
    await apiClient.archiveTask(fixture.task.id);
    await waitForArchiveCancelledSession(
      apiClient,
      fixture.task.id,
      sessionId,
      "Waiting for archive cancellation to mark the recovery session",
    );

    const requests = captureGatewayRequests(testPage);
    await testPage.goto(`/t/${fixture.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible();
    await expect(testPage.getByTestId("failed-session-banner")).toHaveCount(0);
    await expect(testPage.getByTestId("session-recovery-error")).toHaveCount(0);
    expect(sessionLaunchRequests(requests, sessionId)).toHaveLength(0);

    requests.length = 0;
    const unarchiveResponse = testPage.waitForResponse((response) =>
      response.url().endsWith(`/api/v1/tasks/${fixture.task.id}/unarchive`),
    );
    await testPage.getByTestId("task-unarchive-button").click();
    await unarchiveResponse;
    await expect(testPage.getByTestId("task-unarchive-button")).toHaveCount(0);
    await expect
      .poll(
        () => sessionLaunchRequests(requests, sessionId).map((request) => request.payload.intent),
        {
          timeout: 60_000,
          message: "Unarchive did not trigger automatic same-session resume",
        },
      )
      .toEqual(["resume"]);
    await waitForSessionDone(
      apiClient,
      fixture.task.id,
      sessionId,
      "Waiting for automatic same-session resume to settle",
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

    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 60_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "archived session recovery");

    await testPage.screenshot({
      path: testInfo.outputPath("archived-session-recovery-desktop.png"),
      fullPage: true,
    });
    await prCapture.screenshot("archived-session-recovery-desktop", {
      caption: "The archived task stays read-only until in-place unarchive recovery",
      fullPage: true,
    });
  });

  test("honors prevent-auto-start after an archived task is unarchived", async ({
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
        `Archived recovery preference ${Date.now()}`,
      );
      const sessionId = await prepareArchiveRecoverySession(apiClient, fixture);
      await apiClient.archiveTask(fixture.task.id);
      await waitForArchiveCancelledSession(
        apiClient,
        fixture.task.id,
        sessionId,
        "Waiting for archive cancellation to mark the preference session",
      );

      const requests = captureGatewayRequests(testPage);
      await testPage.goto(`/t/${fixture.task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();
      expect(sessionLaunchRequests(requests, sessionId)).toHaveLength(0);

      requests.length = 0;
      const unarchiveResponse = testPage.waitForResponse((response) =>
        response.url().endsWith(`/api/v1/tasks/${fixture.task.id}/unarchive`),
      );
      await testPage.getByTestId("task-unarchive-button").click();
      await unarchiveResponse;
      await expect(testPage.getByTestId("task-unarchive-button")).toHaveCount(0);
      await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
      expect(
        sessionLaunchRequests(requests, sessionId).map((request) => request.payload.intent),
      ).toEqual([]);
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
    }
  });

  test("keeps both automatic recovery causes behind an accessible disclosure", async ({
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
      `Archived recovery feedback ${Date.now()}`,
    );
    const sessionId = await prepareArchiveRecoverySession(apiClient, fixture);
    await apiClient.archiveTask(fixture.task.id);
    await waitForArchiveCancelledSession(
      apiClient,
      fixture.task.id,
      sessionId,
      "Waiting for archive cancellation to mark the feedback session",
    );

    await routeRecoveryFailureAndRetry(testPage, {
      taskId: fixture.task.id,
      sessionId,
      resumeError: "resume attempt failed in bounded e2e",
      restoreError: "restore attempt failed in bounded e2e",
    });
    await testPage.goto(`/t/${fixture.task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("task-unarchive-button").click();
    await expect(testPage.getByTestId("task-unarchive-button")).toHaveCount(0);

    const banner = testPage.getByTestId("session-recovery-error");
    await expect(banner).toBeVisible({ timeout: 30_000 });
    await expect(banner).toContainText("Session recovery failed");
    const details = banner.getByTestId("session-recovery-details");
    await expect(details).not.toHaveAttribute("open");
    await prCapture.screenshot("archived-session-recovery-feedback-desktop-collapsed", {
      caption: "Automatic recovery keeps the detailed causes collapsed",
      fullPage: true,
    });
    await details.getByTestId("session-recovery-details-summary").click();
    await expect(details).toHaveAttribute("open", "");
    await expect(details).toContainText("Resume attempt");
    await expect(details).toContainText("resume attempt failed in bounded e2e");
    await expect(details).toContainText("Workspace restore attempt");
    await expect(details).toContainText("restore attempt failed in bounded e2e");
    await prCapture.screenshot("archived-session-recovery-feedback-desktop-expanded", {
      caption: "Automatic recovery exposes both labeled causes",
      fullPage: true,
    });

    await expect(session.recoveryResumeButton()).toBeEnabled();
    await session.recoveryResumeButton().click();
    await expect(banner).toHaveCount(0, { timeout: 30_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "archived recovery feedback");

    await testPage.screenshot({
      path: testInfo.outputPath("archived-session-recovery-feedback-desktop.png"),
      fullPage: true,
    });
    await prCapture.screenshot("archived-session-recovery-feedback-desktop", {
      caption: "Automatic recovery shows a compact summary with expandable causes",
      fullPage: true,
    });
  });
});
