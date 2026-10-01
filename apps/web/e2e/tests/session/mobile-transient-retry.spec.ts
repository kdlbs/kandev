// Filename starts with "mobile-" so it runs on the mobile-chrome Playwright
// project (Pixel 5 emulation) — see e2e/playwright.config.ts. Mobile parity for
// the transient provider-error (529 Overloaded) retry flow: the yellow retry
// card and its Cancel button must render and work on a narrow touch viewport.
import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { listTransientRetryNotices } from "../../helpers/transient-retry";

test.describe("mobile: transient provider error retry", () => {
  test("yellow retry card + Cancel works on mobile and surfaces recovery", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile Overloaded Test");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");

    await session.sendMessageViaButton("/overloaded:9");
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 10_000,
        intervals: [250],
        message: "the visible transient retry notice should be persisted",
      })
      .toBe(1);
    const [retryNotice] = await listTransientRetryNotices(apiClient, sessionId);
    if (!retryNotice) throw new Error("the persisted transient retry notice was not found");
    const retryNoticeId = retryNotice.id;

    // Yellow retry card + Cancel button render on the narrow viewport.
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.recoveryCancelRetryButton()).toBeVisible();
    await expect(session.recoveryResumeButton()).toBeHidden();

    expect(retryNotice.attempt).toBeGreaterThanOrEqual(1);
    await assertNoDocumentHorizontalOverflow(testPage);

    // Tap Cancel → red recovery banner.
    await session.recoveryCancelRetryButton().tap();
    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 30_000,
        message: `retry notice ${retryNoticeId} should be deleted after cancel`,
      })
      .toBe(0);
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toBeHidden();
  });
});
