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

    await session.sendMessageViaButton("/overloaded:9");
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("active chat did not expose a session id");

    await expect
      .poll(
        async () => {
          const notices = await listTransientRetryNotices(apiClient, sessionId);
          return notices.length === 1 && notices[0].attempt === 1;
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    // Yellow retry card + Cancel button render on the narrow viewport.
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.recoveryCancelRetryButton()).toBeVisible();
    await expect(session.recoveryResumeButton()).toBeHidden();

    await expect
      .poll(
        async () => {
          const notices = await listTransientRetryNotices(apiClient, sessionId);
          return notices.length === 1 && notices[0].attempt >= 2;
        },
        { timeout: 30_000 },
      )
      .toBe(true);
    const advanced = await listTransientRetryNotices(apiClient, sessionId);
    expect(advanced).toHaveLength(1);
    const firstNoticeId = advanced[0].id;
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.transientRetryCard()).toContainText(/attempt [2-5] of 5/i);

    await testPage.reload();
    await session.waitForLoad();
    await expect
      .poll(
        async () => {
          const notices = await listTransientRetryNotices(apiClient, sessionId);
          return notices.length === 1 && notices[0].attempt >= 2;
        },
        { timeout: 30_000 },
      )
      .toBe(true);
    const afterReload = await listTransientRetryNotices(apiClient, sessionId);
    expect(afterReload).toHaveLength(1);
    expect(afterReload[0].id).toBe(firstNoticeId);
    await expect(session.transientRetryCard()).toHaveCount(1);
    await expect(session.transientRetryCard()).toContainText(/attempt [2-5] of 5/i);
    await assertNoDocumentHorizontalOverflow(testPage);

    // Tap Cancel → red recovery banner.
    await session.recoveryCancelRetryButton().tap();
    await expect
      .poll(async () => (await listTransientRetryNotices(apiClient, sessionId)).length, {
        timeout: 30_000,
      })
      .toBe(0);
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    await expect(session.transientRetryCard()).toBeHidden();
  });
});
