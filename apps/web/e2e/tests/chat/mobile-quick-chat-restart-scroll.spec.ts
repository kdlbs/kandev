import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionState } from "../../helpers/session";
import { routeMainWebSocketWithMessageListResponseHold } from "../../helpers/ws-response-hold";
import { readQuickChatViewportLayout } from "./quick-chat-helpers";
import {
  cleanupRestartScrollFixture,
  closeQuickChat,
  createRestartScrollFixture,
  expectActiveResumeStatusInViewport,
  expectLatestReplyAtBottom,
  expectHistoryRefreshPending,
  expectHistoryRefreshSettled,
  expectNoPromptReplay,
  expectReaderPositionPreserved,
  expectRestoredSessionIdentity,
  openQuickChatConversation,
  quickChatScroller,
  readerReturnToBottom,
  RESTART_SCROLL_OPENING_PROMPT,
  RESTART_SCROLL_RESUME_DELAY,
  touchReaderUp,
} from "./quick-chat-restart-scroll-helpers";

let originalAutoScrollControl = true;

test.describe("Mobile Quick Chat restart scroll", () => {
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

  test("keeps the newest reply and resume status visible after a phone restart", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(300_000);
    const fixture = await createRestartScrollFixture({
      page: testPage,
      apiClient,
      mobile: true,
      resumeDelay: RESTART_SCROLL_RESUME_DELAY,
    });
    try {
      await backend.restart();
      const refreshHold = await routeMainWebSocketWithMessageListResponseHold(testPage);
      await testPage.reload();
      await testPage.waitForLoadState("networkidle");

      let dialog = await openQuickChatConversation(testPage, fixture.sessionId, true);
      let scroller = quickChatScroller(dialog);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      const resumeStatus = await expectActiveResumeStatusInViewport(dialog, scroller);

      const layout = await readQuickChatViewportLayout(dialog);
      expect(layout.composerBottom).toBeLessThanOrEqual(layout.dialogBottom + 1);
      expect(layout.dialogScrollHeight).toBeLessThanOrEqual(layout.dialogClientHeight + 1);
      await assertNoDocumentHorizontalOverflow(testPage);

      await waitForSessionState(apiClient, {
        taskId: fixture.taskId,
        sessionId: fixture.sessionId,
        expectedState: "WAITING_FOR_INPUT",
        message: "phone Quick Chat did not finish restoring after the backend restart",
        timeout: 120_000,
      });
      await expect(resumeStatus).toBeHidden({ timeout: 120_000 });
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      await expectNoPromptReplay(apiClient, fixture.sessionId, RESTART_SCROLL_OPENING_PROMPT);
      await expectRestoredSessionIdentity(apiClient, fixture);

      await closeQuickChat(testPage, dialog, true);
      refreshHold.holdNextLatestWindow(fixture.sessionId);
      dialog = await openQuickChatConversation(testPage, fixture.sessionId, true);
      scroller = quickChatScroller(dialog);
      await expect.poll(refreshHold.heldCount, { timeout: 10_000 }).toBeGreaterThanOrEqual(1);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
      await expectHistoryRefreshPending(scroller);

      const readerPosition = await touchReaderUp(testPage, scroller);
      refreshHold.releaseHeldResponse();
      await expectHistoryRefreshSettled(testPage, scroller);
      await expectReaderPositionPreserved(
        scroller,
        readerPosition,
        "phone reader touch position must survive the settled history refresh",
        150,
      );
      await expect
        .poll(
          async () =>
            scroller.evaluate(
              (element) => element.scrollHeight - element.scrollTop - element.clientHeight,
            ),
          { message: "settled phone refresh must not snap the reader to the bottom" },
        )
        .toBeGreaterThan(100);
      await readerReturnToBottom(testPage, scroller, true);
      await expectLatestReplyAtBottom(dialog, scroller, fixture.latestReply);
    } finally {
      await cleanupRestartScrollFixture(apiClient, fixture);
    }
  });
});
