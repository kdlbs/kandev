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

test.describe("Conversation fork into a new agent on phone", () => {
  test("keeps the preview touch-accessible and launches the frozen range", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(180_000);
    const source = await seedConversationForkSource(
      apiClient,
      seedData,
      "Mobile conversation fork source",
    );
    await testPage.goto(`/t/${source.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    const cutoff = session
      .activeChat()
      .getByTestId("agent-message-highlight")
      .filter({ hasText: FORK_SOURCE_ASSISTANT });
    await expect(cutoff).toBeVisible();
    const forkAction = cutoff.getByTestId("conversation-fork-message-action");
    const actionBox = await forkAction.boundingBox();
    expect(actionBox?.height).toBeGreaterThanOrEqual(44);
    await forkAction.tap();

    const picker = testPage.getByTestId("conversation-fork-picker");
    await expect(picker).toBeVisible();
    await picker.getByTestId("conversation-fork-destination-agent").tap();
    await picker.getByRole("button", { name: "Continue" }).tap();

    const launchDialog = session.sessionLaunchDialog();
    await expect(launchDialog).toBeVisible({ timeout: 10_000 });
    const previewSurface = testPage.getByTestId("new-session-drawer");
    await expect
      .poll(async () => (await launchDialog.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(viewport!.height - 4);
    const launchBox = await launchDialog.boundingBox();
    expect(launchBox).not.toBeNull();
    expect(launchBox!.height).toBeGreaterThanOrEqual(viewport!.height - 4);
    expect(launchBox!.x).toBeGreaterThanOrEqual(0);
    expect(launchBox!.x + launchBox!.width).toBeLessThanOrEqual(viewport!.width);
    if (prCapture.capturing) {
      await previewSurface.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      await prCapture.screenshot("phone-conversation-fork-form");
    }

    const chip = launchDialog.getByTestId("conversation-fork-chip");
    await expect(chip.getByRole("button", { name: "Preview" })).toBeVisible();
    const previewButtonBox = await chip.getByRole("button", { name: "Preview" }).boundingBox();
    expect(previewButtonBox?.height).toBeGreaterThanOrEqual(44);
    await chip.getByRole("button", { name: "Preview" }).tap();

    const preview = testPage.getByTestId("conversation-fork-preview");
    await expect(preview).toBeVisible();
    await expect(preview.getByTestId("conversation-fork-content")).toContainText(FORK_SOURCE_USER);
    const scrollOwners = await preview.locator(".overflow-y-auto").count();
    expect(scrollOwners).toBe(1);
    await expect
      .poll(async () => (await previewSurface.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(viewport!.height - 4);
    const previewBounds = await preview.boundingBox();
    expect(previewBounds).not.toBeNull();
    const previewSurfaceBounds = await previewSurface.boundingBox();
    expect(previewSurfaceBounds).not.toBeNull();
    expect(previewSurfaceBounds!.height).toBeGreaterThanOrEqual(viewport!.height - 4);
    expect(previewBounds!.height).toBeGreaterThan(200);
    if (prCapture.capturing) {
      await previewSurface.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      await prCapture.screenshot("phone-conversation-fork-preview");
    }
    await testInfo.attach("mobile-conversation-fork-preview.png", {
      body: await preview.screenshot(),
      contentType: "image/png",
    });

    const range = preview.getByRole("combobox", { name: "Start from" });
    expect((await range.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await range.selectOption(source.startMessageId);
    const evidence = preview.getByRole("checkbox", { name: "Include selected tool evidence" });
    const evidenceLabel = preview
      .locator("label")
      .filter({ hasText: "Include selected tool evidence" });
    expect((await evidenceLabel.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await evidence.check();
    const applyButton = preview.getByRole("button", { name: "Apply selection" });
    expect((await applyButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await applyButton.tap();
    await expect(chip).toBeVisible();
    await chip.getByRole("button", { name: "Preview" }).tap();
    const compiledPreview = await preview.getByTestId("conversation-fork-content").innerText();
    expect(compiledPreview).toContain(FORK_SOURCE_USER);
    expect(compiledPreview).toContain(FORK_SOURCE_TOOL);
    expect(compiledPreview).toContain(FORK_SOURCE_ASSISTANT);
    expect(compiledPreview).not.toContain(FORK_SOURCE_AFTER_CUTOFF);

    await preview.getByRole("button", { name: "Back" }).tap();
    const prompt = session.newSessionPromptInput();
    await prompt.fill(FORK_NEW_INSTRUCTION);
    const startButton = session.newSessionStartButton();
    expect((await startButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await startButton.tap();
    await expect(launchDialog).not.toBeVisible({ timeout: 15_000 });

    await expect
      .poll(async () => (await apiClient.listTaskSessions(source.taskId)).sessions.length)
      .toBe(2);
    const { sessions } = await apiClient.listTaskSessions(source.taskId);
    const destinationSession = sessions.find((candidate) => candidate.id !== source.sessionId);
    expect(destinationSession).toBeTruthy();
    await waitForSessionDone(
      apiClient,
      source.taskId,
      destinationSession!.id,
      "Waiting for phone forked agent",
    );
    const { messages } = await apiClient.listSessionMessages(destinationSession!.id);
    const firstUserMessage = messages.find((message) => message.author_type === "user");
    expect(firstUserMessage?.content).toContain(FORK_NEW_INSTRUCTION);
    const forkId = firstUserMessage?.metadata?.conversation_fork_id;
    expect(typeof forkId).toBe("string");
    const frozen = await apiClient.getConversationForkContent(forkId as string);
    expect(frozen.content).toBe(compiledPreview);
    expect(frozen.content).not.toContain(FORK_SOURCE_AFTER_CUTOFF);
  });
});
