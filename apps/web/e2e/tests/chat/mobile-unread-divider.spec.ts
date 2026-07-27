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
});
