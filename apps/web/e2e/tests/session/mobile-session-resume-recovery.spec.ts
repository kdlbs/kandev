// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import {
  cleanupDelayedResumeFixture,
  countResumeBootMessages,
  createFailOnResumeProfile,
  readSessionMessageIdsContaining,
  readSessionRuntimeIdentity,
  seedDelayedResumeFixture,
  waitForNewSessionMessage,
  waitForSessionReady,
} from "../../helpers/session-resume-prompt-queue";
import {
  removeRecoveryBranch,
  seedWorktreeRecoveryFixture,
} from "../../helpers/session-resume-recovery";

async function seedSessionWithProfile(
  testPage: Parameters<typeof seedDelayedResumeFixture>[0],
  apiClient: Parameters<typeof seedDelayedResumeFixture>[1],
  seedData: SeedData,
  title: string,
  agentProfileId: string,
): Promise<{
  task: Awaited<ReturnType<typeof apiClient.createTaskWithAgent>>;
  session: SessionPage;
}> {
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return { task, session };
}

const CRASH_RECOVERY_TIMEOUT = 170_000;

test.describe("mobile: delayed resume cancellation", () => {
  test.describe.configure({ retries: 1 });

  test.describe("unaccepted startup cancellation", () => {
    test.describe.configure({ retries: 0 });

    test("cancel fences the delayed startup before a touch retry", async ({
      testPage,
      apiClient,
      seedData,
      backend,
    }) => {
      test.setTimeout(180_000);

      const fixture = await seedDelayedResumeFixture(
        testPage,
        apiClient,
        seedData,
        backend,
        "Mobile session cancel and retry recovery",
      );

      try {
        await expect(fixture.session.cancelAgentButton()).toBeVisible({ timeout: 15_000 });
        await fixture.session.cancelAgentButton().tap();
        await waitForSessionState(apiClient, {
          taskId: fixture.task.id,
          sessionId: fixture.identity.sessionId,
          expectedState: "WAITING_FOR_INPUT",
          message: "Waiting for mobile delayed resume cancellation",
          timeout: 60_000,
        });
        // Retry the same saved conversation through the touch composer. The old
        // delayed callback must not publish a second response or consume this
        // new attempt.
        await waitForSessionReady(
          testPage,
          apiClient,
          fixture.task.id,
          fixture.identity.sessionId,
          90_000,
        );
        await expect(fixture.session.activeChat().getByTestId("chat-input-editor")).toHaveAttribute(
          "contenteditable",
          "true",
          { timeout: 30_000 },
        );

        const priorResponses = await readSessionMessageIdsContaining(
          apiClient,
          fixture.identity.sessionId,
          "simple mock response",
        );
        await fixture.session.sendMessageViaButton("/e2e:simple-message");
        await waitForNewSessionMessage(
          apiClient,
          fixture.identity.sessionId,
          priorResponses,
          "simple mock response",
          90_000,
        );
        await fixture.session.expectChatResponseVisible("simple mock response", 1);
        const responses = fixture.session
          .activeChat()
          .locator("[data-agent-message-body][data-message-id]")
          .filter({ hasText: "simple mock response" });
        await expect(responses).toHaveCount(2);
        await assertNoDocumentHorizontalOverflow(testPage, "mobile delayed cancel and retry");
      } finally {
        await cleanupDelayedResumeFixture(apiClient, fixture);
      }
    });
  });

  test("pausing an accepted lazy resume preserves the runtime for later turns", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(150_000);
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Mobile accepted lazy resume pause recovery",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("mobile accepted lazy resume task has no session_id");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.waitForChatIdle({ timeout: 30_000 });

      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();
      await expect(testPage.getByTestId("composer-agent-start-hint")).toBeVisible({
        timeout: 60_000,
      });

      const resumeBootsBeforeMessage = await countResumeBootMessages(apiClient, task.session_id);

      // Provider output proves that the resumed prompt crossed acceptance
      // before the touch cancellation is sent.
      await session.sendMessageViaButton("/slow 8s");
      await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      const initialRuntimeIdentity = await readSessionRuntimeIdentity(
        apiClient,
        task.id,
        task.session_id,
      );
      await session.cancelAgentButton().tap();
      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId: task.session_id,
        expectedState: "WAITING_FOR_INPUT",
        message: "Waiting for mobile accepted lazy resume cancellation",
        timeout: 30_000,
      });
      await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

      expect(await readSessionRuntimeIdentity(apiClient, task.id, task.session_id)).toEqual(
        initialRuntimeIdentity,
      );
      const resumeBootsAfterFirstPause = await countResumeBootMessages(apiClient, task.session_id);
      expect(resumeBootsAfterFirstPause).toBe(resumeBootsBeforeMessage + 1);

      const slowResponseMessageIdsBeforeSecond = await readSessionMessageIdsContaining(
        apiClient,
        task.session_id,
        "Running slow response",
      );
      expect(slowResponseMessageIdsBeforeSecond.size).toBeGreaterThan(0);
      await session.sendMessageViaButton("/slow 8s");
      await waitForNewSessionMessage(
        apiClient,
        task.session_id,
        slowResponseMessageIdsBeforeSecond,
        "Running slow response",
      );
      await session.cancelAgentButton().tap();
      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId: task.session_id,
        expectedState: "WAITING_FOR_INPUT",
        message: "Waiting for the later mobile accepted pause",
        timeout: 30_000,
      });

      expect(await readSessionRuntimeIdentity(apiClient, task.id, task.session_id)).toEqual(
        initialRuntimeIdentity,
      );
      expect(await countResumeBootMessages(apiClient, task.session_id)).toBe(
        resumeBootsAfterFirstPause,
      );

      await session.sendMessageViaButton("/e2e:simple-message");
      await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
      expect(await readSessionRuntimeIdentity(apiClient, task.id, task.session_id)).toEqual(
        initialRuntimeIdentity,
      );
      expect(await countResumeBootMessages(apiClient, task.session_id)).toBe(
        resumeBootsAfterFirstPause,
      );
      await assertNoDocumentHorizontalOverflow(testPage, "mobile accepted lazy resume pause");
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
    }
  });
});

test.describe("mobile: failed resume recovery", () => {
  test.describe.configure({ retries: 1 });

  test("failed saved-session load retains a usable recovery entry", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(220_000);
    const profile = await createFailOnResumeProfile(
      apiClient,
      `Mobile ACP Fail On Resume ${Date.now()}`,
    );

    try {
      const fixture = await seedSessionWithProfile(
        testPage,
        apiClient,
        seedData,
        "Mobile failed saved-session load recovery",
        profile.id,
      );
      await fixture.session.sendMessageViaButton("/crash");
      await expect(fixture.session.recoveryResumeButton()).toBeVisible({
        timeout: CRASH_RECOVERY_TIMEOUT,
      });

      await fixture.session.recoveryResumeButton().tap();
      await expect(fixture.session.recoveryResumeButton()).toBeVisible();
      // The resume endpoint waits through provider startup before it returns;
      // keep the persisted recovery entry usable throughout that failed-load
      // path and ensure the old permanently pending label never replaces it.
      await expect(testPage.getByText(/Resume session requested/i)).toHaveCount(0, {
        timeout: 15_000,
      });
      await expect(fixture.session.recoveryResumeButton()).toBeVisible({ timeout: 90_000 });
      await expect(
        fixture.session.activeChat().getByTestId("session-bootstrap-recovery-card"),
      ).toHaveCount(0);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile failed resume recovery");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true).catch(() => undefined);
    }
  });
});

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
    await expect(fixture.session.agentStatus()).toHaveCount(0);
    await expect(
      fixture.session.activeChat().locator('[data-placeholder="Preparing workspace..."]'),
    ).toHaveCount(0);

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
