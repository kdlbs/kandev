import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";

const ABANDONED_ANSWER = "Abandoned response attempt answer.";
const ABANDONED_REASONING = "Abandoned response attempt reasoning.";
const REPLACEMENT_ANSWER = "Replacement response after provider retry.";

test.describe("mobile: provider response retry", () => {
  test("retracts abandoned output live and after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile Provider Retry");
    const taskId = new URL(testPage.url()).pathname.split("/").filter(Boolean).at(-1);
    if (!taskId) throw new Error("task route did not contain a task ID");
    const { sessions } = await apiClient.listTaskSessions(taskId);
    const sessionId = sessions[0]?.id;
    if (!sessionId) throw new Error("seeded task did not contain a session");

    await session.sendMessageViaButton("/e2e:response-retry");

    await expect(
      session.activeChat().getByText(ABANDONED_REASONING, { exact: true }),
    ).toBeVisible();
    await expect(session.activeChat().getByText(ABANDONED_ANSWER, { exact: true })).toBeVisible();

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
        { timeout: 30_000, message: "waiting for the durable mobile replacement-only transcript" },
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
