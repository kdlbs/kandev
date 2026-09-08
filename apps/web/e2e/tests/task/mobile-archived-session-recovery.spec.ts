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

test.describe("mobile: archived session recovery", () => {
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
      `Mobile archived session recovery ${Date.now()}`,
    );
    const sessionId = fixture.task.session_id!;
    const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const beforeRepository = beforeEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );

    await apiClient.stopSession({
      session_id: sessionId,
      reason: "mobile archive recovery e2e",
      force: true,
    });
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the mobile archived recovery session to stop",
      timeout: 30_000,
    });
    await apiClient.archiveTask(fixture.task.id);

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
      .poll(() => sessionLaunchRequests(requests, sessionId)[0]?.payload.intent ?? null, {
        timeout: 60_000,
        message: "Mobile unarchive did not trigger same-session recovery",
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
    const sessionId = fixture.task.session_id!;
    await apiClient.stopSession({
      session_id: sessionId,
      reason: "mobile archived recovery feedback e2e",
      force: true,
    });
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the mobile feedback recovery session to stop",
      timeout: 30_000,
    });
    await apiClient.archiveTask(fixture.task.id);

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
