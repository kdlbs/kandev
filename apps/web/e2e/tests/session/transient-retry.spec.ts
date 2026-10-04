import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { pollUntil } from "../../helpers/poll-until";
import { seedIdleSession } from "../../helpers/session";
import { listTransientRetryNotices } from "../../helpers/transient-retry";
import { SessionPage } from "../../pages/session-page";

type SessionMessage = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];

async function waitForRetryNotice(
  apiClient: ApiClient,
  sessionId: string,
  minimumAttempt: number,
  timeout = 45_000,
): Promise<SessionMessage> {
  return pollUntil(
    async () => {
      const { messages } = await apiClient.listSessionMessages(sessionId);
      return messages.find(
        (message) =>
          message.type === "status" &&
          message.metadata?.retrying === true &&
          typeof message.metadata.attempt === "number" &&
          message.metadata.attempt >= minimumAttempt,
      );
    },
    (message): message is SessionMessage => message !== undefined,
    timeout,
    `waiting for persisted transient retry attempt ${minimumAttempt}`,
  );
}

test.describe("transient provider error (529 Overloaded) retry", () => {
  test("shows the yellow retrying card, not the red error banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Retry Test");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");
    const firstRetryNotice = waitForRetryNotice(apiClient, sessionId, 1, 60_000);

    // /overloaded:9 keeps failing so the retry loop stays visible until cancel.
    await session.sendMessage("/overloaded:9");
    await firstRetryNotice;

    // The calm yellow "retrying" card + Cancel button must appear...
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 60_000 });
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.recoveryCancelRetryButton()).toBeVisible();

    // ...and the red recovery banner must NOT be shown yet (retries in flight).
    await expect(session.recoveryResumeButton()).toBeHidden();

    // Stop the retry loop before this worker starts another task.
    await session.recoveryCancelRetryButton().click();
    await expect(session.recoveryResumeButton()).toBeVisible();
  });

  test("Cancel stops the retry loop and surfaces the recovery banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Cancel Test");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");
    const firstRetryNotice = waitForRetryNotice(apiClient, sessionId, 1, 60_000);

    await session.sendMessage("/overloaded:9");
    await firstRetryNotice;

    await expect(session.transientRetryCard()).toBeVisible({ timeout: 60_000 });
    await expect(session.recoveryCancelRetryButton()).toBeVisible({ timeout: 15_000 });

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
    await expect(testPage.getByTestId("session-recovery-card")).toContainText(
      "Automatic recovery was cancelled. Resume or start fresh to continue.",
    );
    await expect(testPage.getByTestId("session-recovery-card")).not.toContainText(
      "after several retries",
    );
  });

  test("retries are paced — the attempt counter advances across the backoff", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedIdleSession(testPage, apiClient, seedData, "Overloaded Backoff Test");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");

    // /overloaded:9 keeps failing, so each backoff retry re-drives the prompt
    // and the orchestrator advances the attempt counter. Read the persisted
    // status row instead of depending on a websocket notification. The row is
    // the durable contract and remains observable when a busy gateway drops a
    // short-lived notification.
    const firstNoticePromise = waitForRetryNotice(apiClient, sessionId, 1);
    await session.sendMessage("/overloaded:9");
    const firstNotice = await firstNoticePromise;
    const firstMetadata = firstNotice.metadata as Record<string, unknown>;
    expect(firstMetadata).toMatchObject({ attempt: 1, retry_in_seconds: 5 });
    const createdAt = Date.parse(String(firstNotice.created_at));
    const retryAt = Date.parse(String(firstMetadata.retry_at));
    expect(Number.isFinite(createdAt)).toBe(true);
    expect(Number.isFinite(retryAt)).toBe(true);
    expect(retryAt - createdAt).toBeGreaterThanOrEqual(4_000);
    expect(retryAt - createdAt).toBeLessThanOrEqual(6_000);

    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toContainText(/attempt [1-5] of 5/i);

    const advancedNotice = await waitForRetryNotice(apiClient, sessionId, 2);
    const advancedMetadata = advancedNotice.metadata as Record<string, unknown>;
    expect(advancedMetadata.attempt).toBeGreaterThanOrEqual(2);

    const advanced = await listTransientRetryNotices(apiClient, sessionId);
    expect(advanced).toHaveLength(1);
    const firstNoticeId = advancedNotice.id;
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

    await session.recoveryCancelRetryButton().click();
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(session.transientRetryCard()).toBeHidden();
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

    await session.recoveryCancelRetryButton().click();
    await expect(session.recoveryResumeButton()).toBeVisible();
  });
});
