import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import { isScrolledIntoView, seedScrollTestConversation } from "../../helpers/unread-divider";

/**
 * Mobile parity for the Slack-style unread divider: the same
 * useSessionReadTracking/NativeMessageList code renders on mobile — this
 * confirms the divider still appears and the visit-start scroll still lands
 * on it correctly under the mobile chat layout, not just desktop's Dockview
 * panel. Setup is shared with the desktop scroll spec (see
 * helpers/unread-divider.ts) — same conversation shape, different viewport.
 */
test.describe("Mobile unread divider", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({ unread_divider: true });
  });

  test("scrolls to the New divider on visit start under the mobile chat layout", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const { taskId, newestMessageId } = await seedScrollTestConversation(
      apiClient,
      seedData,
      "Mobile Unread Divider Scroll Test",
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
    // the viewport — must NOT be in view; the mobile layout scrolled up to
    // the divider too.
    const newestRow = activeChat.locator(`[id="msg-${newestMessageId}"]`);
    expect(await isScrolledIntoView(scrollContainer, newestRow)).toBe(false);
  });

  test("does not add a New divider when a visible session advances from its current tail", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Unread Divider Active Visit Test",
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
    // is safe here and prevents a missed fast WS transition from consuming the
    // whole test timeout. The post-send wait below remains reload-free.
    await session.waitForChatIdle({ timeout: 60_000 });
    const initialMessages = await apiClient.listSessionMessages(task.session_id);
    const initialTail = initialMessages.messages[initialMessages.messages.length - 1];
    if (!initialTail) throw new Error("expected the initial transcript to contain a message");
    await expect
      .poll(
        async () => (await apiClient.getTaskSession(task.session_id!)).session.last_read_message_id,
      )
      .toBe(initialTail.id);

    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    session = await openTaskSession(testPage, task.id);
    await expect(session.activeChat().getByTestId("unread-divider")).toHaveCount(0);

    await session.sendMessageViaButton("prompt while actively reading");
    await session.waitForChatIdle({ timeout: 60_000, attemptTimeout: 60_000 });
    await expect(session.activeChat().getByTestId("unread-divider")).toHaveCount(0);
  });
});
