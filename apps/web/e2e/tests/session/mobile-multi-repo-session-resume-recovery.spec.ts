import fs from "node:fs";
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import type { ApiClient } from "../../helpers/api-client";
import {
  assertSuccessfulRelocationResponse,
  captureSessionRecoveryMessages,
  capturedSessionRecoveryResponse,
  capturedSessionRecoveryResponseType,
  countSimpleMockResponses,
  readManagedCloneRecoveryConsumers,
} from "../../helpers/session-resume-recovery";
import {
  assertRelocatedSlot,
  assertSnapshotContent,
  capturedSessionLaunchResponses,
  captureSessionLaunchMessages,
  cleanupMultiRepoManagedCloneRelocationFixture,
  readSessionErrorStamp,
  seedMultiRepoManagedCloneRelocationFixture,
  stopAndSeedLegacySessionFailure,
  type MultiRepoRelocationFixture,
} from "../../helpers/multi-repo-managed-clone-recovery";
import { waitForSessionState } from "../../helpers/session";

type MultiRepoFixture = MultiRepoRelocationFixture;
type BackendRuntime = { tmpDir: string };
type SessionLaunchCapture = ReturnType<typeof captureSessionLaunchMessages>;
type SessionRecoveryCapture = ReturnType<typeof captureSessionRecoveryMessages>;
type SessionLaunchResponse =
  SessionLaunchCapture["responses"] extends Map<string, infer T> ? T : never;

function relocationDetails(response: SessionLaunchResponse | undefined) {
  const payload = response?.payload;
  if (typeof payload !== "object" || payload === null) return null;
  const details = (payload as { details?: unknown }).details;
  if (typeof details !== "object" || details === null) return null;
  return details as { kind?: string; error_stamp?: string; recovery_action?: string };
}

async function expectConfirmationWithinPhoneViewport(page: Page, confirmation: Locator) {
  const viewport = page.viewportSize()!;
  await expect
    .poll(
      async () => {
        const box = await confirmation.boundingBox();
        return box !== null && box.y + box.height <= viewport.height + 1;
      },
      { timeout: 5_000, message: "Waiting for the phone confirmation drawer to finish opening" },
    )
    .toBe(true);
  const box = (await confirmation.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
}

async function restoreWorkspaceAndReadRelocationStamp(
  page: Page,
  fixture: MultiRepoFixture,
  backend: BackendRuntime,
  launch: SessionLaunchCapture,
  recovery: SessionRecoveryCapture,
) {
  const restore = page.getByTestId("recovery-restore-workspace-button");
  await expect(restore).toBeVisible({ timeout: 30_000 });
  await expect(restore).toBeInViewport();
  expect((await restore.boundingBox())!.height).toBeGreaterThanOrEqual(44);

  const requestOffset = launch.requestCounts.restore_workspace ?? 0;
  await restore.tap();
  await expect
    .poll(() => launch.requestCounts.restore_workspace ?? 0)
    .toBeGreaterThan(requestOffset);
  const typedResponse = () =>
    capturedSessionLaunchResponses(launch, "restore_workspace", requestOffset).find(
      (response) => relocationDetails(response)?.kind === "managed_clone_relocation_required",
    );
  await expect
    .poll(typedResponse, {
      timeout: 30_000,
      message: "Waiting for Restore workspace to report the relocation conflict",
    })
    .toBeTruthy();

  const response = typedResponse();
  expect(response?.type).toBe("error");
  const details = relocationDetails(response);
  expect(details).toMatchObject({
    kind: "managed_clone_relocation_required",
    error_stamp: expect.any(String),
    recovery_action: "relocate_and_resume",
  });
  const durableStamp = details?.error_stamp;
  expect(readSessionErrorStamp(backend.tmpDir, fixture.task.session_id!)).toBe(durableStamp);
  expect(recovery.requestCounts.resume ?? 0).toBe(0);
  await expect(page.getByTestId("managed-clone-relocate-button")).toHaveCount(1);
  await expect(page.getByTestId("managed-clone-relocate-button")).toBeInViewport();
  await expect(page.getByTestId("recovery-resume-button")).toHaveCount(0);
  await expect(page.getByTestId("recovery-fresh-button")).toHaveCount(0);
  await expect(page.getByTestId("recovery-restore-workspace-button")).toHaveCount(0);
  return durableStamp as string;
}

async function assertPhoneRelocationCancelLeavesSourcesUntouched(
  page: Page,
  apiClient: ApiClient,
  fixture: MultiRepoFixture,
  recovery: SessionRecoveryCapture,
) {
  await page.getByTestId("managed-clone-relocate-button").tap();
  const confirmation = page.getByTestId("managed-clone-relocation-confirmation");
  await expect(confirmation).toBeVisible();
  await expect(confirmation).toContainText("snapshot");
  await expect(confirmation).toContainText("staging choices");
  await expectConfirmationWithinPhoneViewport(page, confirmation);
  const cancel = confirmation.getByRole("button", { name: "Cancel" });
  await expect(cancel).toBeInViewport();
  expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await cancel.tap();
  await expect(confirmation).toHaveCount(0);
  expect(recovery.requestCounts.relocate_and_resume ?? 0).toBe(0);
  for (const slot of fixture.slots) {
    expect(fs.existsSync(slot.originalPath)).toBe(true);
    expect(fs.readFileSync(`${slot.originalPath}/${slot.dirtyFileName}`, "utf8")).toBe(
      slot.dirtyFileContent,
    );
    expect(fs.existsSync(`${slot.originalPath}.kandev-clone-relocation.json`)).toBe(false);
  }
  expect(
    (await apiClient.getTaskEnvironment(fixture.task.id))?.repos?.map((repo) => repo.worktree_id),
  ).toEqual(fixture.environment.repos?.map((repo) => repo.worktree_id));
  await assertNoDocumentHorizontalOverflow(page, "multi-repository relocation confirmation");
}

async function confirmPhoneRelocation(
  page: Page,
  fixture: MultiRepoFixture,
  recovery: SessionRecoveryCapture,
) {
  await page.getByTestId("managed-clone-relocate-button").tap();
  const confirm = page.getByTestId("managed-clone-relocation-confirm");
  await expect(confirm).toBeInViewport();
  expect((await confirm.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await confirm.tap();
  await expect.poll(() => recovery.requestCounts.relocate_and_resume ?? 0).toBe(1);
  await expect
    .poll(
      () =>
        capturedSessionRecoveryResponseType(
          recovery.requestIds,
          recovery.responses,
          "relocate_and_resume",
        ),
      { timeout: 30_000, message: "Waiting for both phone repository relocations to finish" },
    )
    .toBeTruthy();
  expect(recovery.requestCounts.resume ?? 0).toBe(0);
  return capturedSessionRecoveryResponse(
    recovery.requestIds,
    recovery.responses,
    "relocate_and_resume",
  );
}

async function assertPhoneRelocatedSlots(
  apiClient: ApiClient,
  fixture: MultiRepoFixture,
  backend: BackendRuntime,
  recoveryResponse: ReturnType<typeof capturedSessionRecoveryResponse>,
) {
  await fixture.session.waitForChatIdle({ timeout: 60_000 });
  let afterEnvironment: Awaited<ReturnType<ApiClient["getTaskEnvironment"]>> = null;
  await expect
    .poll(
      async () => {
        afterEnvironment = await apiClient.getTaskEnvironment(fixture.task.id);
        return afterEnvironment?.repos?.filter((repository) =>
          fixture.slots.some(
            (slot) =>
              slot.repositoryId === repository.repository_id &&
              repository.worktree_path !== slot.originalPath,
          ),
        ).length;
      },
      { timeout: 30_000, message: "Waiting for both phone-recovered worktrees to be published" },
    )
    .toBe(2);
  expect(afterEnvironment?.id).toBe(fixture.environment.id);
  for (const slot of fixture.slots) {
    const relocated = afterEnvironment?.repos?.find(
      (repository) => repository.repository_id === slot.repositoryId,
    );
    expect(relocated?.worktree_path).toBeTruthy();
    expect(relocated?.worktree_path).not.toBe(slot.originalPath);
    expect(relocated?.worktree_id).not.toBe(slot.originalWorktreeId);
    expect(relocated?.worktree_branch).toBe(slot.originalBranch);
    assertSnapshotContent(slot);
    expect(fs.existsSync(slot.originalPath)).toBe(false);
    assertRelocatedSlot(slot, relocated!.worktree_path!, backend.tmpDir);
  }
  assertSuccessfulRelocationResponse(
    recoveryResponse,
    readManagedCloneRecoveryConsumers(backend.tmpDir, fixture.environment.id),
  );
}

test.describe("mobile: multi-repository managed clone recovery", () => {
  let fixture: MultiRepoRelocationFixture | null = null;

  test.describe.configure({ retries: 0 });
  test.afterEach(async ({ apiClient, seedData }) => {
    if (!fixture) return;
    await cleanupMultiRepoManagedCloneRelocationFixture(apiClient, seedData, fixture);
    fixture = null;
  });

  test("recovers a legacy multi-repository workspace through Restore first", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(240_000);
    const launch = captureSessionLaunchMessages(testPage);
    const recovery = captureSessionRecoveryMessages(testPage);
    const activeFixture = await seedMultiRepoManagedCloneRelocationFixture(
      testPage,
      apiClient,
      seedData,
      backend,
      `Mobile legacy multi-repository recovery ${Date.now()}`,
    );
    fixture = activeFixture;
    const sessionId = activeFixture.task.session_id!;
    await stopAndSeedLegacySessionFailure(
      apiClient,
      backend.tmpDir,
      activeFixture,
      "mobile e2e legacy multi-repository recovery",
    );
    await testPage.reload();
    await activeFixture.session.waitForLoad();
    const durableStamp = await restoreWorkspaceAndReadRelocationStamp(
      testPage,
      activeFixture,
      backend,
      launch,
      recovery,
    );

    await testPage.reload();
    await activeFixture.session.waitForLoad();
    await expect(testPage.getByTestId("managed-clone-relocate-button")).toHaveCount(1, {
      timeout: 30_000,
    });
    expect(readSessionErrorStamp(backend.tmpDir, sessionId)).toBe(durableStamp);
    expect(recovery.requestCounts.resume ?? 0).toBe(0);

    await assertPhoneRelocationCancelLeavesSourcesUntouched(
      testPage,
      apiClient,
      activeFixture,
      recovery,
    );
    const relocationResponse = await confirmPhoneRelocation(testPage, activeFixture, recovery);
    await assertPhoneRelocatedSlots(apiClient, activeFixture, backend, relocationResponse);

    await waitForSessionState(apiClient, {
      taskId: activeFixture.task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for the same phone session to resume",
      timeout: 30_000,
    });
    const responseCount = await countSimpleMockResponses(apiClient, sessionId);
    await activeFixture.session.sendMessageViaButton("/e2e:simple-message");
    await expect
      .poll(() => countSimpleMockResponses(apiClient, sessionId), {
        timeout: 60_000,
        message: "Waiting for a response from the relocated phone session",
      })
      .toBeGreaterThan(responseCount);
    await assertNoDocumentHorizontalOverflow(
      testPage,
      "mobile multi-repository managed clone recovery",
    );
  });
});
