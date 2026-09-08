import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  captureGatewayRequests,
  routeRecoveryFailureAndRetry,
  sessionLaunchRequests,
} from "../../helpers/archived-session-recovery";
import { waitForSessionState } from "../../helpers/session";
import { seedWorktreeRecoveryFixture } from "../../helpers/session-resume-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("archived session recovery", () => {
  test.describe.configure({ retries: 1 });

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
    const sessionId = fixture.task.session_id!;
    const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const beforeRepository = beforeEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );
    expect(beforeEnvironment?.id).toBe(fixture.environment.id);

    const stopResponse = await apiClient.stopSession({
      session_id: sessionId,
      reason: "archive recovery e2e",
      force: true,
    });
    expect(stopResponse.success).toBe(true);
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the archived recovery session to stop",
      timeout: 30_000,
    });
    await apiClient.archiveTask(fixture.task.id);

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
      .poll(() => sessionLaunchRequests(requests, sessionId)[0]?.payload.intent ?? null, {
        timeout: 60_000,
        message: "Unarchive did not trigger same-session recovery",
      })
      .toMatch(/^(resume|restore_workspace)$/);
    await session.waitForChatIdle({ timeout: 60_000 });

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
      const sessionId = fixture.task.session_id!;
      await apiClient.stopSession({
        session_id: sessionId,
        reason: "archive recovery preference e2e",
        force: true,
      });
      await waitForSessionState(apiClient, {
        taskId: fixture.task.id,
        sessionId,
        expectedState: "CANCELLED",
        message: "Waiting for the preference recovery session to stop",
        timeout: 30_000,
      });
      await apiClient.archiveTask(fixture.task.id);

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
      ).not.toContain("resume");
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
    const sessionId = fixture.task.session_id!;
    await apiClient.stopSession({
      session_id: sessionId,
      reason: "archived recovery feedback e2e",
      force: true,
    });
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the feedback recovery session to stop",
      timeout: 30_000,
    });
    await apiClient.archiveTask(fixture.task.id);

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
