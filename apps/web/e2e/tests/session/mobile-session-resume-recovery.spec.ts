// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import type { SeedData } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
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
  cleanupManagedCloneRelocationFixture,
  countSimpleMockResponses,
  readPrivateManagedCloneRecoveryArtifacts,
  readManagedCloneRecoveryConsumers,
  removeRecoveryBranch,
  seedManagedCloneRelocationFixture,
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

  test("cancel fences the delayed startup before a touch retry", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);

    const fixture = await seedDelayedResumeFixture(testPage, apiClient, seedData, backend, {
      title: "Mobile session cancel and retry recovery",
    });

    try {
      await expect(fixture.session.cancelAgentButton()).toBeVisible({ timeout: 15_000 });
      await fixture.session.cancelAgentButton().tap();
      await waitForSessionState(apiClient, {
        taskId: fixture.task.id,
        sessionId: fixture.identity.sessionId,
        expectedState: "WAITING_FOR_INPUT",
        message: "Waiting for mobile delayed resume cancellation",
        timeout: 30_000,
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
    const branchButton = fixture.session.recoveryNewBranchButton();
    await expect(branchButton).toBeInViewport();
    expect((await branchButton.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    const restoreButton = testPage.getByTestId("recovery-restore-workspace-button");
    await expect(restoreButton).toBeInViewport();
    expect((await restoreButton.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await restoreButton.focus();
    await expect(restoreButton).toBeFocused();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile lost branch recovery error");

    // A normal retry remains distinct from the explicit replacement action.
    await testPage.getByTestId("recovery-resume-button").tap();
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

test.describe("mobile: dirty managed clone relocation", () => {
  test.describe.configure({ retries: 0 });

  test.afterEach(async ({ apiClient, seedData }) => {
    await cleanupManagedCloneRelocationFixture(apiClient, seedData);
  });

  test("moves the dirty worktree through touch confirmation and resumes the session", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(180_000);

    const fixture = await seedManagedCloneRelocationFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      `Mobile managed clone relocation ${Date.now()}`,
    );
    const sessionId = fixture.task.session_id!;
    const originalPath = fixture.repository.worktree_path!;
    const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
    const beforeRepository = beforeEnvironment?.repos?.find(
      (repository) => repository.repository_id === seedData.repositoryId,
    );

    const stopResponse = await apiClient.stopSession({
      session_id: sessionId,
      reason: "mobile e2e managed clone relocation",
      force: true,
    });
    expect(stopResponse.success).toBe(true);
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the mobile managed clone relocation session to stop",
      timeout: 30_000,
    });
    await fixture.session.recoveryResumeButton().click();

    const relocate = testPage.getByTestId("managed-clone-relocate-button");
    await expect(relocate).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(
        async () => {
          const status = await apiClient.wsRequest<{
            is_agent_running: boolean;
          }>("task.session.status", { task_id: fixture.task.id, session_id: sessionId });
          return status.is_agent_running;
        },
        {
          timeout: 30_000,
          intervals: [250, 500, 1_000],
          message: "Waiting for the mobile stopped session runtime to exit",
        },
      )
      .toBe(false);
    await expect
      .poll(() => readManagedCloneRecoveryConsumers(backend.tmpDir, fixture.environment.id), {
        timeout: 30_000,
        intervals: [250, 500, 1_000],
        message: "Waiting for the mobile stopped resumable runtime to settle durably",
      })
      .toEqual([{ sessionId, state: "CANCELLED", runtimeStatus: "stopped" }]);
    await expect(relocate).toBeInViewport();
    expect((await relocate.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(0);
    await expect(testPage.getByTestId("recovery-fresh-button")).toHaveCount(0);
    await expect(testPage.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);
    if (prCapture.capturing) {
      await waitForFiniteAnimations(testPage.locator("body"));
      await prCapture.screenshot("managed-clone-relocation-card-phone", {
        caption: "The phone recovery card keeps the repair action reachable.",
      });
    }
    await relocate.tap();

    const confirmation = testPage.getByTestId("managed-clone-relocation-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toContainText("snapshot");
    await expect(confirmation).toContainText("staging choices");
    const confirm = testPage.getByTestId("managed-clone-relocation-confirm");
    await confirm.scrollIntoViewIfNeeded();
    if (prCapture.capturing) {
      await waitForFiniteAnimations(confirmation);
      await prCapture.screenshot("managed-clone-relocation-confirm-phone", {
        caption: "The phone drawer explains the preserved files and staging limit.",
      });
    }
    await expect(confirm).toBeInViewport();
    expect((await confirm.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await confirm.tap();

    await fixture.session.waitForChatIdle({ timeout: 60_000 });
    let afterEnvironment: Awaited<ReturnType<typeof apiClient.getTaskEnvironment>> = null;
    let afterRepository = beforeRepository;
    await expect
      .poll(
        async () => {
          afterEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
          afterRepository = afterEnvironment?.repos?.find(
            (repository) => repository.repository_id === seedData.repositoryId,
          );
          return afterRepository?.worktree_path ?? null;
        },
        { timeout: 30_000, message: "Waiting for the mobile relocated worktree identity" },
      )
      .not.toBe(originalPath);

    const relocatedPath = afterRepository?.worktree_path;
    expect(afterEnvironment?.id).toBe(beforeEnvironment?.id);
    expect(afterRepository?.worktree_id).not.toBe(beforeRepository?.worktree_id);
    expect(afterRepository?.worktree_branch).toBe(fixture.originalBranch);
    const recoveryArtifacts = readPrivateManagedCloneRecoveryArtifacts(
      backend.tmpDir,
      fixture.environment.id,
      seedData.repositoryId,
    );
    expect(recoveryArtifacts.relocationPath).toContain(`${path.sep}.kandev-recovery${path.sep}`);
    expect(recoveryArtifacts.recoveryPath).toContain(`${path.sep}.kandev-recovery${path.sep}`);
    expect(fs.existsSync(`${originalPath}.kandev-clone-relocation.json`)).toBe(false);
    const retainedOriginal = recoveryArtifacts.original;
    expect(retainedOriginal).not.toBe(originalPath);
    expect(retainedOriginal).toContain(`${path.sep}.kandev-recovery${path.sep}`);
    expect(recoveryArtifacts.snapshot).toContain(`${path.sep}.kandev-recovery${path.sep}`);
    expect(fs.existsSync(originalPath)).toBe(false);
    expect(fs.readFileSync(path.join(retainedOriginal, fixture.dirtyFileName), "utf8")).toBe(
      fixture.dirtyFileContent,
    );
    expect(
      fs.readFileSync(path.join(recoveryArtifacts.snapshot, fixture.dirtyFileName), "utf8"),
    ).toBe(fixture.dirtyFileContent);
    expect(fs.readFileSync(path.join(relocatedPath!, fixture.dirtyFileName), "utf8")).toBe(
      fixture.dirtyFileContent,
    );
    const relocatedGit = new GitHelper(relocatedPath!, makeGitEnv(backend.tmpDir));
    expect(relocatedGit.getCurrentSha()).toBe(fixture.originalHead);
    expect(relocatedGit.exec("git rev-parse --git-common-dir").trim()).toBe(
      path.join(fixture.destinationClonePath, ".git"),
    );
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for the mobile relocated session to resume",
      timeout: 30_000,
    });
    await expect(fixture.session.agentStatus()).toHaveCount(0, { timeout: 30_000 });
    await expect(
      fixture.session.activeChat().locator('[data-placeholder="Preparing workspace..."]'),
    ).toHaveCount(0);
    const priorMockResponses = await countSimpleMockResponses(apiClient, sessionId);
    await fixture.session.sendMessageViaButton("/e2e:simple-message");
    await expect
      .poll(() => countSimpleMockResponses(apiClient, sessionId), {
        timeout: 60_000,
        message: "Waiting for the relocated mobile session's follow-up response",
      })
      .toBeGreaterThan(priorMockResponses);
    await expect(
      fixture.session.activeChat().getByText("simple mock response", { exact: false }).last(),
    ).toBeVisible({ timeout: 30_000 });
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    const expectedFilesPath = relocatedPath!.replace(/^\/(?:Users|home)\/[^/]+\//, "~/");
    await expect(fixture.session.files.getByTestId("file-browser-workspace-path")).toHaveText(
      expectedFilesPath,
      { timeout: 30_000 },
    );
    await assertNoDocumentHorizontalOverflow(testPage, "mobile managed clone relocation");
  });
});
