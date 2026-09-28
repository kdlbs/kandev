import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  SAME_TASK_LONG_SENDER_NAME,
  SAME_TASK_QUEUED_MESSAGE,
  selectSameTaskSession,
  seedSameTaskAttributionScenario,
  startSameTaskReceiverSession,
} from "./agent-message-attribution-helpers";

function mcpScript(args: Record<string, string>): string {
  return `e2e:mcp:kandev:message_task_kandev(${JSON.stringify(args)})`;
}

test("phone users can tap a real sibling-message chip to read its full sender context", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const scenario = await seedSameTaskAttributionScenario(
    apiClient,
    seedData,
    "Phone same-task sender attribution",
  );
  await testPage.goto(`/t/${scenario.taskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const receiverSessionId = await startSameTaskReceiverSession(testPage, apiClient, scenario);
  await selectSameTaskSession(testPage, session, scenario.senderSessionId);
  await session.waitForChatIdle();

  const identity = await apiClient.getQueueSessionIdentity(scenario.taskId, receiverSessionId);
  await session.sendMessageViaButton(
    mcpScript({
      task_id: scenario.taskId,
      session_id: receiverSessionId,
      prompt: SAME_TASK_QUEUED_MESSAGE,
    }),
  );

  let queuedEntry:
    | Awaited<ReturnType<typeof apiClient.getQueueStatus>>["entries"][number]
    | undefined;
  await expect
    .poll(
      async () => {
        const status = await apiClient.getQueueStatus(identity);
        queuedEntry = status.entries.find((entry) =>
          entry.content.includes(SAME_TASK_QUEUED_MESSAGE),
        );
        return queuedEntry
          ? { count: status.count, queuedBy: queuedEntry.queued_by, metadata: queuedEntry.metadata }
          : null;
      },
      { timeout: 30_000, message: "same-task MCP delivery should persist its queue metadata" },
    )
    .toMatchObject({
      count: 1,
      queuedBy: "agent",
      metadata: {
        sender_task_id: scenario.taskId,
        sender_session_id: scenario.senderSessionId,
        sender_session_name: SAME_TASK_LONG_SENDER_NAME,
      },
    });

  await selectSameTaskSession(testPage, session, receiverSessionId);

  const chat = session.activeChat();
  await chat.getByTestId("queue-chip").tap();
  const queuedRow = chat.getByTestId("queue-entry").filter({ hasText: SAME_TASK_QUEUED_MESSAGE });
  const senderBadge = queuedRow.getByTestId("sender-task-badge");
  await expect(senderBadge).toHaveAttribute(
    "aria-label",
    `From session "${SAME_TASK_LONG_SENDER_NAME}" in task "${scenario.title}"`,
  );
  const badgeBounds = await senderBadge.boundingBox();
  expect(badgeBounds).not.toBeNull();
  expect(badgeBounds!.height).toBeGreaterThanOrEqual(44);
  await senderBadge.tap();

  const context = testPage.getByTestId("sender-task-context");
  await expect(context).toBeVisible();
  await expect(context).toContainText(SAME_TASK_LONG_SENDER_NAME);
  await expect(context).toContainText(scenario.title);
  const bounds = await context.boundingBox();
  const viewportWidth = testPage.viewportSize()?.width;
  expect(bounds).not.toBeNull();
  expect(viewportWidth).toBeDefined();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewportWidth!);

  const documentWidth = await testPage.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(documentWidth).toBe(0);
});
