import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  CURRENT_USER_PROMPT,
  OLD_USER_PROMPT,
  expectMessageAtNavigationPosition,
  expectNavigationOutline,
  openLongHistory,
  seedLongUserMessageHistory,
} from "./user-message-navigation-helpers";

test.describe("@chat user message navigation", () => {
  for (const renderer of ["native", "virtuoso"] as const) {
    test(`reveals and navigates paginated history with the ${renderer} renderer`, async ({
      testPage,
      apiClient,
      seedData,
      prCapture,
    }) => {
      test.setTimeout(180_000);
      const { taskId, sessionId } = await seedLongUserMessageHistory(apiClient, seedData);
      const session = new SessionPage(testPage);
      await testPage.emulateMedia({ reducedMotion: "reduce" });
      await openLongHistory(testPage, session, taskId, renderer);
      await testPage.mouse.move(0, 0);
      const currentPrompt = session.userMessageContaining(CURRENT_USER_PROMPT);
      const currentActions = currentPrompt.getByTestId("message-actions");
      const previous = session.previousUserMessageButton(currentPrompt);
      const next = session.nextUserMessageButton(currentPrompt);

      await expect(session.activeChat().getByTestId("user-message-navigation-rail")).toHaveCount(0);
      await expect(currentActions).toHaveCSS("opacity", "0");
      await currentPrompt.getByText(CURRENT_USER_PROMPT, { exact: true }).hover();
      await expect(currentActions).toHaveCSS("opacity", "1");
      await previous.focus();
      await expect(previous).toBeFocused();
      await expect(currentActions).toHaveCSS("opacity", "1");

      await expect(previous).toBeEnabled();
      await expect(next).toBeDisabled();
      await expect(session.loadOlderMessagesButton()).toBeVisible();
      await expect(session.loadOlderMessagesButton()).toBeEnabled();

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
      await expectNavigationOutline(oldPrompt);
      await expectMessageAtNavigationPosition(session.messageScrollOwner(), oldPrompt);
      expect(olderPageRequests).toBeGreaterThanOrEqual(3);
      const oldPrevious = session.previousUserMessageButton(oldPrompt);
      const oldNext = session.nextUserMessageButton(oldPrompt);
      await expect(oldPrevious).toBeDisabled();
      await expect(oldNext).toBeEnabled();
      await oldPrompt.getByText(OLD_USER_PROMPT, { exact: true }).hover();
      await expect(oldPrompt.getByTestId("message-actions")).toHaveCSS("opacity", "1");
      await prCapture.screenshot(`inline-user-message-navigation-${renderer}`, {
        caption: `Inline user-message navigation using the ${renderer} renderer`,
      });

      const requestsBeforeDown = olderPageRequests;
      await oldNext.click();
      await expect(currentPrompt).toHaveClass(/search-flash/, { timeout: 15_000 });
      await expectNavigationOutline(currentPrompt);
      await expectMessageAtNavigationPosition(session.messageScrollOwner(), currentPrompt);
      expect(olderPageRequests).toBe(requestsBeforeDown);
      await expect(previous).toBeEnabled();
      await expect(next).toBeDisabled();

      testPage.off("request", countOlderPageRequest);
    });
  }
});
