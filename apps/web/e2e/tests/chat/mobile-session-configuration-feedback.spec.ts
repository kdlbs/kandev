import type { Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { waitForHttp, watchWs } from "../../helpers/causal-waits";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

async function seedAndOpenChat(page: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Phone Picker Configuration Feedback",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("expected an auto-started picker feedback session");
  await waitForAgentMessage(
    apiClient,
    task.session_id,
    "This is a simple mock response for e2e testing.",
    30_000,
  );
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    timeout: 30_000,
    message: "initial phone picker feedback turn must settle before interaction",
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return { session, sessionId: task.session_id };
}

test.describe("Picker feedback on phone", () => {
  test.describe.configure({ retries: 1, timeout: 90_000 });

  test("keeps draft and reading position, and provides a touch-sized dismiss action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedAndOpenChat(testPage, apiClient, seedData);
    await session.sendMessageViaButton("/e2e:bulk:20");
    await expect(session.chat.getByText(/Done\. Emitted 20 messages/)).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle();
    const trigger = testPage
      .getByTestId("mobile-chat-toolbar-left-actions")
      .getByTestId("model-config-trigger");
    const transcript = session.activeChat().locator(".chat-message-list");
    const composer = await session.fillMessage("keep this phone draft");
    expect((await trigger.boundingBox())?.height).toBeGreaterThanOrEqual(44);

    // The shorter viewport represents the phone keyboard. Focusing the composer
    // can scroll the transcript to keep it visible, so capture the baseline
    // after that viewport change and before opening the picker.
    await testPage.setViewportSize({ width: 390, height: 420 });
    await composer.focus();
    await expect(composer).toBeFocused();
    await expect
      .poll(() => transcript.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    const transcriptBounds = await transcript.boundingBox();
    expect(transcriptBounds).not.toBeNull();
    await testPage.mouse.move(
      transcriptBounds!.x + transcriptBounds!.width / 2,
      transcriptBounds!.y + transcriptBounds!.height / 2,
    );
    const beforeReadingScroll = await transcript.evaluate((element) => element.scrollTop);
    await testPage.mouse.wheel(0, -160);
    await expect
      .poll(() => transcript.evaluate((element) => element.scrollTop))
      .toBeLessThan(beforeReadingScroll);
    const keyboardScrollTop = await transcript.evaluate((element) => element.scrollTop);

    await trigger.tap();
    const applied = waitForHttp(
      testPage,
      "POST",
      /\/api\/v1\/task-sessions\/[^/]+\/set-(?:config-option|model|mode)$/,
    );
    await testPage.getByRole("option", { name: /Mock Smart/ }).tap();
    await applied;
    const feedback = testPage.getByTestId("session-configuration-feedback");
    await expect(feedback).toBeVisible();
    await expect(feedback).toHaveAttribute("data-status", "applied");
    await expect(feedback).toContainText("mock-smart");

    await testPage.route("**/set-config-option", (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({
          error: "phone picker rejected model. " + "Provider detail remains available. ".repeat(30),
          configuration_result: {
            status: "rejected",
            config_id: "model",
            requested: "mock-fast",
            confirmed: false,
          },
        }),
      }),
    );
    const rejected = waitForHttp(testPage, "POST", /\/set-config-option$/, {
      predicate: (response) => response.status() === 500,
    });
    await testPage.getByRole("option", { name: /Mock Fast/ }).tap();
    await rejected;
    await expect(feedback).toHaveAttribute("data-status", "rejected");
    await expect(feedback).toContainText("phone picker rejected model");
    await testPage.getByTestId("model-config-sheet-close").tap();
    await expect(testPage.getByTestId("model-config-sheet-close")).toHaveCount(0);

    await expect(composer).toContainText("keep this phone draft");
    expect(await transcript.evaluate((element) => element.scrollTop)).toBe(keyboardScrollTop);

    const feedbackPanel = feedback.locator("xpath=..");
    const feedbackBounds = await feedbackPanel.boundingBox();
    expect(feedbackBounds).not.toBeNull();
    expect(feedbackBounds!.y).toBeGreaterThanOrEqual(0);
    expect(feedbackBounds!.y + feedbackBounds!.height).toBeLessThanOrEqual(420);
    const panelMetrics = await feedbackPanel.evaluate((element) => ({
      clientHeight: element.clientHeight,
      overflowY: getComputedStyle(element).overflowY,
      scrollHeight: element.scrollHeight,
    }));
    expect(panelMetrics.overflowY).toBe("auto");
    const dismiss = testPage.getByTestId("session-configuration-feedback-dismiss");
    const bounds = await dismiss.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.y).toBeGreaterThanOrEqual(0);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(420);
    expect(bounds?.width).toBeGreaterThanOrEqual(44);
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    if (panelMetrics.scrollHeight > panelMetrics.clientHeight) {
      await feedbackPanel.evaluate((element) => {
        element.scrollTop = element.scrollHeight;
      });
      await expect
        .poll(() => feedbackPanel.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(0);
      await feedbackPanel.evaluate((element) => {
        element.scrollTop = 0;
      });
    }
    await dismiss.tap();
    await expect(feedback).toBeHidden();
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
  });

  test("shows silent command feedback without replacing the draft or reading position", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const { session, sessionId } = await seedAndOpenChat(testPage, apiClient, seedData);
    await session.sendMessageViaButton("/e2e:bulk:20");
    await expect(session.chat.getByText(/Done\. Emitted 20 messages/)).toBeVisible({
      timeout: 30_000,
    });
    await session.waitForChatIdle();

    const transcript = session.activeChat().locator(".chat-message-list");
    await expect
      .poll(() => transcript.evaluate((element) => element.scrollHeight > element.clientHeight))
      .toBe(true);
    const composer = await session.composerReady();
    await testPage.clock.install();
    const completion = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await session.sendMessageViaButton("/model mock-smart");
    const command = session.chat.getByText("/model mock-smart", { exact: true });
    const completedTurn = await completion;
    expect(completedTurn.payload.had_output).toBe(false);
    expect(completedTurn.payload.configuration_result).toMatchObject({
      status: "applied",
      requested: "Mock Smart",
      effective: "Mock Smart",
      confirmed: true,
    });
    await expect(command).toBeVisible();
    await composer.fill("keep this phone draft");
    await transcript.evaluate((element) => {
      element.scrollTop = 0;
    });
    const scrollTopBeforeReceipt = await transcript.evaluate((element) => element.scrollTop);

    const receipt = session.chat
      .getByRole("status")
      .filter({ hasText: "The agent applied Mock Smart." });
    await expect(receipt).toHaveCount(1);
    await expect(receipt).toContainText("The agent applied Mock Smart.");
    await expect(session.chat).not.toContainText(/finished without producing any output/i);
    const followsCommand = await receipt.evaluate((status, commandText) => {
      const commandMessage = Array.from(
        document.querySelectorAll('[data-testid="user-message-bubble"]'),
      ).find((message) => message.textContent?.trim() === commandText);
      return (
        commandMessage !== undefined &&
        Boolean(commandMessage.compareDocumentPosition(status) & Node.DOCUMENT_POSITION_FOLLOWING)
      );
    }, "/model mock-smart");
    expect(followsCommand).toBe(true);
    await expect(composer).toBeFocused();
    await expect(composer).toContainText("keep this phone draft");
    expect(await transcript.evaluate((element) => element.scrollTop)).toBe(scrollTopBeforeReceipt);
    await testPage.clock.runFor(5_000);
    await expect(receipt).toHaveCount(0);
  });
});
