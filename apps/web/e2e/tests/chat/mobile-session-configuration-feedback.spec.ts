import { test, expect } from "../../fixtures/test-base";
import { waitForHttp, watchWs } from "../../helpers/causal-waits";
import {
  holdConfigurationCompletions,
  seedAndOpenChat,
} from "./session-configuration-feedback-helpers";

test.describe("Picker feedback on phone", () => {
  test.describe.configure({ retries: 1, timeout: 90_000 });

  test("keeps draft and reading position, and provides a touch-sized dismiss action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedAndOpenChat(
      testPage,
      apiClient,
      seedData,
      "Phone Picker Configuration Feedback",
    );
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

    await testPage.getByTestId("session-mode-selector").tap();
    const modeResponse = waitForHttp(testPage, "POST", /\/set-mode$/);
    await testPage.getByTestId("session-mode-option-plan-mock").tap();
    await modeResponse;
    const feedback = transcript.getByTestId("session-configuration-feedback");
    await expect(feedback).toContainText("Plan Mock");

    await trigger.tap();
    const applied = waitForHttp(
      testPage,
      "POST",
      /\/api\/v1\/task-sessions\/[^/]+\/set-(?:config-option|model|mode)$/,
    );
    await testPage.getByRole("option", { name: /Mock Smart/ }).tap();
    await applied;
    await expect(feedback).toHaveCount(1);
    await expect(feedback).toHaveClass(/text-muted-foreground/);
    await expect(feedback).toHaveText("Changed mode Plan Mock, model Mock Smart.");
    await expect(feedback.locator("code")).toHaveText(["Plan Mock", "Mock Smart"]);

    await testPage.route("**/set-config-option", (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({
          error: "Agent reply timed out. " + "Provider detail remains available. ".repeat(30),
          configuration_result: {
            status: "unconfirmed",
            config_id: "model",
            requested: "mock-fast",
            confirmed: false,
          },
        }),
      }),
    );
    const unconfirmedResponse = waitForHttp(testPage, "POST", /\/set-config-option$/, {
      predicate: (response) => response.status() === 500,
    });
    await testPage.getByRole("option", { name: /Mock Fast/ }).tap();
    await unconfirmedResponse;
    await expect(feedback).toHaveCount(1);
    await expect(feedback).toContainText("Changed mode Plan Mock.");
    await expect(feedback).not.toHaveClass(/text-destructive/);
    await expect(feedback.locator(".text-destructive")).toContainText(
      "Could not confirm model Mock Fast. Agent reply timed out.",
    );
    await expect(feedback.locator("code")).toHaveText(["Plan Mock", "Mock Fast"]);
    await testPage.getByTestId("model-config-sheet-close").tap();
    await expect(testPage.getByTestId("model-config-sheet-close")).toHaveCount(0);

    await expect(composer).toContainText("keep this phone draft");
    expect(await transcript.evaluate((element) => element.scrollTop)).toBe(keyboardScrollTop);

    const hasHorizontalOverflow = await feedback.evaluate(
      (element) => element.scrollWidth > element.clientWidth,
    );
    expect(hasHorizontalOverflow).toBe(false);
    const dismiss = transcript.getByTestId("session-configuration-feedback-dismiss");
    await dismiss.scrollIntoViewIfNeeded();
    const bounds = await dismiss.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.y).toBeGreaterThanOrEqual(0);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(420);
    expect(bounds?.width).toBeGreaterThanOrEqual(44);
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
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
    const completionHold = await holdConfigurationCompletions(testPage);
    let sessionSubscriptionsReady: Promise<unknown[]> | undefined;
    const { session, sessionId } = await seedAndOpenChat(
      testPage,
      apiClient,
      seedData,
      "Phone Picker Command Feedback",
      (nextSessionId) => {
        sessionSubscriptionsReady = Promise.all([
          ws.waitForResponse("session.subscribe", {
            requestWhere: (payload) => payload.session_id === nextSessionId,
          }),
          ws.waitForResponse("session.conversation.subscribe", {
            requestWhere: (payload) => payload.session_id === nextSessionId,
          }),
        ]);
      },
    );
    if (!sessionSubscriptionsReady) throw new Error("Session subscription waits were not armed");
    await sessionSubscriptionsReady;
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
    completionHold.arm(sessionId);
    const completion = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await session.sendMessageViaButton("/model mock-smart");
    const command = session.chat.getByText("/model mock-smart", { exact: true });
    await expect(command).toBeVisible();
    await composer.fill("keep this phone draft");
    const autoScroll = session.chat.getByTestId("auto-scroll-toggle-button");
    await expect(async () => {
      if ((await autoScroll.getAttribute("aria-pressed")) === "true") {
        await autoScroll.tap({ timeout: 2_000 });
      }
      expect(await autoScroll.getAttribute("aria-pressed")).toBe("false");
    }).toPass({ timeout: 5_000 });
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
    await composer.fill("keep this phone draft");
    const scrollTopBeforeReceipt = await transcript.evaluate((element) => element.scrollTop);
    expect(scrollTopBeforeReceipt).toBeGreaterThan(0);
    await expect.poll(() => completionHold.pendingCount()).toBeGreaterThan(0);
    completionHold.release();
    const completedTurn = await completion;
    expect(completedTurn.payload.had_output).toBe(false);
    expect(completedTurn.payload.configuration_result).toMatchObject({
      status: "applied",
      requested: "Mock Smart",
      effective: "Mock Smart",
      confirmed: true,
    });
    await expect(command).toBeVisible();

    const receipt = session.chat
      .getByRole("status")
      .filter({ hasText: "The agent applied Mock Smart." });
    await expect(receipt).toHaveCount(1);
    await expect(receipt).toHaveClass(/text-muted-foreground/);
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
