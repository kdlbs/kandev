import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/office-fixture";
import { SessionPage } from "../../pages/session-page";
import {
  createAssignedOfficeTask,
  expectUploadScope,
  openOfficeTaskChat,
  pastePng,
  waitForReadyDraftAttachment,
  waitForUserImageAttachment,
  writePngFile,
} from "./composer-attachment-scope-helpers";

async function expectTouchTarget(locator: import("@playwright/test").Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  if (!box) return;
  expect(box.width).toBeGreaterThanOrEqual(44);
  expect(box.height).toBeGreaterThanOrEqual(44);
}

test.describe("mobile task chat attachment workspace scope", () => {
  test("retries and sends real image files in the Office task workspace", async ({
    testPage,
    prCapture,
    apiClient,
    officeApi,
    officeSeed,
  }, testInfo) => {
    test.setTimeout(90_000);
    const task = await createAssignedOfficeTask(
      apiClient,
      officeApi,
      officeSeed,
      "Mobile Office composer attachment scope",
    );
    let uploadAttempts = 0;
    await testPage.route("**/api/v1/attachments", async (route) => {
      if (route.request().method() === "POST" && uploadAttempts++ === 0) {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Temporary upload error" }),
        });
        return;
      }
      await route.continue();
    });

    const sessionPage = new SessionPage(testPage);
    const { chat, editor } = await openOfficeTaskChat(testPage, task.taskId);
    await sessionPage.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    const failedUploadPromise = pastePng(testPage, editor, "mobile-paste.png");
    const failedResponse = await failedUploadPromise;
    expect(failedResponse.ok()).toBe(false);
    await expectUploadScope(testPage, officeSeed.workspaceId);
    await expect(chat.getByText(/Image \(/)).toBeVisible();

    await chat
      .getByText(/Image \(/)
      .first()
      .tap();
    const retry = testPage.getByTestId("attachment-upload-retry");
    await expect(retry).toBeVisible();
    await expectTouchTarget(retry);
    await retry.tap();
    await expect.poll(async () => uploadAttempts).toBe(2);
    await waitForReadyDraftAttachment(testPage, task.sessionId, "mobile-paste.png");

    await testPage.keyboard.press("Escape");
    const remove = chat.getByTestId("context-chip-remove").first();
    await expect(remove).toBeVisible();
    await expectTouchTarget(remove);
    await remove.tap();
    await expect(chat.getByText(/Image \(/)).toHaveCount(0);

    const pickerPath = testInfo.outputPath("mobile-picker.png");
    await fs.promises.mkdir(path.dirname(pickerPath), { recursive: true });
    await writePngFile(pickerPath);
    const fileInput = chat.locator('input[type="file"]').first();
    await fileInput.setInputFiles(pickerPath);
    await expect(chat.getByText(/Image \(/)).toBeVisible();
    await waitForReadyDraftAttachment(testPage, task.sessionId, "mobile-picker.png");
    await prCapture.screenshot("mobile-office-attachment-ready", {
      caption: "Mobile Office task chat with an uploaded image ready to send",
    });

    const marker = "send the mobile Office image";
    await sessionPage.sendMessageViaButton(marker);
    const attachment = await waitForUserImageAttachment(
      apiClient,
      task.sessionId,
      marker,
      "mobile-picker.png",
    );
    expect(attachment).toMatchObject({ type: "image", name: "mobile-picker.png" });
    expect(attachment?.attachment_id).toBeTruthy();

    await testPage.reload();
    const reloadedChat = testPage.getByTestId("session-chat");
    await expect(reloadedChat).toBeVisible({ timeout: 30_000 });
    const image = reloadedChat.locator(
      `img[src*="/api/v1/attachments/${String(attachment?.attachment_id)}/content"]`,
    );
    await expect(image).toBeVisible();
    await expect
      .poll(() => image.evaluate((element) => (element as HTMLImageElement).naturalWidth))
      .toBeGreaterThan(0);

    const chatInput = reloadedChat.getByTestId("chat-input-editor-shell");
    await expectTouchTarget(chatInput);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
