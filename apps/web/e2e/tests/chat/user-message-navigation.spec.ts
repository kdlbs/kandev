import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  CURRENT_USER_PROMPT,
  OLD_USER_PROMPT,
  expectLegacyRowArrowsAbsent,
  expectMessageAtNavigationPosition,
  openLongHistory,
  seedLongUserMessageHistory,
} from "./user-message-navigation-helpers";

test.describe("@chat user message navigation", () => {
  for (const renderer of ["native", "virtuoso"] as const) {
    test(`reveals and navigates paginated history with the ${renderer} renderer`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      test.setTimeout(180_000);
      const { taskId, sessionId } = await seedLongUserMessageHistory(apiClient, seedData);
      const session = new SessionPage(testPage);
      await testPage.emulateMedia({ reducedMotion: "reduce" });
      await openLongHistory(testPage, session, taskId, renderer);
      await testPage.mouse.move(0, 0);
      const rail = session.userMessageNavigationRail();
      const previous = session.previousUserMessageButton();
      const next = session.nextUserMessageButton();
      const currentPrompt = session.userMessageContaining(CURRENT_USER_PROMPT);

      await expect(rail).toHaveCSS("opacity", "0");
      await session.messageScrollOwner().hover();
      await expect(rail).toHaveCSS("opacity", "1");
      await previous.focus();
      await expect(previous).toBeFocused();
      await expect(rail).toHaveCSS("opacity", "1");

      await expect(previous).toBeEnabled();
      await expect(next).toBeDisabled();
      await expect(session.loadOlderMessagesButton()).toBeAttached();
      await expectLegacyRowArrowsAbsent(currentPrompt);

      let olderPageRequests = 0;
      const countOlderPageRequest = (request: { url(): string }) => {
        const url = new URL(request.url());
        if (
          url.pathname === `/api/v1/task-sessions/${sessionId}/messages` &&
          url.searchParams.has("before")
        ) {
          olderPageRequests++;
        }
      };
      testPage.on("request", countOlderPageRequest);

      await previous.click();
      await expect.poll(() => olderPageRequests, { timeout: 5_000 }).toBeGreaterThan(0);
      const oldPrompt = session.userMessageContaining(OLD_USER_PROMPT);
      await expect(oldPrompt).toHaveClass(/search-flash/, { timeout: 60_000 });
      await expect(oldPrompt).toHaveCSS("animation-name", "none");
      await expectMessageAtNavigationPosition(session.messageScrollOwner(), oldPrompt);
      expect(olderPageRequests).toBeGreaterThanOrEqual(3);
      await expect(previous).toBeDisabled();
      await expect(next).toBeEnabled();
      await expectLegacyRowArrowsAbsent(oldPrompt);

      const requestsBeforeDown = olderPageRequests;
      await next.click();
      await expect(currentPrompt).toHaveClass(/search-flash/, { timeout: 15_000 });
      await expectMessageAtNavigationPosition(session.messageScrollOwner(), currentPrompt);
      expect(olderPageRequests).toBe(requestsBeforeDown);
      await expect(previous).toBeEnabled();
      await expect(next).toBeDisabled();

      testPage.off("request", countOlderPageRequest);
    });
  }
});
