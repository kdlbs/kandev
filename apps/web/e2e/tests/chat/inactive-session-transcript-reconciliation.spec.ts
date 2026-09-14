import { test, expect, type Page, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForStableActiveSession } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";
import { scrollToOldestLoadedEdge } from "./message-pagination-helpers";
import type { Message } from "@/lib/types/http";

const INITIAL_RECEIVER_MARKER = "INACTIVE-RECEIVER-INITIAL-8D4K";
const PEER_PROMPT_MARKER = "INACTIVE-RECEIVER-PEER-PROMPT-2J7M";
const RECENT_RECEIVER_MARKER = "INACTIVE-RECEIVER-RECENT-5Q9V";
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type E2EMessageStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      messages: { bySession: Record<string, Message[]> };
      setMessages: (sessionId: string, messages: Message[]) => void;
    };
  };
};

async function snapshotCachedMessages(page: Page, sessionId: string): Promise<Message[]> {
  return page.evaluate((sid) => {
    const store = (window as E2EMessageStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    return store.getState().messages.bySession[sid] ?? [];
  }, sessionId);
}

async function restoreCachedMessages(
  page: Page,
  sessionId: string,
  messages: Message[],
): Promise<void> {
  await page.evaluate(
    ({ sid, cached }) => {
      const store = (window as E2EMessageStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.getState().setMessages(sid, cached);
    },
    { sid: sessionId, cached: messages },
  );
}

async function createTaskWithTwoSessions(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ taskId: string; senderId: string; receiverId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Inactive sibling transcript",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 30_000, message: "Waiting for the first session to finish" },
    )
    .toBe(true);

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.openNewSessionDialog();
  await session.newSessionPromptInput().fill("/e2e:simple-message");
  await session.newSessionStartButton().click();
  await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.filter((candidate) => DONE_STATES.includes(candidate.state)).length;
      },
      { timeout: 60_000, message: "Waiting for both sessions to finish" },
    )
    .toBe(2);

  const { sessions } = await apiClient.listTaskSessions(task.id);
  const sorted = sessions.sort(
    (left, right) => new Date(left.started_at).getTime() - new Date(right.started_at).getTime(),
  );
  return { taskId: task.id, senderId: sorted[0].id, receiverId: sorted[1].id };
}

test.describe("inactive session transcript reconciliation", () => {
  // @covers AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
  test("reaches an attributed sibling prompt after the receiver cache becomes disjoint", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { taskId, senderId, receiverId } = await createTaskWithTwoSessions(
      testPage,
      apiClient,
      seedData,
    );

    await apiClient.seedSessionMessage(receiverId, {
      type: "message",
      content: INITIAL_RECEIVER_MARKER,
      authorType: "user",
    });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sessionTabBySessionId(receiverId)).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.sessionTabBySessionId(senderId)).toBeVisible({ timeout: 15_000 });

    await session.sessionTabBySessionId(receiverId).click();
    await waitForStableActiveSession(testPage, receiverId);
    await expect(
      session.activeChat().getByText(INITIAL_RECEIVER_MARKER, { exact: true }),
    ).toBeVisible();
    const staleReceiverCache = await snapshotCachedMessages(testPage, receiverId);

    await session.sessionTabBySessionId(senderId).click();
    await waitForStableActiveSession(testPage, senderId);

    await apiClient.seedSessionMessage(receiverId, {
      type: "message",
      content: PEER_PROMPT_MARKER,
      authorType: "user",
      metadata: {
        sender_task_id: taskId,
        sender_task_title: "Inactive sibling transcript",
        sender_session_id: senderId,
      },
    });
    await apiClient.seedToolCallMessages(receiverId, 105);
    await apiClient.seedSessionMessage(receiverId, {
      type: "message",
      content: RECENT_RECEIVER_MARKER,
      authorType: "agent",
    });
    await testPage.waitForFunction(
      ({ sid, marker }) => {
        const store = (window as E2EMessageStoreWindow).__KANDEV_E2E_STORE__;
        return store
          ?.getState()
          .messages.bySession[sid]?.some((message) => message.content === marker);
      },
      { sid: receiverId, marker: RECENT_RECEIVER_MARKER },
      {
        message:
          "RECENT_RECEIVER_MARKER should arrive in the inactive receiver cache via live delivery",
      },
    );
    await restoreCachedMessages(testPage, receiverId, staleReceiverCache);
    await testPage.evaluate(() => window.dispatchEvent(new Event("focus")));
    await testPage.waitForFunction(
      ({ sid, marker }) => {
        const store = (window as E2EMessageStoreWindow).__KANDEV_E2E_STORE__;
        return store
          ?.getState()
          .messages.bySession[sid]?.some((message) => message.content === marker);
      },
      { sid: receiverId, marker: RECENT_RECEIVER_MARKER },
      {
        message: "Foreground recovery should reconcile the receiver's latest message window",
      },
    );

    const persisted = await apiClient.listSessionMessages(receiverId);
    expect(
      persisted.messages.slice(-100).some((message) => message.content === PEER_PROMPT_MARKER),
    ).toBe(false);

    await session.sessionTabBySessionId(receiverId).click();
    await waitForStableActiveSession(testPage, receiverId);
    await session.waitForLoad();
    const chat = session.activeChat();
    const list = chat.locator(".chat-message-list");
    await expect(chat.getByText(RECENT_RECEIVER_MARKER, { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    await scrollToOldestLoadedEdge(list, RECENT_RECEIVER_MARKER);

    const peerRow = chat.locator("[id^='msg-']").filter({ hasText: PEER_PROMPT_MARKER });
    await expect(peerRow).toHaveCount(1);
    await expect(peerRow.getByText(PEER_PROMPT_MARKER, { exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(peerRow.getByTestId("sender-task-badge")).toHaveCount(1);
  });
});
