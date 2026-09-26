// Filename starts with "mobile-" so it runs on the mobile-chrome Playwright
// project (Pixel 5 emulation) — see e2e/playwright.config.ts. Mobile parity for
// the transient provider-error (529 Overloaded) retry flow: the yellow retry
// card and its Cancel button must render and work on a narrow touch viewport.
import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { seedIdleSession } from "../../helpers/session";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  isTransientRetryNoticePayload,
  listTransientRetryNotices,
} from "../../helpers/transient-retry";

test.describe("mobile: transient provider error retry", () => {
  test("yellow retry card + Cancel works on mobile and surfaces recovery", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile Overloaded Test");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");

    const retryNoticeAdded = ws.waitForEvent("session.message.added", {
      timeout: 30_000,
      where: (payload) => isTransientRetryNoticePayload(payload, sessionId),
    });

    await session.sendMessageViaButton("/overloaded:9");
    const retryNotice = await retryNoticeAdded;
    const retryNoticeId = retryNotice.payload.message_id;
    if (typeof retryNoticeId !== "string") {
      throw new Error("transient retry event did not expose a message id");
    }

    // Yellow retry card + Cancel button render on the narrow viewport.
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.recoveryCancelRetryButton()).toBeVisible();
    await expect(session.recoveryResumeButton()).toBeHidden();

    const persistedNotice = await listTransientRetryNotices(apiClient, sessionId);
    expect(persistedNotice).toHaveLength(1);
    expect(persistedNotice[0].id).toBe(retryNoticeId);
    expect(persistedNotice[0].attempt).toBeGreaterThanOrEqual(1);
    await assertNoDocumentHorizontalOverflow(testPage);

    // Tap Cancel → red recovery banner.
    const retryNoticeDeleted = ws.waitForEvent("session.message.deleted", {
      where: (payload) => payload.session_id === sessionId && payload.message_id === retryNoticeId,
    });
    await session.recoveryCancelRetryButton().tap();
    await retryNoticeDeleted;
    expect(await listTransientRetryNotices(apiClient, sessionId)).toHaveLength(0);
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toBeHidden();
  });
});
