import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionState } from "../../helpers/session";
import {
  removeRecoveryBranch,
  seedWorktreeRecoveryFixture,
} from "../../helpers/session-resume-recovery";

test.describe("worktree branch resume recovery", () => {
  test.describe.configure({ retries: 1 });

  test("keeps normal resume unchanged and explicitly replaces a lost branch", async ({
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
      `Worktree branch recovery ${Date.now()}`,
    );
    const sessionId = fixture.task.session_id!;
    const originalBranch = fixture.repository.worktree_branch!;
    const originalPath = fixture.repository.worktree_path!;
    const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const beforeRepository = beforeEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );
    expect(beforeEnvironment?.id).toBe(fixture.environment.id);
    expect(beforeRepository?.worktree_id).toBe(fixture.repository.worktree_id);

    const stopResponse = await apiClient.stopSession({
      session_id: sessionId,
      reason: "e2e branch recovery",
      force: true,
    });
    expect(stopResponse.success).toBe(true);
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the worktree recovery session to stop",
      timeout: 30_000,
    });
    await expect(fixture.session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });

    removeRecoveryBranch(seedData.repositoryPath, backend.tmpDir, fixture.repository);

    // A normal Resume must report the lost branch and leave the environment
    // untouched. The replacement is only available from the typed failure.
    await fixture.session.recoveryResumeButton().click();
    await expect(fixture.session.recoveryError()).toBeVisible({ timeout: 30_000 });
    await expect(fixture.session.recoveryError()).toContainText("no longer available");
    await expect(fixture.session.recoveryNewBranchButton()).toBeVisible();
    await expect(fixture.session.recoveryRestoreWorkspaceButton()).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "lost branch recovery error");

    // A second ordinary attempt remains retryable and must not silently switch
    // the branch or consume the explicit replacement decision.
    await fixture.session.recoveryResumeButton().click();
    await expect(fixture.session.recoveryError()).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(
        async () =>
          (await apiClient.getTaskEnvironment(fixture.task.id))?.repos?.find(
            (repository) => repository.repository_id === seedData.repositoryId,
          )?.worktree_branch ?? null,
        { timeout: 15_000, message: "Normal resume changed the branch before explicit consent" },
      )
      .toBe(originalBranch);

    // The typed action preserves the existing session and environment identity
    // while creating a new branch and worktree path.
    await fixture.session.recoveryNewBranchButton().click();
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
        { timeout: 30_000, message: "Waiting for the replacement worktree branch" },
      )
      .toBeTruthy();

    const afterRepository = afterEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );
    expect(afterEnvironment?.id).toBe(beforeEnvironment?.id);
    expect(afterRepository?.worktree_id).toBe(beforeRepository?.worktree_id);
    expect(afterRepository?.worktree_path).not.toBe(originalPath);
    expect(newBranch).not.toBe("");
    expect(newBranch).not.toBe(originalBranch);
    await expect(fixture.session.branchRecreatedWarning()).toContainText(originalBranch);
    await expect(fixture.session.branchRecreatedWarning()).toContainText(newBranch);
    await expect(fixture.session.branchRecreatedWarning()).toContainText("main");
    await expect(fixture.session.recoveryError()).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "replacement branch warning");

    // A resumed session must accept a real follow-up turn after the branch
    // replacement. The original response remains visible, and the second
    // response proves the provider session continued instead of stopping at
    // workspace preparation.
    await fixture.session.sendMessage("/e2e:simple-message");
    await fixture.session.expectChatResponseVisible("simple mock response", 1, {
      timeout: 60_000,
    });

    await testPage.screenshot({
      path: testInfo.outputPath("session-resume-recovery-desktop.png"),
      fullPage: true,
    });
    await prCapture.screenshot("session-resume-recovery-desktop", {
      caption: "Explicit worktree branch recovery keeps the session and warns about lost code",
      fullPage: true,
    });

    // The warning is a persisted status message, not a transient component
    // state. Reloading must preserve exactly one honest warning and no blocker.
    await testPage.reload();
    await fixture.session.waitForLoad();
    await expect(fixture.session.branchRecreatedWarning()).toHaveCount(1, { timeout: 30_000 });
    await expect(fixture.session.branchRecreatedWarning()).toContainText(newBranch);
    await expect(fixture.session.recoveryError()).toHaveCount(0);
    await expect(fixture.session.activeChat()).toContainText("simple mock response");
    await assertNoDocumentHorizontalOverflow(testPage, "reloaded replacement branch warning");
  });
});
