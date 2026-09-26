import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { seedIdleSession } from "../../helpers/session";
import {
  isTransientRetryNoticePayload,
  listTransientRetryNotices,
} from "../../helpers/transient-retry";
import { SessionPage } from "../../pages/session-page";

test.describe("transient provider error (529 Overloaded) retry", () => {
  test("shows the yellow retrying card, not the red error banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Retry Test");

    // /overloaded:9 keeps failing so the retry loop stays visible until cancel.
    await session.sendMessage("/overloaded:9");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");

    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 30_000,
      })
      .toBe(1);

    // The calm yellow "retrying" card + Cancel button must appear...
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.recoveryCancelRetryButton()).toBeVisible();

    // ...and the red recovery banner must NOT be shown yet (retries in flight).
    await expect(session.recoveryResumeButton()).toBeHidden();
  });

  test("Cancel stops the retry loop and surfaces the recovery banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Cancel Test");

    await session.sendMessage("/overloaded:9");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");
    await expect(session.recoveryCancelRetryButton()).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 30_000,
      })
      .toBe(1);

    await session.recoveryCancelRetryButton().click();

    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 30_000,
      })
      .toBe(0);

    // Cancelling falls through to the red Resume / Start-fresh recovery banner.
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryFreshButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
  });

  test("retries are paced — the attempt counter advances across the backoff", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const ws = watchWs(testPage);
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Backoff Test");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");

    let retryNoticeId: string | undefined;
    const firstNoticeAdded = ws.waitForEvent("session.message.added", {
      timeout: 45_000,
      where: (payload) => {
        if (!isTransientRetryNoticePayload(payload, sessionId)) return false;
        retryNoticeId = typeof payload.message_id === "string" ? payload.message_id : undefined;
        return retryNoticeId !== undefined;
      },
    });
    const nextAttemptUpdated = ws.waitForEvent("session.message.updated", {
      timeout: 45_000,
      where: (payload) =>
        retryNoticeId !== undefined &&
        payload.message_id === retryNoticeId &&
        isTransientRetryNoticePayload(payload, sessionId, 2),
    });

    // /overloaded:9 keeps failing, so each backoff retry re-drives the prompt
    // and the orchestrator advances the attempt counter. Observe the persisted
    // status updates through the websocket so a short-lived attempt cannot be
    // missed by polling the message list while the next backoff is in flight.
    await session.sendMessage("/overloaded:9");
    const firstNotice = await firstNoticeAdded;
    expect(firstNotice.payload.metadata).toMatchObject({ attempt: 1, retry_in_seconds: 5 });
    const firstMetadata = firstNotice.payload.metadata as Record<string, unknown>;
    const createdAt = Date.parse(String(firstNotice.payload.created_at));
    const retryAt = Date.parse(String(firstMetadata.retry_at));
    expect(Number.isFinite(createdAt)).toBe(true);
    expect(Number.isFinite(retryAt)).toBe(true);
    expect(retryAt - createdAt).toBeGreaterThanOrEqual(4_000);
    expect(retryAt - createdAt).toBeLessThanOrEqual(6_000);

    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toContainText(/attempt [1-5] of 5/i);

    const advancedEvent = await nextAttemptUpdated;
    const advancedMetadata = advancedEvent.payload.metadata as Record<string, unknown>;
    expect(advancedMetadata.attempt).toBeGreaterThanOrEqual(2);

    const advanced = await listTransientRetryNotices(apiClient, sessionId);
    expect(advanced).toHaveLength(1);
    const firstNoticeId = advanced[0].id;
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.transientRetryCard()).toContainText(/attempt [2-5] of 5/i);

    await testPage.reload();
    await session.waitForLoad();
    const afterReload = await listTransientRetryNotices(apiClient, sessionId);
    expect(afterReload).toHaveLength(1);
    expect(afterReload[0].id).toBe(firstNoticeId);
    expect(afterReload[0].attempt).toBeGreaterThanOrEqual(2);
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.transientRetryCard()).toContainText(/attempt [2-5] of 5/i);
  });

  test("a 529 on the very first turn retries (launch prompt is cached)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Start the task with the failing prompt as the INITIAL turn. Initial
    // launches go through LaunchPreparedSession, not PromptTask, so the prompt
    // must be cached there too or the retry would park behind a stuck card.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Initial Overload Test",
      seedData.agentProfileId,
      {
        description: "/overloaded:9",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.chat.getByText(/attempt 1 of 5/i)).toBeVisible({ timeout: 30_000 });
    await expect(session.chat.getByText(/attempt 2 of 5/i)).toBeVisible({ timeout: 30_000 });
  });
});
