import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  cleanupCopilotInterruptionTest,
  expectCopilotInterruptionWarningFrame,
  expectCopilotNoticeBeforeReply,
  expectFreshCopilotSessionLoad,
  seedCopilotInterruptionTask,
  waitForCopilotInterruptionACPStartup,
} from "./copilot-interrupted-tool-notice-helpers";

test.describe("Copilot interrupted tool notice", () => {
  test.setTimeout(150_000);

  test("restores the saved question, then places the interruption status before the reply", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const releaseEnvironment = await backend.useEnv({ KANDEV_E2E_MOCK_COPILOT: "true" });
    let disposeFixture: (() => Promise<void>) | undefined;
    try {
      const fixture = await seedCopilotInterruptionTask(testPage, apiClient, backend, seedData);
      disposeFixture = fixture.dispose;
      const initialACP = await waitForCopilotInterruptionACPStartup(fixture.tracePath);

      await backend.restart();
      await testPage.reload();
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expectFreshCopilotSessionLoad(
        fixture.tracePath,
        initialACP.session_id,
        initialACP.process_id!,
      );
      await expect(session.clarificationOverlay()).toBeVisible({ timeout: 45_000 });

      await session.clarificationSkip().click();
      await expect(session.clarificationOverlay()).toBeHidden();
      await session.waitForDirectInput();
      await session.sendMessage("Continue after skipping the saved question");
      await session.waitForChatIdle({ timeout: 45_000 });
      await expectCopilotInterruptionWarningFrame(fixture.tracePath, initialACP.session_id);

      const status = testPage.getByTestId("copilot-interruption-status");
      await expect(status).toBeVisible();
      await expect(status).toContainText(
        "The pending question was interrupted. You can continue the conversation.",
      );
      const detailsButton = status.getByRole("button", { name: "Details" });
      await expect(detailsButton).toHaveAttribute("aria-expanded", "false");
      const previousMessageControl = status.locator(
        'xpath=preceding::button[@aria-label="Go to previous message"][1]',
      );
      await expect(previousMessageControl).toBeVisible();
      await previousMessageControl.click();
      let reachedDetails = false;
      for (let tabCount = 0; tabCount < 5; tabCount += 1) {
        if (await detailsButton.evaluate((button) => button === document.activeElement)) {
          reachedDetails = true;
          break;
        }
        await testPage.keyboard.press("Tab");
      }
      expect(reachedDetails).toBe(true);
      await expect(detailsButton).toBeFocused();
      await testPage.keyboard.press("Enter");
      await expect(status.getByTestId("copilot-interruption-details")).toContainText(
        "Warning: 1 tool call(s) had no recorded result at resume.",
      );
      await expect(session.chat).toContainText("Here is the next step.");

      await expectCopilotNoticeBeforeReply(apiClient, fixture.sessionId);
      await expect(status).toHaveCount(1);
      const replyFollowsStatus = await session.chat.evaluate((chat) => {
        const notice = chat.querySelector('[data-testid="copilot-interruption-status"]');
        const reply = Array.from(chat.querySelectorAll('[data-agent-message-body="true"]')).find(
          (message) => message.textContent?.trim() === "Here is the next step.",
        );
        return Boolean(
          notice &&
          reply &&
          notice.compareDocumentPosition(reply) & Node.DOCUMENT_POSITION_FOLLOWING,
        );
      });
      expect(replyFollowsStatus).toBe(true);

      const originalViewport = testPage.viewportSize();
      expect(originalViewport).toBeTruthy();
      expect(await testPage.evaluate(() => window.matchMedia("(pointer: fine)").matches)).toBe(
        true,
      );
      const responsiveDetailsButton = status.getByRole("button");
      try {
        await testPage.setViewportSize({ width: 375, height: originalViewport!.height });
        await responsiveDetailsButton.scrollIntoViewIfNeeded();
        const phoneTarget = await responsiveDetailsButton.boundingBox();
        expect(phoneTarget?.height).toBeGreaterThanOrEqual(44);

        await testPage.setViewportSize({ width: 1280, height: originalViewport!.height });
        const desktopTarget = await responsiveDetailsButton.boundingBox();
        expect(desktopTarget?.height).toBe(24);
      } finally {
        await testPage.setViewportSize(originalViewport!);
      }

      await testPage.reload();
      await session.waitForLoad();
      await expect(testPage.getByTestId("copilot-interruption-status")).toHaveCount(1);
      await expect(
        testPage
          .getByTestId("copilot-interruption-status")
          .getByTestId("copilot-interruption-details"),
      ).toHaveCount(0);
      const restoredStatus = testPage.getByTestId("copilot-interruption-status");
      await expect(restoredStatus).toContainText(
        "The pending question was interrupted. You can continue the conversation.",
      );
      await restoredStatus.getByRole("button", { name: "Details" }).click();
      await expect(restoredStatus.getByTestId("copilot-interruption-details")).toContainText(
        "Warning: 1 tool call(s) had no recorded result at resume.",
      );
      await expectCopilotNoticeBeforeReply(apiClient, fixture.sessionId);
    } finally {
      await cleanupCopilotInterruptionTest(disposeFixture, releaseEnvironment);
    }
  });
});
