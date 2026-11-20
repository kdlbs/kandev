import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { routeMainWebSocketWithMessageListResponseHold } from "../../helpers/ws-response-hold";
import { readQuickChatViewportLayout } from "./quick-chat-helpers";
import {
  cleanupRestartScrollFixture,
  closeQuickChat,
  createRestartScrollFixture,
  expectLatestReplyAtBottom,
  expectActiveResumeStatusInViewport,
  expectHistoryRefreshPending,
  expectHistoryRefreshSettled,
  expectNoPromptReplay,
  expectReaderPositionPreserved,
  expectRestoredSessionIdentity,
  expectScrollerAtBottom,
  openQuickChatConversation,
  quickChatScroller,
  readerReturnToBottom,
  RESTART_SCROLL_OPENING_PROMPT,
  RESTART_SCROLL_RESUME_DELAY,
  wheelReaderUp,
} from "./quick-chat-restart-scroll-helpers";

let originalAutoScrollControl = true;

test.describe("Quick Chat restart scroll", () => {
  test.beforeAll(async ({ apiClient }) => {
    const settings = await apiClient.getUserSettings();
    originalAutoScrollControl = settings.settings.show_transcript_auto_scroll_control ?? true;
    await apiClient.saveUserSettings({ show_transcript_auto_scroll_control: true });
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.saveUserSettings({
      show_transcript_auto_scroll_control: originalAutoScrollControl,
    });
  });

  test("restores the newest reply and active resume status after a backend restart", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(300_000);
    await testPage.setViewportSize({ width: 1440, height: 800 });
    const fixture = await createRestartScrollFixture({
      page: testPage,
      apiClient,
      mobile: false,
      resumeDelay: RESTART_SCROLL_RESUME_DELAY,
    });
    try {
      await backend.restart();
      // Arm the socket interceptor before reload so later cached reopens can
      // hold the latest-window refresh without racing socket creation.
      const refreshHold = await routeMainWebSocketWithMessageListResponseHold(testPage);
      await testPage.reload();
      await testPage.waitForLoadState("networkidle");

      let dialog = await openQuickChatConversation(testPage, fixture.sessionId, false);
      let scroller = quickChatScroller(dialog);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      const layout = await readQuickChatViewportLayout(dialog);
      expect(layout.composerBottom).toBeLessThanOrEqual(layout.dialogBottom + 1);
      expect(layout.dialogScrollHeight).toBeLessThanOrEqual(layout.dialogClientHeight + 1);
      const resumeStatus = await expectActiveResumeStatusInViewport(dialog, scroller);

      await waitForSessionState(apiClient, {
        taskId: fixture.taskId,
        sessionId: fixture.sessionId,
        expectedState: "WAITING_FOR_INPUT",
        message: "Quick Chat did not finish restoring after the backend restart",
        timeout: 120_000,
      });
      await expect(resumeStatus).toBeHidden({ timeout: 120_000 });
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      await expectNoPromptReplay(apiClient, fixture.sessionId, RESTART_SCROLL_OPENING_PROMPT);
      await expectRestoredSessionIdentity(apiClient, fixture);

      // A cached reopen renders stored rows before its refresh settles. The
      // pending overflow-anchor override proves the held response is still
      // outstanding; its removal below proves the client applied it.
      await closeQuickChat(testPage, dialog, false);
      refreshHold.holdNextLatestWindow(fixture.sessionId);
      dialog = await openQuickChatConversation(testPage, fixture.sessionId, false);
      scroller = quickChatScroller(dialog);
      await expect.poll(refreshHold.heldCount, { timeout: 10_000 }).toBeGreaterThanOrEqual(1);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      await expectHistoryRefreshPending(scroller);
      refreshHold.releaseHeldResponse();
      await expectHistoryRefreshSettled(testPage, scroller);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);

      // The expanded desktop width keeps the same bottom placement.
      await closeQuickChat(testPage, dialog, false);
      await testPage.evaluate(() => {
        localStorage.setItem("kandev.quickChat.dialogWidth", JSON.stringify(1400));
      });
      await testPage.reload();
      await testPage.waitForLoadState("networkidle");
      dialog = await openQuickChatConversation(testPage, fixture.sessionId, false);
      const expandedWidth = await dialog.evaluate(
        (element) => element.getBoundingClientRect().width,
      );
      expect(expandedWidth).toBeGreaterThan(1200);
      scroller = quickChatScroller(dialog);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
    } finally {
      await cleanupRestartScrollFixture(apiClient, fixture);
    }
  });

  test("keeps a cached reopen at the bottom while history refresh is pending", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(180_000);
    await testPage.setViewportSize({ width: 1440, height: 800 });
    const fixture = await createRestartScrollFixture({ page: testPage, apiClient, mobile: false });
    try {
      // The interceptor must be installed before the page opens its gateway
      // socket; an existing connection is never re-routed.
      const refreshHold = await routeMainWebSocketWithMessageListResponseHold(testPage);
      await testPage.reload();
      await testPage.waitForLoadState("networkidle");
      let dialog = await openQuickChatConversation(testPage, fixture.sessionId, false);
      let scroller = quickChatScroller(dialog);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      await closeQuickChat(testPage, dialog, false);

      refreshHold.holdNextLatestWindow(fixture.sessionId);
      dialog = await openQuickChatConversation(testPage, fixture.sessionId, false);
      scroller = quickChatScroller(dialog);
      await expect.poll(refreshHold.heldCount, { timeout: 10_000 }).toBeGreaterThanOrEqual(1);
      // Cached rows must already rest at the newest reply before the refresh
      // arrives; releasing it must not move the transcript away from there.
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      await expectScrollerAtBottom(scroller, "cached Quick Chat should stay at the bottom");
      await expectHistoryRefreshPending(scroller);

      const readerPosition = await wheelReaderUp(testPage, scroller);
      refreshHold.releaseHeldResponse();
      await expectHistoryRefreshSettled(testPage, scroller);
      // The settled refresh may insert timestamp rows above the reader; the
      // contract is that follow intent is not stolen, not pixel immobility.
      await expectReaderPositionPreserved(
        scroller,
        readerPosition,
        "reader scroll must survive the settled history refresh",
        150,
      );
      await expect
        .poll(
          async () =>
            scroller.evaluate(
              (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
            ),
          { message: "settled refresh must not snap the reader to the bottom" },
        )
        .toBeGreaterThan(100);
      await readerReturnToBottom(testPage, scroller, false);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
    } finally {
      await cleanupRestartScrollFixture(apiClient, fixture);
    }
  });

  test("restores a disabled transcript's saved position after a backend restart", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(240_000);
    await testPage.setViewportSize({ width: 1440, height: 800 });
    const fixture = await createRestartScrollFixture({ page: testPage, apiClient, mobile: false });
    try {
      let dialog = await openQuickChatConversation(testPage, fixture.sessionId, false);
      let scroller = quickChatScroller(dialog);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);

      const savedScrollTop = await scroller.evaluate((element) => {
        element.scrollTop = 400;
        element.dispatchEvent(new Event("scroll"));
        return element.scrollTop;
      });
      expect(savedScrollTop).toBeLessThan(1000);
      const toggle = dialog.getByTestId("auto-scroll-toggle-button");
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-pressed", "false");

      await backend.restart();
      await testPage.reload();
      await testPage.waitForLoadState("networkidle");
      dialog = await openQuickChatConversation(testPage, fixture.sessionId, false);
      scroller = quickChatScroller(dialog);
      await expect
        .poll(
          async () => scroller.evaluate((element) => element.scrollHeight - element.clientHeight),
          {
            timeout: 30_000,
            message: "disabled Quick Chat history did not load after the restart",
          },
        )
        .toBeGreaterThan(200);
      await expectReaderPositionPreserved(
        scroller,
        savedScrollTop,
        "disabled Quick Chat must restore its saved offset after restart",
      );
      const gap = await scroller.evaluate(
        (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
      );
      expect(gap).toBeGreaterThan(100);
    } finally {
      await cleanupRestartScrollFixture(apiClient, fixture);
    }
  });
});
