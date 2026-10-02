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
  assertSnapshotContent,
  cleanupMultiRepoManagedCloneRelocationFixture,
  readSessionErrorStamp,
  seedMultiRepoManagedCloneRelocationFixture,
  stopAndSeedLegacySessionFailure,
  type MultiRepoRelocationFixture,
} from "../../helpers/multi-repo-managed-clone-recovery";

test.describe("multi-repository managed clone recovery", () => {
  let fixture: MultiRepoRelocationFixture | null = null;

  test.describe.configure({ retries: 0 });
  test.afterEach(async ({ apiClient, seedData }) => {
    if (!fixture) return;
    await cleanupMultiRepoManagedCloneRelocationFixture(apiClient, seedData, fixture);
    fixture = null;
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
    await expect
      .poll(
        () =>
          capturedSessionRecoveryResponseType(
            recovery.requestIds,
            recovery.responses,
            "relocate_and_resume",
          ),
        { timeout: 30_000, message: "Waiting for both repository relocations to finish" },
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

    await fixture.session.waitForChatIdle({ timeout: 60_000 });
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
      expect(assertSnapshotContent(slot)).toBeTruthy();
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
});
