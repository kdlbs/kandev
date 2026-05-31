// Filename starts with "mobile-" so it runs on the mobile-chrome Playwright
// project (Pixel 5 emulation) — see e2e/playwright.config.ts. Mobile parity for
// the transient provider-error (529 Overloaded) retry flow: the yellow retry
// card and its Cancel button must render and work on a narrow touch viewport.
import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";

test.describe("mobile: transient provider error retry", () => {
  test("yellow retry card + Cancel works on mobile and surfaces recovery", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Mobile Overloaded Test");

    await session.sendMessageViaButton("/overloaded:9");

    // Yellow retry card + Cancel button render on the narrow viewport.
    await expect(session.transientRetryCard()).toBeVisible({ timeout: 30_000 });
    await expect(session.recoveryCancelRetryButton()).toBeVisible();
    await expect(session.recoveryResumeButton()).toBeHidden();

    // Tap Cancel → red recovery banner.
    await session.recoveryCancelRetryButton().click();
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
  });
});
