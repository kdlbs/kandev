// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionState } from "../../helpers/session";
import {
  removeRecoveryBranch,
  seedWorktreeRecoveryFixture,
} from "../../helpers/session-resume-recovery";

test.describe("mobile: worktree branch resume recovery", () => {
  test.describe.configure({ retries: 1 });

  test("keeps branch recovery touch-safe and reload-stable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }, testInfo) => {
    test.setTimeout(180_000);

    const fixture = await seedWorktreeRecoveryFixture(
      testPage,
      apiClient,
      seedData,
      `Mobile worktree branch recovery ${Date.now()}`,
    );
    const sessionId = fixture.task.session_id!;
    const originalBranch = fixture.repository.worktree_branch!;
    const originalPath = fixture.repository.worktree_path!;
    const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const beforeRepository = beforeEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );

    const stopResponse = await apiClient.stopSession({
      session_id: sessionId,
      reason: "mobile e2e branch recovery",
      force: true,
    });
    expect(stopResponse.success).toBe(true);
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the mobile worktree recovery session to stop",
      timeout: 30_000,
    });
    await expect(fixture.session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });

    removeRecoveryBranch(seedData.repositoryPath, backend.tmpDir, fixture.repository);

    await fixture.session.recoveryResumeButton().tap();
    await expect(fixture.session.recoveryError()).toBeVisible({ timeout: 30_000 });
    await expect(fixture.session.recoveryError()).toContainText("no longer available");
    await expect(fixture.session.recoveryNewBranchButton()).toBeVisible();
    await expect(fixture.session.recoveryRestoreWorkspaceButton()).toBeVisible();

    for (const button of [
      fixture.session.recoveryNewBranchButton(),
      fixture.session.recoveryRestoreWorkspaceButton(),
    ]) {
      await expect(button).toBeInViewport();
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await fixture.session.recoveryRestoreWorkspaceButton().focus();
    await expect(fixture.session.recoveryRestoreWorkspaceButton()).toBeFocused();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile lost branch recovery error");

    // The normal action remains retryable and cannot alter the persisted branch
    // until the user taps the explicit replacement action.
    await fixture.session.recoveryResumeButton().tap();
    await expect(fixture.session.recoveryError()).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(
        async () =>
          (await apiClient.getTaskEnvironment(fixture.task.id))?.repos?.find(
            (repository) => repository.repository_id === seedData.repositoryId,
          )?.worktree_branch ?? null,
        { timeout: 15_000, message: "Mobile Resume changed the branch before explicit consent" },
      )
      .toBe(originalBranch);

    await fixture.session.recoveryNewBranchButton().tap();
    await expect(fixture.session.branchRecreatedWarning()).toBeVisible({ timeout: 60_000 });
    await fixture.session.waitForChatIdle({ timeout: 30_000 });

    let afterEnvironment: Awaited<ReturnType<typeof apiClient.getTaskEnvironment>> = null;
    let newBranch = "";
    await expect
      .poll(
        async () => {
          afterEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
          newBranch =
            afterEnvironment?.repos?.find(
              (repository) => repository.repository_id === seedData.repositoryId,
            )?.worktree_branch ?? "";
          return newBranch && newBranch !== originalBranch ? newBranch : null;
        },
        { timeout: 30_000, message: "Waiting for the mobile replacement worktree branch" },
      )
      .toBeTruthy();

    const afterRepository = afterEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );
    expect(afterEnvironment?.id).toBe(beforeEnvironment?.id);
    expect(afterRepository?.worktree_id).toBe(beforeRepository?.worktree_id);
    expect(afterRepository?.worktree_path).not.toBe(originalPath);
    expect(newBranch).not.toBe("");
    await expect(fixture.session.branchRecreatedWarning()).toContainText(newBranch);
    await expect(fixture.session.recoveryError()).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile replacement branch warning");

    // Verify the recovered provider accepts a follow-up turn on the same
    // session after the replacement worktree is ready.
    await fixture.session.sendMessageViaButton("/e2e:simple-message");
    await fixture.session.expectChatResponseVisible("simple mock response", 1, {
      timeout: 60_000,
    });

    await testPage.screenshot({
      path: testInfo.outputPath("session-resume-recovery-mobile.png"),
      fullPage: true,
    });
    await prCapture.screenshot("session-resume-recovery-mobile", {
      caption: "Mobile branch recovery keeps explicit actions touch-safe",
      fullPage: true,
    });

    await testPage.reload();
    await fixture.session.waitForLoad();
    await expect(fixture.session.branchRecreatedWarning()).toHaveCount(1, { timeout: 30_000 });
    await expect(fixture.session.branchRecreatedWarning()).toContainText(newBranch);
    await expect(fixture.session.recoveryError()).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "reloaded mobile branch warning");
  });
});
