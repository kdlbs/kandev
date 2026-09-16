import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { seedIdleSession } from "../../helpers/session";

const ABANDONED_ANSWER = "Abandoned response attempt answer.";
const ABANDONED_REASONING = "Abandoned response attempt reasoning.";
const REPLACEMENT_ANSWER = "Replacement response after provider retry.";

function payloadMessageId(payload: Record<string, unknown>): string | undefined {
  return typeof payload.message_id === "string" ? payload.message_id : undefined;
}

function payloadMessageType(payload: Record<string, unknown>): string | undefined {
  if (typeof payload.message_type === "string") return payload.message_type;
  return typeof payload.type === "string" ? payload.type : undefined;
}

test.describe("provider response retry", () => {
  test("retracts abandoned output live and after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const ws = watchWs(testPage);
    const session = await seedIdleSession(testPage, apiClient, seedData, "Provider Response Retry");
    const taskId = new URL(testPage.url()).pathname.split("/").filter(Boolean).at(-1);
    if (!taskId) throw new Error("task route did not contain a task ID");
    const { sessions } = await apiClient.listTaskSessions(taskId);
    const sessionId = sessions[0]?.id;
    if (!sessionId) throw new Error("seeded task did not contain a session");

    let abandonedReasoningId: string | undefined;
    let abandonedAnswerId: string | undefined;
    const retractedMessageIds = new Set<string>();
    const abandonedReasoningAdded = ws.waitForEvent("session.message.added", {
      where: (payload) => {
        if (payload.session_id !== sessionId || payloadMessageType(payload) !== "thinking") {
          return false;
        }
        abandonedReasoningId = payloadMessageId(payload);
        return abandonedReasoningId !== undefined;
      },
    });
    const abandonedAnswerAdded = ws.waitForEvent("session.message.added", {
      where: (payload) => {
        if (payload.session_id !== sessionId || payload.content !== ABANDONED_ANSWER) return false;
        abandonedAnswerId = payloadMessageId(payload);
        return abandonedAnswerId !== undefined;
      },
    });
    const abandonedReasoningDeleted = ws.waitForEvent("session.message.deleted", {
      where: (payload) => {
        const messageId = payloadMessageId(payload);
        if (messageId === undefined || messageId !== abandonedReasoningId) return false;
        retractedMessageIds.add(messageId);
        return true;
      },
    });
    const abandonedAnswerDeleted = ws.waitForEvent("session.message.deleted", {
      where: (payload) => {
        const messageId = payloadMessageId(payload);
        if (messageId === undefined || messageId !== abandonedAnswerId) return false;
        retractedMessageIds.add(messageId);
        return true;
      },
    });
    const replacementAdded = ws.waitForEvent("session.message.added", {
      where: (payload) =>
        payload.session_id === sessionId &&
        payload.content === REPLACEMENT_ANSWER &&
        retractedMessageIds.size === 2,
    });

    await session.sendMessage("/e2e:response-retry");

    await Promise.all([
      abandonedReasoningAdded,
      abandonedAnswerAdded,
      abandonedReasoningDeleted,
      abandonedAnswerDeleted,
      replacementAdded,
    ]);

    await expect(session.activeChat().getByText(REPLACEMENT_ANSWER, { exact: true })).toBeVisible();
    await expect(session.activeChat().getByText(ABANDONED_ANSWER, { exact: true })).toHaveCount(0);
    await expect(session.activeChat().getByText(ABANDONED_REASONING, { exact: true })).toHaveCount(
      0,
    );
    await session.waitForChatIdle({ timeout: 30_000 });

    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(sessionId);
          const text = (message: (typeof messages)[number]) =>
            String(message.metadata?.thinking ?? message.content ?? "");
          return {
            abandonedAnswer: messages.some((message) => text(message).includes(ABANDONED_ANSWER)),
            abandonedReasoning: messages.some((message) =>
              text(message).includes(ABANDONED_REASONING),
            ),
            replacementCount: messages.filter((message) =>
              text(message).includes(REPLACEMENT_ANSWER),
            ).length,
          };
        },
        { timeout: 30_000, message: "waiting for the durable replacement-only transcript" },
      )
      .toEqual({ abandonedAnswer: false, abandonedReasoning: false, replacementCount: 1 });

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.activeChat().getByText(REPLACEMENT_ANSWER, { exact: true })).toBeVisible();
    await expect(session.activeChat().getByText(ABANDONED_ANSWER, { exact: true })).toHaveCount(0);
    await expect(session.activeChat().getByText(ABANDONED_REASONING, { exact: true })).toHaveCount(
      0,
    );
  });
});
