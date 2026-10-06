import { test, expect } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { waitForSessionState } from "../../helpers/session";
import {
  assertSuccessfulRelocationResponse,
  captureSessionRecoveryMessages,
  capturedSessionRecoveryResponse,
  capturedSessionRecoveryResponseType,
  cleanupManagedCloneRelocationFixture,
  countSimpleMockResponses,
  expectMinimumElementHeight,
  readManagedCloneRecoveryConsumers,
  readPrivateManagedCloneRecoveryArtifacts,
  removeRecoveryBranch,
  seedManagedCloneRelocationFixture,
  seedWorktreeRecoveryFixture,
  taskEnvironmentRepository,
  taskEnvironmentRepositoryWorktreePath,
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
    await expect(testPage.getByTestId("recovery-restore-workspace-button")).toBeVisible();
    await testPage.keyboard.press("Escape");
    await assertNoDocumentHorizontalOverflow(testPage, "lost branch recovery error");

    // A second ordinary attempt remains retryable and must not silently switch
    // the branch or consume the explicit replacement decision.
    await testPage.getByTestId("recovery-resume-button").click();
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

  test.describe("dirty managed clone relocation", () => {
    test.describe.configure({ retries: 0 });

    test.afterEach(async ({ apiClient, seedData }) => {
      await cleanupManagedCloneRelocationFixture(apiClient, seedData);
    });

    test("moves a dirty managed worktree and resumes the same conversation", async ({
      testPage,
      apiClient,
      seedData,
      backend,
      prCapture,
    }) => {
      test.setTimeout(180_000);

      const { requestIds: recoveryRequestIds, responses: recoveryResponses } =
        captureSessionRecoveryMessages(testPage);

      const fixture = await seedManagedCloneRelocationFixture(
        testPage,
        apiClient,
        seedData,
        backend,
        `Managed clone relocation ${Date.now()}`,
      );
      const sessionId = fixture.task.session_id!;
      const originalPath = fixture.repository.worktree_path!;
      const beforeEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
      const beforeRepository = taskEnvironmentRepository(beforeEnvironment, seedData.repositoryId);
      expect(beforeRepository).toBeDefined();
      expect(beforeRepository!.branch_slug).toBe("main");
      expect(beforeRepository!.worktree_branch).toBe(fixture.originalBranch);
      expect(
        new GitHelper(fixture.sourceClonePath, makeGitEnv(backend.tmpDir))
          .exec("git remote get-url origin")
          .trim(),
      ).toBe(`https://github.com/e2e/${path.basename(fixture.sourceClonePath)}.git`);

      const stopResponse = await apiClient.stopSession({
        session_id: sessionId,
        reason: "e2e managed clone relocation",
        force: true,
      });
      expect(stopResponse.success).toBe(true);
      await waitForSessionState(apiClient, {
        taskId: fixture.task.id,
        sessionId,
        expectedState: "CANCELLED",
        message: "Waiting for the managed clone relocation session to stop",
        timeout: 30_000,
      });
      await fixture.session.recoveryResumeButton().click();

      const relocate = testPage.getByTestId("managed-clone-relocate-button");
      await expect(relocate).toBeVisible({ timeout: 30_000 });
      await expect
        .poll(() => {
          return capturedSessionRecoveryResponseType(
            recoveryRequestIds,
            recoveryResponses,
            "resume",
          );
        })
        .toBe("error");
      const resumeResponse = capturedSessionRecoveryResponse(
        recoveryRequestIds,
        recoveryResponses,
        "resume",
      );
      expect(resumeResponse).toBeDefined();
      expect(resumeResponse!.payload).toMatchObject({
        details: {
          kind: "managed_clone_relocation_required",
          error_stamp: expect.any(String),
        },
      });
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
            message: "Waiting for the stopped session runtime to exit",
          },
        )
        .toBe(false);
      await expect
        .poll(() => readManagedCloneRecoveryConsumers(backend.tmpDir, fixture.environment.id), {
          timeout: 30_000,
          intervals: [250, 500, 1_000],
          message: "Waiting for the stopped resumable runtime to settle durably",
        })
        .toEqual([{ sessionId, state: "CANCELLED", runtimeStatus: "stopped" }]);
      await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(0);
      await expect(testPage.getByTestId("recovery-fresh-button")).toHaveCount(0);
      await expect(testPage.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);
      if (prCapture.capturing) {
        await waitForFiniteAnimations(testPage.locator("body"));
        await prCapture.screenshot("managed-clone-relocation-card-desktop", {
          caption: "The task card explains why the managed clone needs repair.",
        });
      }
      await relocate.click();
      const confirmation = testPage.getByTestId("managed-clone-relocation-confirmation");
      await expect(confirmation).toBeVisible();
      await expect(confirmation).toContainText("snapshot");
      await expect(confirmation).toContainText("staging choices");
      if (prCapture.capturing) {
        await waitForFiniteAnimations(confirmation);
        await prCapture.screenshot("managed-clone-relocation-confirm-desktop", {
          caption: "The confirmation states what moves and what remains in the original checkout.",
        });
      }
      const cancel = confirmation.getByRole("button", { name: "Cancel" });
      const confirm = testPage.getByTestId("managed-clone-relocation-confirm");
      await expectMinimumElementHeight(cancel, 26);
      await expectMinimumElementHeight(confirm, 26);
      await testPage.getByTestId("managed-clone-relocation-confirm").click();
      await expect
        .poll(
          () =>
            capturedSessionRecoveryResponseType(
              recoveryRequestIds,
              recoveryResponses,
              "relocate_and_resume",
            ),
          { timeout: 30_000, message: "Waiting for managed clone relocation response" },
        )
        .toBeTruthy();
      const relocationResponse = capturedSessionRecoveryResponse(
        recoveryRequestIds,
        recoveryResponses,
        "relocate_and_resume",
      );
      assertSuccessfulRelocationResponse(
        relocationResponse,
        readManagedCloneRecoveryConsumers(backend.tmpDir, fixture.environment.id),
      );

      await fixture.session.waitForChatIdle({ timeout: 60_000 });
      let afterEnvironment: Awaited<ReturnType<typeof apiClient.getTaskEnvironment>> = null;
      let afterRepository = beforeRepository;
      await expect
        .poll(
          async () => {
            afterEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
            afterRepository = taskEnvironmentRepository(afterEnvironment, seedData.repositoryId);
            return taskEnvironmentRepositoryWorktreePath(afterEnvironment, seedData.repositoryId);
          },
          { timeout: 30_000, message: "Waiting for the relocated worktree identity" },
        )
        .not.toBe(originalPath);

      const relocatedPath = afterRepository?.worktree_path;
      expect(afterEnvironment).not.toBeNull();
      expect(afterRepository).toBeDefined();
      expect(afterEnvironment!.id).toBe(beforeEnvironment!.id);
      expect(afterRepository!.worktree_id).not.toBe(beforeRepository!.worktree_id);
      expect(afterRepository!.branch_slug).toBe(beforeRepository!.branch_slug);
      expect(afterRepository!.worktree_branch).toBe(fixture.originalBranch);
      expect(relocatedPath).toContain(".relocated-");
      const recoveryArtifacts = readPrivateManagedCloneRecoveryArtifacts(
        backend.tmpDir,
        beforeEnvironment!.id,
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
        message: "Waiting for the relocated session to resume",
        timeout: 30_000,
      });
      await expect(fixture.session.agentStatus()).toHaveCount(0, { timeout: 30_000 });
      await expect(
        fixture.session.activeChat().locator('[data-placeholder="Preparing workspace..."]'),
      ).toHaveCount(0);
      const priorMockResponses = await countSimpleMockResponses(apiClient, sessionId);
      await fixture.session.sendMessage("/e2e:simple-message");
      await expect
        .poll(() => countSimpleMockResponses(apiClient, sessionId), {
          timeout: 60_000,
          message: "Waiting for the relocated session's follow-up response",
        })
        .toBeGreaterThan(priorMockResponses);
      await expect(
        fixture.session.activeChat().getByText("simple mock response", { exact: false }).last(),
      ).toBeVisible({ timeout: 30_000 });
      await fixture.session.clickTab("Files");
      const expectedFilesPath = relocatedPath!.replace(/^\/(?:Users|home)\/[^/]+\//, "~/");
      await expect(fixture.session.files.getByTestId("file-browser-workspace-path")).toHaveText(
        expectedFilesPath,
        { timeout: 30_000 },
      );
      await assertNoDocumentHorizontalOverflow(testPage, "managed clone relocation recovery");
    });
  });
});
