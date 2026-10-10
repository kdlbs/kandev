import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { waitForActiveSessionCancellationPending } from "../../helpers/session-store";
import { waitForSessionState } from "../../helpers/session";
import {
  FOLLOW_UP_PROMPT,
  FOLLOW_UP_RESPONSE,
  PARKED_PROMPT,
  QUEUED_CANCEL_PROMPT,
  queueFromBusyComposer,
  seedQueuedPauseSession,
  waitForAcceptedQueuedTurn,
  waitForSessionExecution,
} from "./queued-prompt-cancel.helpers";

test.describe("queued prompt pause", () => {
  test.describe.configure({ retries: 1 });
  let releaseBackendEnv: (() => Promise<void>) | undefined;

  test.beforeEach(async ({ backend }) => {
    releaseBackendEnv = await backend.useEnv({ KANDEV_E2E_CANCEL_HOLD_DURATION: "300ms" });
  });

  test.afterEach(async () => {
    await releaseBackendEnv?.();
    releaseBackendEnv = undefined;
  });

  test("pauses an accepted queued prompt and keeps the execution usable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session, taskId, sessionId, executionId } = await seedQueuedPauseSession(
      testPage,
      apiClient,
      seedData,
      "Queued prompt pause",
    );
    const chat = session.activeChat();
    const queueIdentity = await apiClient.getQueueSessionIdentity(taskId, sessionId);
    await apiClient.setQueueAutoMerge(queueIdentity, false);
    await apiClient.setQueueAutoRun(queueIdentity, false);
    const acceptanceMarker = `queued-pause-provider-accepted-${randomUUID()}`;
    const queuedCancelPrompt = `${QUEUED_CANCEL_PROMPT} ${acceptanceMarker}`;

    await session.sendMessage("/slow 18s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(
      chat.getByText("Running slow response (18s total)...", { exact: false }),
    ).toBeVisible({
      timeout: 25_000,
    });

    await queueFromBusyComposer(testPage, session, queuedCancelPrompt, false);
    await queueFromBusyComposer(testPage, session, PARKED_PROMPT, false);
    await expect.poll(async () => (await apiClient.getQueueStatus(queueIdentity)).count).toBe(2);
    await expect(chat.getByTestId("queue-chip")).toBeVisible();
    await waitForSessionState(apiClient, {
      taskId,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "the streaming turn should settle before queue dispatch is enabled",
      timeout: 30_000,
    });
    const queueStart = await apiClient.setQueueAutoRun(queueIdentity, true);
    expect(queueStart).toMatchObject({ auto_run: true, dispatched: true });

    const accepted = await waitForAcceptedQueuedTurn(
      apiClient,
      sessionId,
      queuedCancelPrompt,
      acceptanceMarker,
    );
    await waitForSessionState(apiClient, {
      taskId,
      sessionId,
      expectedState: "RUNNING",
      message: "the queued cancellation fixture should own an active turn",
      timeout: 30_000,
    });
    await expect.poll(async () => (await apiClient.getQueueStatus(queueIdentity)).count).toBe(1);
    await expect
      .poll(() => waitForSessionExecution(apiClient, taskId, sessionId))
      .toBe(executionId);

    const cancel = session.cancelAgentButton();
    await expect(cancel).toBeVisible();
    await cancel.click();
    await waitForActiveSessionCancellationPending(testPage, true);
    await expect(cancel).toBeDisabled();
    await expect(cancel.getByRole("status", { name: "Cancelling..." })).toBeVisible();

    await waitForActiveSessionCancellationPending(testPage, false);
    await waitForSessionState(apiClient, {
      taskId,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "pause should settle the accepted queued turn",
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.cancelAgentButton()).not.toBeVisible();
    await expect(chat.getByTestId("queue-chip")).toBeVisible();
    await expect
      .poll(async () => apiClient.getQueueStatus(queueIdentity))
      .toMatchObject({
        count: 1,
        auto_run: false,
      });
    await expect
      .poll(async () => {
        const { turns } = await apiClient.listSessionTurns(sessionId);
        return turns.find((turn) => turn.id === accepted.turnId)?.completed_at ?? null;
      })
      .not.toBeNull();
    await expect
      .poll(() => waitForSessionExecution(apiClient, taskId, sessionId))
      .toBe(executionId);

    await session.sendMessageViaButton(FOLLOW_UP_PROMPT);
    await expect(chat.getByText(FOLLOW_UP_RESPONSE, { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await waitForSessionState(apiClient, {
      taskId,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "follow-up should finish on the same session",
      timeout: 30_000,
    });

    const { messages } = await apiClient.listSessionMessages(sessionId);
    expect(
      messages.filter(
        (message) =>
          message.author_type.toLowerCase() === "user" &&
          message.content.trim() === FOLLOW_UP_PROMPT,
      ),
    ).toHaveLength(1);
    expect(
      messages.filter(
        (message) =>
          message.author_type.toLowerCase() === "agent" &&
          message.content.trim() === FOLLOW_UP_RESPONSE,
      ),
    ).toHaveLength(1);
    await expect.poll(async () => (await apiClient.getQueueStatus(queueIdentity)).count).toBe(1);
    await expect
      .poll(() => waitForSessionExecution(apiClient, taskId, sessionId))
      .toBe(executionId);
  });
});
