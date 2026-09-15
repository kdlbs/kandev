import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import {
  chooseCapture,
  dragScreenshotRegion,
  openBrowserPreview,
  saveDraft,
  selectGeneratedText,
  startPreviewServer,
} from "./preview-feedback-helpers";

test.describe("Web preview feedback", () => {
  test.describe.configure({ retries: 1, timeout: 180_000 });

  test("persists multi-route captures and sends them directly or through the queue", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      const { session, frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Web Preview Feedback",
        server.url,
      );

      await chooseCapture(testPage, "Select element");
      await frame.locator("#save").hover();
      const candidate = frame.locator('[data-kandev-inspector-ui="candidate"]');
      await expect(candidate).toBeVisible();
      await expect(candidate).toContainText("button#save.primary");
      await frame.locator("#save").click();
      await saveDraft(testPage, "Make the primary action more prominent");

      await frame.locator("#details-route").click();
      await expect(frame.locator("h1")).toHaveText("Order details");
      await chooseCapture(testPage, "Select text");
      await selectGeneratedText(frame);
      await expect(testPage.getByTestId("preview-feedback-draft")).toContainText("$42.00");
      await saveDraft(testPage, "Explain how this generated total was calculated");

      await chooseCapture(testPage, "Select screenshot region");
      await dragScreenshotRegion(testPage, frame.locator("#save"));
      const screenshotDraft = testPage.getByTestId("preview-feedback-draft");
      await expect(screenshotDraft.getByRole("img", { name: "Screenshot preview" })).toBeVisible();
      await expect(screenshotDraft).toContainText(/\d+ × \d+ · PNG/);
      await saveDraft(testPage, "Tighten the spacing in this region");

      await expect(testPage.getByTestId("preview-feedback-trigger")).toContainText("3");
      await session.clickSessionChatTab();
      await expect(session.activeChat()).toContainText("3 preview feedback items");

      await testPage.reload();
      await session.waitForLoad();
      await expect(session.activeChat()).toContainText("3 preview feedback items", {
        timeout: 15_000,
      });

      await session.sendMessageViaButton("Apply the pending preview feedback");
      const directMessage = session
        .activeChat()
        .getByTestId("user-message-bubble")
        .filter({ hasText: "Apply the pending preview feedback" });
      await expect(directMessage).toContainText("Web Preview Feedback", { timeout: 20_000 });
      await expect(directMessage).toContainText("Explain how this generated total was calculated");
      await expect(directMessage).toContainText("Rendered text anchor");
      await expect(directMessage).toContainText("node_path");
      await expect(directMessage).toContainText("viewport_width");
      const screenshotAttachment = directMessage.getByRole("button", {
        name: "Open Attachment 1",
        exact: true,
      });
      await expect(screenshotAttachment).toBeVisible();
      await screenshotAttachment.click();
      await expect(testPage.getByRole("dialog", { name: "Image preview" })).toBeVisible();
      await testPage.keyboard.press("Escape");
      await session.waitForChatIdle({ timeout: 45_000 });

      await session.sendMessage("/slow 5s");
      await waitForComposerQueueMode(testPage);
      await session.clickTab("Browser");
      await session.browserAddressInput.fill(server.url);
      await session.browserAddressInput.press("Enter");
      const queuedFrame = session.browserPanel.frameLocator("iframe");
      await expect(queuedFrame.locator("#save")).toBeVisible({ timeout: 15_000 });
      await chooseCapture(testPage, "Select element");
      await queuedFrame.locator("#save").click();
      await saveDraft(testPage, "Queue this button adjustment while the agent is busy");

      await session.clickSessionChatTab();
      const editor = session.activeChat().locator(".tiptap.ProseMirror");
      await typeWhileBusy(testPage, editor, "Apply the queued preview feedback");
      await session.submitButton().click();
      await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 15_000 });

      const queuedMessage = session
        .activeChat()
        .getByTestId("user-message-bubble")
        .filter({ hasText: "Apply the queued preview feedback" });
      await expect(queuedMessage).toContainText(
        "Queue this button adjustment while the agent is busy",
        { timeout: 45_000 },
      );
      await session.waitForChatIdle({ timeout: 45_000 });
      await expect(session.activeChat()).not.toContainText("1 preview feedback item");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { dev_script: "" });
      await server.close();
    }
  });
});
