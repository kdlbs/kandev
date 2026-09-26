import { test, expect } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import {
  FORK_NEW_INSTRUCTION,
  FORK_SOURCE_AFTER_CUTOFF,
  FORK_SOURCE_ASSISTANT,
  FORK_SOURCE_TOOL,
  FORK_SOURCE_USER,
  seedConversationForkSource,
} from "./conversation-fork-helpers";

test.describe("Conversation fork into a new agent", () => {
  test("delivers the selected frozen history to a new agent session", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(180_000);
    const source = await seedConversationForkSource(
      apiClient,
      seedData,
      "Conversation fork source",
    );
    await testPage.goto(`/t/${source.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const cutoff = session
      .activeChat()
      .getByTestId("agent-message-highlight")
      .filter({ hasText: FORK_SOURCE_ASSISTANT });
    await expect(cutoff).toBeVisible();
    const forkAction = cutoff.getByTestId("conversation-fork-message-action");
    await forkAction.hover();
    await forkAction.click();

    const picker = testPage.getByTestId("conversation-fork-picker");
    await expect(picker).toBeVisible();
    const agentDestination = picker.getByTestId("conversation-fork-destination-agent");
    await expect(agentDestination).toHaveAttribute("aria-pressed", "false");
    await agentDestination.click();
    await picker.getByRole("button", { name: "Continue" }).click();

    await expect(session.newSessionDialog()).toBeVisible({ timeout: 10_000 });
    const launchDialog = session.sessionLaunchDialog();
    const chip = launchDialog.getByTestId("conversation-fork-chip");
    await expect(chip).toContainText("Conversation fork source");
    await expect(chip).toContainText("Full conversation");
    if (prCapture.capturing) {
      await launchDialog.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      await prCapture.screenshot("desktop-conversation-fork-form");
    }

    expect(await testPage.evaluate(() => matchMedia("(pointer:fine)").matches)).toBe(true);
    const previewButton = chip.getByRole("button", { name: "Preview" });
    const previewButtonBox = await previewButton.boundingBox();
    expect(previewButtonBox?.height).toBeGreaterThanOrEqual(27);
    expect(previewButtonBox?.height).toBeLessThanOrEqual(32);
    await previewButton.focus();
    await expect(chip.getByTestId("conversation-fork-chip-history-preview")).toBeVisible();
    await previewButton.click();
    let preview = testPage.getByTestId("conversation-fork-preview");
    await expect(preview).toBeVisible();
    const rangeBox = await preview.getByRole("combobox", { name: "Start from" }).boundingBox();
    expect(rangeBox?.height).toBeGreaterThanOrEqual(27);
    expect(rangeBox?.height).toBeLessThanOrEqual(32);
    await expect(preview.getByTestId("conversation-fork-content")).toContainText(FORK_SOURCE_USER);
    await expect(preview.getByTestId("conversation-fork-content")).toContainText(
      FORK_SOURCE_ASSISTANT,
    );
    await expect(preview.getByTestId("conversation-fork-content")).not.toContainText(
      FORK_SOURCE_AFTER_CUTOFF,
    );
    if (prCapture.capturing) {
      await preview.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      await prCapture.screenshot("desktop-conversation-fork-preview");
    }
    await testInfo.attach("conversation-fork-preview.png", {
      body: await preview.screenshot(),
      contentType: "image/png",
    });

    await preview.getByRole("combobox", { name: "Start from" }).selectOption(source.startMessageId);
    await preview.getByRole("checkbox", { name: "Include selected tool evidence" }).check();
    await preview.getByRole("button", { name: "Apply selection" }).click();
    await expect(chip).toBeVisible();

    await chip.getByRole("button", { name: "Preview" }).click();
    preview = testPage.getByTestId("conversation-fork-preview");
    const compiledPreview = await preview.getByTestId("conversation-fork-content").innerText();
    expect(compiledPreview).toContain(FORK_SOURCE_USER);
    expect(compiledPreview).toContain(FORK_SOURCE_TOOL);
    expect(compiledPreview).toContain(FORK_SOURCE_ASSISTANT);
    expect(compiledPreview).not.toContain(FORK_SOURCE_AFTER_CUTOFF);
    expect(compiledPreview).not.toContain("/e2e:simple-message");
    await preview.getByRole("button", { name: "Back" }).click();
    await expect(previewButton).toBeFocused();

    const prompt = session.newSessionPromptInput();
    await prompt.fill(FORK_NEW_INSTRUCTION);
    const startButton = session.newSessionStartButton();
    await expect(startButton).toBeEnabled();
    await startButton.click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 15_000 });

    await expect
      .poll(async () => (await apiClient.listTaskSessions(source.taskId)).sessions.length)
      .toBe(2);
    const { sessions: launchedSessions } = await apiClient.listTaskSessions(source.taskId);
    const destinationSession = launchedSessions.find(
      (candidate) => candidate.id !== source.sessionId,
    );
    expect(destinationSession).toBeTruthy();
    await waitForSessionDone(
      apiClient,
      source.taskId,
      destinationSession!.id,
      "Waiting for forked agent turn",
    );

    const { messages } = await apiClient.listSessionMessages(destinationSession!.id);
    const firstUserMessage = messages.find((message) => message.author_type === "user");
    expect(firstUserMessage?.content).toContain(FORK_NEW_INSTRUCTION);
    const forkId = firstUserMessage?.metadata?.conversation_fork_id;
    expect(typeof forkId).toBe("string");
    const frozen = await apiClient.getConversationForkContent(forkId as string);
    expect(frozen.content).toBe(compiledPreview);
    expect(frozen.content).toContain(FORK_SOURCE_TOOL);
    expect(frozen.content).not.toContain(FORK_SOURCE_AFTER_CUTOFF);

    await expect(
      session.activeChat().getByTestId("conversation-fork-provenance-trigger"),
    ).toBeVisible();
    await session.activeChat().getByTestId("conversation-fork-provenance-trigger").click();
    await expect(testPage.getByTestId("conversation-fork-provenance-content")).toContainText(
      FORK_SOURCE_TOOL,
    );
  });
});
