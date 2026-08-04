import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import { isScrolledIntoView, seedScrollTestConversation } from "../../helpers/unread-divider";

test.describe("Unread divider", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({ unread_divider: true });
  });

  test("shows the Slack-style New divider only for messages read before a rewound cursor, and clears after the next visit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);

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
    // attemptTimeout must be raised explicitly, not just `timeout` — its
    // default is capped at min(15_000, ...) regardless of `timeout` (see
    // SessionPage.waitForChatIdle). Past that cap it does a page.reload()
    // to recover from a stuck WS subscription, which is a *real*
    // navigate-away-and-back from this feature's own perspective — it
    // would legitimately (and correctly) re-capture a fresh divider
    // anchor, making later assertions in this test fail for a reason
    // unrelated to what they're testing (the cursor advancing live while
    // the session stays genuinely, continuously visible in one browser
    // session).
    await session.waitForChatIdle({ timeout: 60_000, attemptTimeout: 60_000 });

    // First-ever visit: there is no prior read cursor, so nothing renders as
    // "New" — but the cursor must still advance to the latest message so a
    // later visit has a real boundary to compare against.
    await expect(session.activeChat().getByTestId("unread-divider")).toHaveCount(0);
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
    await session.waitForChatIdle({ timeout: 60_000, attemptTimeout: 60_000 });
    await expect(session.activeChat().getByTestId("unread-divider")).toHaveCount(0);

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
    // the backend repository/service/handler tests. Uses the e2e-only
    // force-set backdoor rather than the production mark-read endpoint:
    // the real endpoint is monotonic (it never regresses the persisted
    // cursor), so replaying an older messageId through it would now be a
    // rejected no-op instead of rewinding state for this test.
    await apiClient.forceSetSessionReadCursor(sessionId, readCursorMessageId);

    // Navigate away, then back — the actual "navigate into a task that was
    // running outside of active view" trigger this feature targets.
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    session = await openTaskSession(testPage, task.id);

    const activeChat = session.activeChat();
    const divider = activeChat.getByTestId("unread-divider");
    await expect(divider).toBeVisible();

    // Positioned as part of the first unread message's own row (the
    // renderer nests the divider inside that message's wrapper, immediately
    // before its content) — never inside or before an already-read row.
    const firstUnreadRow = activeChat.locator(`[id="msg-${firstUnreadMessage.id}"]`);
    const readRow = activeChat.locator(`[id="msg-${readCursorMessageId}"]`);
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
    await expect(session.activeChat().getByTestId("unread-divider")).toHaveCount(0);
  });

  test("does not add a New divider when a visible session advances from its current tail", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Unread Divider Active Visit Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    let session = await openTaskSession(testPage, task.id);
    // No live-tail observation has happened yet, so the normal reload recovery
    // is safe here. The post-send wait below stays reload-free because that
    // interval is the continuously-visible behavior under test.
    await session.waitForChatIdle({ timeout: 60_000 });
    const initialMessages = await apiClient.listSessionMessages(task.session_id);
    const initialTail = initialMessages.messages[initialMessages.messages.length - 1];
    if (!initialTail) throw new Error("expected the initial transcript to contain a message");
    await expect
      .poll(
        async () => (await apiClient.getTaskSession(task.session_id!)).session.last_read_message_id,
      )
      .toBe(initialTail.id);

    // Returning at the persisted tail starts a clean visit: it has no visible
    // divider, but the hook must also discard its latent tail anchor.
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    session = await openTaskSession(testPage, task.id);
    await expect(session.activeChat().getByTestId("unread-divider")).toHaveCount(0);

    await session.sendMessageViaButton("prompt while actively reading");
    await session.waitForChatIdle({ timeout: 60_000, attemptTimeout: 60_000 });
    await expect(session.activeChat().getByTestId("unread-divider")).toHaveCount(0);
  });

  test("scrolls straight to the New divider on visit start when it's outside the initial viewport, instead of jumping to the newest message", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const { taskId, newestMessageId } = await seedScrollTestConversation(
      apiClient,
      seedData,
      "Unread Divider Scroll Test",
    );

    // First visit to a task that was running off-screen. The seeded cursor
    // must remain unread until this visit, so do not make a throwaway visit
    // before opening the task under test.
    const session = await openTaskSession(testPage, taskId);
    const activeChat = session.activeChat();
    const scrollContainer = activeChat.locator(".chat-message-list");

    const divider = activeChat.getByTestId("unread-divider");
    await expect(divider).toBeVisible();

    await expect.poll(() => isScrolledIntoView(scrollContainer, divider)).toBe(true);

    // The newest message — where a naive scroll-to-bottom would have left
    // the viewport — must NOT be in view; we scrolled up to the divider
    // instead.
    const newestRow = activeChat.locator(`[id="msg-${newestMessageId}"]`);
    expect(await isScrolledIntoView(scrollContainer, newestRow)).toBe(false);
  });

  test("reserves room for the anchored last-prompt bar so it does not cover the New divider on visit start", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    await apiClient.saveUserSettings({ show_anchored_prompt_bar: true });

    const { taskId } = await seedScrollTestConversation(
      apiClient,
      seedData,
      "Unread Divider Anchored Bar Overlap Test",
    );

    // First visit: the divider lands at the unread boundary while the task
    // description (the session's only user-authored message, seeded far
    // above it) is scrolled well past — exactly the condition that opens
    // the anchored bar at the same moment the divider is placed.
    const session = await openTaskSession(testPage, taskId);
    const activeChat = session.activeChat();

    const bar = activeChat.getByTestId("anchored-last-prompt-bar");
    await expect(bar).toHaveAttribute("data-state", "open", { timeout: 10_000 });

    const divider = activeChat.getByTestId("unread-divider");
    await expect(divider).toBeVisible();

    const barContent = activeChat.getByTestId("anchored-last-prompt-content");
    await expect
      .poll(async () => {
        const [barBox, dividerBox] = await Promise.all([
          barContent.boundingBox(),
          divider.boundingBox(),
        ]);
        if (!barBox || !dividerBox) return null;
        return dividerBox.y - (barBox.y + barBox.height);
      })
      .toBeGreaterThanOrEqual(0);
  });
});
