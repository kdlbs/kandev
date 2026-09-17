import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { routeMainWebSocketWithPreviewFeedbackCreateFailure } from "../../helpers/ws-drop";
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
    const createFailure = await routeMainWebSocketWithPreviewFeedbackCreateFailure(testPage);
    try {
      const { session, frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Web Preview Feedback",
        server.url,
      );

      const trigger = testPage.getByTestId("preview-feedback-trigger");
      const triggerBox = await trigger.boundingBox();
      expect(triggerBox?.height).toBeLessThanOrEqual(30);
      expect(triggerBox?.width).toBeLessThanOrEqual(48);
      await trigger.click();
      const choices = [
        { name: "Select text", label: "Text" },
        { name: "Select element", label: "Element" },
        { name: "Select screenshot region", label: "Screenshot" },
      ];
      const choiceBoxes = [];
      for (const choice of choices) {
        const button = testPage.getByRole("button", { name: choice.name, exact: true });
        await expect(button).toHaveText(choice.label);
        choiceBoxes.push(await button.boundingBox());
      }
      expect(new Set(choiceBoxes.map((box) => Math.round(box?.y ?? -1))).size).toBe(1);

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

      const failedScreenshotComment = "Keep this screenshot comment after create fails";
      createFailure.failNextCreate();
      await chooseCapture(testPage, "Select screenshot region");
      await dragScreenshotRegion(testPage, frame.locator("#save"));
      const failedDraft = testPage.getByTestId("preview-feedback-draft");
      await expect(failedDraft.getByRole("img", { name: "Screenshot preview" })).toBeVisible();
      await failedDraft
        .getByRole("textbox", { name: "Comment on selection" })
        .fill(failedScreenshotComment);
      await failedDraft.getByRole("button", { name: "Save feedback" }).click();
      await expect.poll(() => createFailure.failedCount()).toBe(1);
      await expect(failedDraft).toBeVisible();
      await expect(failedDraft.getByRole("textbox", { name: "Comment on selection" })).toHaveValue(
        failedScreenshotComment,
      );
      await expect(failedDraft.getByRole("img", { name: "Screenshot preview" })).toBeVisible();
      await expect(
        testPage.getByTestId("preview-feedback-popover").getByRole("alert"),
      ).toContainText("feedback could not be saved");

      await failedDraft.getByRole("button", { name: "Save feedback" }).click();
      await expect(failedDraft).toBeHidden({ timeout: 15_000 });
      await expect(testPage.getByTestId("preview-feedback-trigger")).toContainText("4");

      await session.clickSessionChatTab();
      const userMessageCount = await session
        .activeChat()
        .getByTestId("user-message-bubble")
        .count();
      await expect(session.activeChat()).toContainText("4 preview feedback items");

      await testPage.evaluate(() => {
        type Panel = { id: string; api: { close: () => void } };
        type DockviewApi = { panels: Panel[] };
        const api = (window as unknown as { __dockviewApi__?: DockviewApi }).__dockviewApi__;
        for (const panel of [...(api?.panels ?? [])]) {
          if (panel.id.startsWith("browser:")) panel.api.close();
        }
      });
      await expect(session.browserPanel).toHaveCount(0);

      const feedbackChip = session
        .activeChat()
        .getByRole("button", { name: "4 preview feedback items", exact: true });
      await feedbackChip.click();
      const collection = testPage.getByTestId("preview-feedback-collection-popover");
      await expect(collection).toBeVisible();
      const collectionItems = collection.getByTestId("preview-feedback-item");
      await expect(collectionItems).toHaveCount(4);
      await expect(
        collectionItems.nth(2).getByRole("img", { name: "Screenshot preview" }),
      ).toBeVisible();

      const firstItem = collectionItems.nth(0);
      await firstItem.getByRole("button", { name: "Edit feedback" }).click();
      await firstItem
        .getByRole("textbox", { name: "Edit comment" })
        .fill("Make the action clearer");
      await firstItem.getByRole("button", { name: "Save changes" }).click();
      await expect(firstItem).toContainText("Make the action clearer");

      await collectionItems.nth(1).getByRole("button", { name: "Delete feedback" }).click();
      await expect(collectionItems).toHaveCount(3);
      await expect(collection).toContainText(failedScreenshotComment);
      await expect(collection).not.toContainText("Explain how this generated total was calculated");
      expect(await session.activeChat().getByTestId("user-message-bubble").count()).toBe(
        userMessageCount,
      );
      await testPage.keyboard.press("Escape");

      await testPage.reload();
      await session.waitForLoad();
      await expect(session.activeChat()).toContainText("3 preview feedback items", {
        timeout: 15_000,
      });
      const restoredFeedbackChip = session
        .activeChat()
        .getByRole("button", { name: "3 preview feedback items", exact: true });
      await restoredFeedbackChip.click();
      await expect(collection).toBeVisible();
      await expect(collection).toContainText("Make the action clearer");
      await expect(collection).not.toContainText("Explain how this generated total was calculated");
      await testPage.keyboard.press("Escape");

      await session.sendMessageViaButton("Apply the pending preview feedback");
      const directMessage = session
        .activeChat()
        .getByTestId("user-message-bubble")
        .filter({ hasText: "Apply the pending preview feedback" });
      await expect(directMessage).toContainText("Web Preview Feedback", { timeout: 20_000 });
      await expect(directMessage).toContainText("Make the action clearer");
      await expect(directMessage).not.toContainText(
        "Explain how this generated total was calculated",
      );
      await expect(directMessage).toContainText("Rendered element snapshot");
      await expect(directMessage).toContainText('"selector": "button#save"');
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
      await session.addBrowserPanel();
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
