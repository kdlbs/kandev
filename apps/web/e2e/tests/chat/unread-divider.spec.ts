import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";

test.describe("Unread divider", () => {
  test("shows the Slack-style New divider only for messages read before a rewound cursor, and clears after the next visit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Unread Divider Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    const sessionId = task.session_id;

    let session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 30_000 });

    // First-ever visit: there is no prior read cursor, so nothing renders as
    // "New" — but the cursor must still advance to the latest message so a
    // later visit has a real boundary to compare against.
    await expect(session.chat.getByTestId("unread-divider")).toHaveCount(0);
    await expect
      .poll(async () => (await apiClient.getTaskSession(sessionId)).session.last_read_message_id)
      .not.toBeUndefined();

    const firstExchange = await apiClient.listSessionMessages(sessionId);
    expect(firstExchange.messages.length).toBeGreaterThanOrEqual(2);
    const readCursorMessageId = firstExchange.messages[firstExchange.messages.length - 1].id;

    // Grow the transcript with a second exchange while still visible — the
    // cursor keeps advancing live while the session stays in view, so this
    // alone must not produce a divider either.
    await session.sendMessageViaButton("second question");
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.chat.getByTestId("unread-divider")).toHaveCount(0);

    const fullTranscript = await apiClient.listSessionMessages(sessionId);
    expect(fullTranscript.messages.length).toBeGreaterThan(firstExchange.messages.length);
    const firstUnreadMessage = fullTranscript.messages[firstExchange.messages.length];

    // Rewind the persisted cursor back to the end of the first exchange —
    // deterministically simulating "the last time the user actually looked
    // at this task, only the first exchange existed." The newer messages
    // above are real, already-persisted transcript rows produced by the
    // mock agent above; only the read cursor is synthetically rewound here
    // (driving a second live agent turn from a truly backgrounded task
    // isn't practical from a single Playwright page). What this proves is
    // the navigation → hydration → divider-positioning path end to end;
    // cursor persistence and message-ownership validation are covered by
    // the backend repository/service/handler tests.
    await apiClient.markSessionRead(sessionId, readCursorMessageId);

    // Navigate away, then back — the actual "navigate into a task that was
    // running outside of active view" trigger this feature targets.
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    session = await openTaskSession(testPage, task.id);

    const divider = session.chat.getByTestId("unread-divider");
    await expect(divider).toBeVisible();

    // Positioned as part of the first unread message's own row (the
    // renderer nests the divider inside that message's wrapper, immediately
    // before its content) — never inside or before an already-read row.
    const firstUnreadRow = testPage.locator(`[id="msg-${firstUnreadMessage.id}"]`);
    const readRow = testPage.locator(`[id="msg-${readCursorMessageId}"]`);
    await expect(firstUnreadRow).toBeVisible();
    await expect(readRow).toBeVisible();
    await expect(firstUnreadRow.getByTestId("unread-divider")).toHaveCount(1);
    await expect(readRow.getByTestId("unread-divider")).toHaveCount(0);

    const [dividerBox, readBox] = await Promise.all([divider.boundingBox(), readRow.boundingBox()]);
    if (!dividerBox || !readBox) {
      throw new Error("expected the divider and the read row to be measurable");
    }
    expect(dividerBox.y).toBeGreaterThanOrEqual(readBox.y + readBox.height);

    // Re-visiting already advanced the cursor to the latest message, so a
    // second navigate-away-and-back shows no divider — it doesn't linger.
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    session = await openTaskSession(testPage, task.id);
    await expect(session.chat.getByTestId("unread-divider")).toHaveCount(0);
  });
});
