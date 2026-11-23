import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import {
  createDisposableCopilotProfile,
  enableCopilotNativeBackend,
  startCopilotContextTask,
  waitForCopilotContextInfoMessages,
} from "./copilot-context-tier-helpers";

test.describe("Mobile Copilot context window selection", () => {
  test.setTimeout(120_000);

  test("switches Copilot context window by touch and reloads the session without overflow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const release = await enableCopilotNativeBackend(backend);
    let profile: Awaited<ReturnType<typeof createDisposableCopilotProfile>> | null = null;
    try {
      profile = await createDisposableCopilotProfile(apiClient);
      const { taskId, sessionId, session } = await startCopilotContextTask(
        testPage,
        apiClient,
        seedData,
        profile.profileId,
      );

      const [initialInfo] = await waitForCopilotContextInfoMessages(apiClient, sessionId, 1);
      expect(initialInfo).toMatchObject({
        contextTier: "default",
        loads: 0,
        priorTurns: 0,
      });

      const viewport = testPage.viewportSize();
      expect(viewport).not.toBeNull();

      const trigger = testPage
        .getByTestId("mobile-chat-toolbar-left-actions")
        .getByTestId("model-config-trigger");
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      const triggerBox = await trigger.boundingBox();
      expect(triggerBox).not.toBeNull();
      expect(triggerBox!.height).toBeGreaterThanOrEqual(44);

      await trigger.tap();
      const sheetScroll = testPage.getByTestId("model-config-sheet-scroll");
      await expect(sheetScroll).toBeVisible({ timeout: 10_000 });

      const contextTrigger = sheetScroll.getByTestId("config-option-trigger-context_tier");
      await expect(contextTrigger).toBeVisible({ timeout: 10_000 });
      const contextTriggerBox = await contextTrigger.boundingBox();
      expect(contextTriggerBox).not.toBeNull();
      expect(contextTriggerBox!.x).toBeGreaterThanOrEqual(0);
      expect(contextTriggerBox!.x + contextTriggerBox!.width).toBeLessThanOrEqual(
        viewport!.width + 1,
      );

      await contextTrigger.tap();
      const contextSection = sheetScroll.getByTestId("config-option-section-context_tier");
      await expect(contextSection).toBeVisible();

      const longContextOption = contextSection.getByRole("button", {
        name: "Long Context",
        exact: true,
      });
      await expect(longContextOption).toBeVisible();
      const optionBox = await longContextOption.boundingBox();
      expect(optionBox).not.toBeNull();
      expect(optionBox!.x).toBeGreaterThanOrEqual(0);
      expect(optionBox!.x + optionBox!.width).toBeLessThanOrEqual(viewport!.width + 1);

      const sectionOverflowY = await contextSection.evaluate(
        (element) => getComputedStyle(element).overflowY,
      );
      expect(["auto", "scroll"]).toContain(sectionOverflowY);

      const hasHorizontalOverflow = await testPage.evaluate(() => {
        const root = document.scrollingElement ?? document.documentElement;
        return root.scrollWidth > root.clientWidth + 1;
      });
      expect(hasHorizontalOverflow).toBe(false);

      await longContextOption.tap();
      await expect(trigger).toContainText("Long Context", { timeout: 15_000 });
      await expect
        .poll(async () => {
          const { sessions } = await apiClient.listTaskSessionDetails(taskId);
          const runtime = sessions.find((item) => item.id === sessionId)?.metadata
            ?.runtime_config as { config_options?: Record<string, string> } | undefined;
          return runtime?.config_options?.context_tier;
        })
        .toBe("long_context");

      const closeSheet = testPage.getByTestId("model-config-sheet-close");
      await expect(closeSheet).toBeVisible();
      await closeSheet.tap();
      await expect(sheetScroll).toHaveCount(0);
      await waitForSessionState(apiClient, {
        taskId,
        sessionId,
        expectedState: "WAITING_FOR_INPUT",
        timeout: 30_000,
        message: "Phone Copilot session must settle back to WAITING_FOR_INPUT after reload",
      });
      await session.waitForChatIdle({ timeout: 30_000 });

      await session.sendMessageViaButton("/e2e:copilot-context-info mobile-second-turn");
      const turns = await waitForCopilotContextInfoMessages(apiClient, sessionId, 2);
      await waitForSessionState(apiClient, {
        taskId,
        sessionId,
        expectedState: "WAITING_FOR_INPUT",
        timeout: 30_000,
        message: "Reloaded phone Copilot session must complete the follow-up turn",
      });
      await session.waitForChatIdle({ timeout: 30_000 });

      expect(turns[1].contextTier).toBe("long_context");
      expect(turns[1].sessionId).toBe(initialInfo.sessionId);
      expect(turns[1].pid).not.toBe(initialInfo.pid);
      expect(turns[1].loads).toBe(1);
      expect(turns[1].priorTurns).toBe(1);

      await expect(session.chat).toContainText("Copilot runtime context_tier=default");
      await expect(session.chat).toContainText("Copilot runtime context_tier=long_context");

      const postReloadOverflow = await testPage.evaluate(() => {
        const root = document.scrollingElement ?? document.documentElement;
        return root.scrollWidth > root.clientWidth + 1;
      });
      expect(postReloadOverflow).toBe(false);
    } finally {
      if (profile) await profile.dispose();
      await release();
    }
  });
});
