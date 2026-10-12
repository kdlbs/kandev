import { test, expect } from "../../fixtures/test-base";
import fs from "node:fs";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionState } from "../../helpers/session";
import {
  assertSuccessfulRelocationResponse,
  captureSessionRecoveryMessages,
  capturedSessionRecoveryRequest,
  capturedSessionRecoveryResponse,
  capturedSessionRecoveryResponseType,
  countSimpleMockResponses,
  readManagedCloneRecoveryConsumers,
} from "../../helpers/session-resume-recovery";
import {
  assertRelocatedSlot,
  assertPrivateRecoveryArtifactContent,
  cleanupMultiRepoManagedCloneRelocationFixture,
  cleanupManagedCloneRecoveryGitOperationGate,
  installManagedCloneRecoveryGitOperationGate,
  readWorkspaceRecoveryOperation,
  readWorkspaceRecoveryProjectionFromStore,
  releaseManagedCloneRecoveryGitOperationGate,
  waitForMultiRepoRecoveryReady,
  readSessionErrorStamp,
  seedMultiRepoManagedCloneRelocationFixture,
  stopAndSeedLegacySessionFailure,
  type ManagedCloneRecoveryGitOperationGate,
  type MultiRepoRelocationFixture,
} from "../../helpers/multi-repo-managed-clone-recovery";

test.describe("multi-repository managed clone recovery", () => {
  let fixture: MultiRepoRelocationFixture | null = null;
  let phaseGate: ManagedCloneRecoveryGitOperationGate | null = null;

  test.describe.configure({ retries: 0 });
  test.afterEach(async ({ apiClient, seedData }) => {
    if (phaseGate) releaseManagedCloneRecoveryGitOperationGate(phaseGate);
    if (fixture) {
      await cleanupMultiRepoManagedCloneRelocationFixture(apiClient, seedData, fixture);
      fixture = null;
    }
    if (phaseGate) cleanupManagedCloneRecoveryGitOperationGate(phaseGate);
    phaseGate = null;
  });

  test("recovers a legacy multi-repository workspace", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(360_000);
    const recovery = captureSessionRecoveryMessages(testPage);
    fixture = await seedMultiRepoManagedCloneRelocationFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      `Legacy multi-repository recovery ${Date.now()}`,
    );
    const sessionId = fixture.task.session_id!;
    const beforeEnvironment = fixture.environment;

    await stopAndSeedLegacySessionFailure(
      apiClient,
      backend.tmpDir,
      fixture,
      "e2e legacy multi-repository recovery",
    );
    await testPage.reload();
    await fixture.session.waitForLoad();
    await expect(testPage.getByTestId("recovery-resume-button")).toBeVisible({ timeout: 30_000 });
    await expect(testPage.getByTestId("recovery-restore-workspace-button")).toBeVisible();

    await testPage.getByTestId("recovery-resume-button").click();
    const relocate = testPage.getByTestId("managed-clone-relocate-button");
    await expect(relocate).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(() =>
        capturedSessionRecoveryResponseType(recovery.requestIds, recovery.responses, "resume"),
      )
      .toBe("error");
    expect(recovery.requestCounts.resume).toBe(1);
    expect(capturedSessionRecoveryRequest(recovery.requests, "resume")).toMatchObject({
      task_id: fixture.task.id,
      session_id: sessionId,
      action: "resume",
    });
    const resumePayload = capturedSessionRecoveryResponse(
      recovery.requestIds,
      recovery.responses,
      "resume",
    )?.payload as { details?: { error_stamp?: string } } | undefined;
    expect(resumePayload).toMatchObject({
      details: {
        kind: "managed_clone_relocation_required",
        error_stamp: expect.any(String),
        recovery_action: "relocate_and_resume",
      },
    });
    const durableStamp = resumePayload?.details?.error_stamp;
    expect(readSessionErrorStamp(backend.tmpDir, sessionId)).toBe(durableStamp);
    await testPage.reload();
    await fixture.session.waitForLoad();
    await expect(testPage.getByTestId("managed-clone-relocate-button")).toHaveCount(1, {
      timeout: 30_000,
    });
    await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(0);
    await expect(testPage.getByTestId("recovery-fresh-button")).toHaveCount(0);
    await expect(testPage.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);
    expect(readSessionErrorStamp(backend.tmpDir, sessionId)).toBe(durableStamp);
    expect(recovery.requestCounts.resume).toBe(1);

    await relocate.click();
    const confirmation = testPage.getByTestId("managed-clone-relocation-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toContainText("snapshot");
    await expect(confirmation).toContainText("staging choices");
    await testPage.getByTestId("managed-clone-relocation-confirm").click();
    await expect.poll(() => recovery.requestCounts.relocate_and_resume ?? 0).toBe(1);
    await expect(testPage.getByTestId("workspace-recovery-agent-pending")).toBeVisible({
      timeout: 60_000,
    });
    await expect(testPage.getByTestId("workspace-recovery-progress")).toHaveAttribute(
      "data-recovery-phase",
      "resuming",
    );
    await waitForMultiRepoRecoveryReady(
      apiClient,
      backend.tmpDir,
      fixture.task.id,
      sessionId,
      beforeEnvironment.id,
    );
    await expect
      .poll(
        () =>
          capturedSessionRecoveryResponseType(
            recovery.requestIds,
            recovery.responses,
            "relocate_and_resume",
          ),
        { timeout: 30_000, message: "Waiting for the resumed agent response" },
      )
      .toBeTruthy();
    assertSuccessfulRelocationResponse(
      capturedSessionRecoveryResponse(
        recovery.requestIds,
        recovery.responses,
        "relocate_and_resume",
      ),
      readManagedCloneRecoveryConsumers(backend.tmpDir, beforeEnvironment.id),
    );
    let afterEnvironment: Awaited<ReturnType<typeof apiClient.getTaskEnvironment>> = null;
    await expect
      .poll(
        async () => {
          afterEnvironment = await apiClient.getTaskEnvironment(fixture!.task.id);
          return afterEnvironment?.repos?.filter((repository) =>
            fixture!.slots.some(
              (slot) =>
                slot.repositoryId === repository.repository_id &&
                repository.worktree_path !== slot.originalPath,
            ),
          ).length;
        },
        {
          timeout: 30_000,
          message: "Waiting for both selected repositories to publish the new worktrees",
        },
      )
      .toBe(2);
    expect(afterEnvironment?.id).toBe(beforeEnvironment.id);

    for (const slot of fixture.slots) {
      const relocated = afterEnvironment?.repos?.find(
        (repository) => repository.repository_id === slot.repositoryId,
      );
      expect(relocated?.worktree_path).toBeTruthy();
      expect(relocated?.worktree_path).not.toBe(slot.originalPath);
      expect(relocated?.worktree_id).not.toBe(slot.originalWorktreeId);
      expect(relocated?.worktree_branch).toBe(slot.originalBranch);
      expect(
        assertPrivateRecoveryArtifactContent(slot, backend.tmpDir, beforeEnvironment.id),
      ).toBeTruthy();
      expect(fs.existsSync(slot.originalPath)).toBe(false);
      assertRelocatedSlot(slot, relocated!.worktree_path!, backend.tmpDir);
    }

    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for the same multi-repository session to resume",
      timeout: 30_000,
    });
    const responseCount = await countSimpleMockResponses(apiClient, sessionId);
    await fixture.session.sendMessage("/e2e:simple-message");
    await expect
      .poll(() => countSimpleMockResponses(apiClient, sessionId), {
        timeout: 60_000,
        message: "Waiting for a response from the relocated session",
      })
      .toBeGreaterThan(responseCount);
    await assertNoDocumentHorizontalOverflow(testPage, "multi-repository managed clone recovery");
  });

  test("reopens an active migration without offering another transfer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(360_000);
    const recovery = captureSessionRecoveryMessages(testPage);
    fixture = await seedMultiRepoManagedCloneRelocationFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      `Reopen active multi-repository recovery ${Date.now()}`,
    );
    const sessionId = fixture.task.session_id!;
    const beforeEnvironment = fixture.environment;
    await stopAndSeedLegacySessionFailure(
      apiClient,
      backend.tmpDir,
      fixture,
      "e2e reopen active multi-repository recovery",
    );
    await testPage.reload();
    await fixture.session.waitForLoad();
    await testPage.getByTestId("recovery-resume-button").click();
    await expect(testPage.getByTestId("managed-clone-relocate-button")).toBeVisible({
      timeout: 30_000,
    });

    phaseGate = installManagedCloneRecoveryGitOperationGate(backend.tmpDir);
    await testPage.getByTestId("managed-clone-relocate-button").click();
    await testPage.getByTestId("managed-clone-relocation-confirm").click();
    await expect
      .poll(() => fs.existsSync(phaseGate!.startedFile), {
        timeout: 30_000,
        message: "Waiting for original-checkout retention to reach the Git operation barrier",
      })
      .toBe(true);
    await expect(testPage.getByTestId("workspace-recovery-progress")).toHaveAttribute(
      "data-recovery-phase",
      "publishing",
    );
    expect(readWorkspaceRecoveryOperation(backend.tmpDir, beforeEnvironment.id)).toMatchObject({
      state: "running",
      phase: "publishing",
      workspace_complete: false,
      agent_ready: false,
    });

    await testPage.reload();
    await fixture.session.waitForLoad();
    expect(await readWorkspaceRecoveryProjectionFromStore(testPage, sessionId)).toMatchObject({
      id: sessionId,
      task_id: fixture.task.id,
      task_environment_id: beforeEnvironment.id,
      workspace_recovery: {
        state: "running",
        phase: "publishing",
        workspace_complete: false,
        agent_ready: false,
        runner_live: true,
      },
    });
    const { session: resumedSession } = await apiClient.getTaskSession(sessionId);
    expect(resumedSession.workspace_recovery).toMatchObject({
      state: "running",
      phase: "publishing",
      workspace_complete: false,
      agent_ready: false,
      runner_live: true,
    });
    const progress = testPage.getByTestId("workspace-recovery-progress");
    await expect(progress).toBeVisible({ timeout: 30_000 });
    await expect(progress).toHaveAttribute("data-recovery-phase", "publishing");
    await expect(testPage.getByTestId("managed-clone-relocate-button")).toBeHidden();
    expect(recovery.requestCounts.relocate_and_resume).toBe(1);
    await assertNoDocumentHorizontalOverflow(testPage, "reopened workspace recovery");

    releaseManagedCloneRecoveryGitOperationGate(phaseGate);
    await waitForMultiRepoRecoveryReady(
      apiClient,
      backend.tmpDir,
      fixture.task.id,
      sessionId,
      beforeEnvironment.id,
    );
    let afterEnvironment: Awaited<ReturnType<typeof apiClient.getTaskEnvironment>> = null;
    await expect
      .poll(
        async () => {
          afterEnvironment = await apiClient.getTaskEnvironment(fixture!.task.id);
          return afterEnvironment?.repos?.filter((repository) =>
            fixture!.slots.some(
              (slot) =>
                slot.repositoryId === repository.repository_id &&
                repository.worktree_path !== slot.originalPath,
            ),
          ).length;
        },
        { timeout: 60_000, message: "Waiting for every repository move to publish" },
      )
      .toBe(2);
    expect(afterEnvironment?.id).toBe(beforeEnvironment.id);
    for (const slot of fixture.slots) {
      const relocated = afterEnvironment?.repos?.find(
        (repository) => repository.repository_id === slot.repositoryId,
      );
      expect(relocated?.worktree_path).toBeTruthy();
      expect(relocated?.worktree_path).not.toBe(slot.originalPath);
      expect(
        assertPrivateRecoveryArtifactContent(slot, backend.tmpDir, beforeEnvironment.id),
      ).toBeTruthy();
      expect(fs.existsSync(slot.originalPath)).toBe(false);
      assertRelocatedSlot(slot, relocated!.worktree_path!, backend.tmpDir);
    }
    expect(recovery.requestCounts.relocate_and_resume).toBe(1);
    expect(
      readManagedCloneRecoveryConsumers(backend.tmpDir, beforeEnvironment.id).length,
    ).toBeGreaterThan(0);
    await expect(progress).toHaveCount(0, { timeout: 30_000 });

    await fixture.session.clickTab("Files");
    const repositoryNames = await Promise.all(
      fixture.slots.map(
        async (slot) => (await apiClient.getRepository(slot.repositoryId))?.name ?? "",
      ),
    );
    await expect
      .poll(async () => {
        const labels = await testPage.getByTestId("file-tree-node").allTextContents();
        return repositoryNames.every(
          (name) => name && labels.some((label) => label.includes(name)),
        );
      })
      .toBe(true);

    const slot = fixture.slots[0];
    const repositoryName = repositoryNames[0];
    await fixture.session.fileSearchButton().click();
    await fixture.session.fileSearchInput().fill(slot.dirtyFileName);
    const searchResult = testPage
      .getByTestId("file-search-result")
      .filter({ hasText: repositoryName })
      .first();
    await expect(searchResult).toBeVisible({ timeout: 15_000 });
    const canonicalPath = await searchResult.getAttribute("data-path");
    expect(canonicalPath).toContain(slot.dirtyFileName);
    expect(canonicalPath).toBeTruthy();
    await searchResult.click({ button: "right" });
    await fixture.session.fileTreeAddToChatContextMenuItem().click();
    await fixture.session.clickSessionChatTab();
    await expect(fixture.session.chatContextFile(canonicalPath!)).toContainText(repositoryName);
    await assertNoDocumentHorizontalOverflow(testPage, "repository-labelled Files search");
    const responseCount = await countSimpleMockResponses(apiClient, sessionId);
    await fixture.session.sendMessage("/e2e:simple-message");
    await expect
      .poll(() => countSimpleMockResponses(apiClient, sessionId), {
        timeout: 60_000,
        message: "Waiting for a response from the recovered session",
      })
      .toBeGreaterThan(responseCount);
  });
});
