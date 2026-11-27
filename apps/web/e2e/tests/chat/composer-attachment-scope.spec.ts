import { expect, test } from "../../fixtures/office-fixture";
import { SessionPage } from "../../pages/session-page";
import {
  createAssignedOfficeTask,
  expectUploadScope,
  openOfficeTaskChat,
  pastePng,
  waitForReadyDraftAttachment,
  waitForUserImageAttachment,
} from "./composer-attachment-scope-helpers";

test.describe("task chat attachment workspace scope", () => {
  test("uploads a pasted image to an Office task workspace from a cold advanced route", async ({
    testPage,
    prCapture,
    apiClient,
    officeApi,
    officeSeed,
    seedData,
  }) => {
    test.setTimeout(90_000);
    expect(seedData.workspaceId).not.toBe(officeSeed.workspaceId);
    const task = await createAssignedOfficeTask(
      apiClient,
      officeApi,
      officeSeed,
      "Office composer attachment scope",
    );
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
    });

    const sessionPage = new SessionPage(testPage);
    const { chat, editor } = await openOfficeTaskChat(testPage, task.taskId);
    await sessionPage.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    const uploadResponse = pastePng(testPage, editor, "office-pasted.png");
    const response = await uploadResponse;
    expect(response.ok()).toBe(true);
    await expectUploadScope(testPage, officeSeed.workspaceId);
    await expect(chat.getByText(/Image \(/)).toBeVisible();
    await waitForReadyDraftAttachment(testPage, task.sessionId, "office-pasted.png");
    await prCapture.screenshot("office-attachment-ready", {
      caption: "Office task chat with an uploaded image ready to send",
    });

    const marker = "deliver the Office image attachment";
    await sessionPage.sendMessageViaButton(marker);
    const attachment = await waitForUserImageAttachment(
      apiClient,
      task.sessionId,
      marker,
      "office-pasted.png",
    );
    expect(attachment).toMatchObject({
      type: "image",
      name: "office-pasted.png",
      mime_type: "image/png",
    });
    expect(attachment?.attachment_id).toBeTruthy();

    await expect(editor).toBeEmpty({ timeout: 15_000 });
    await expect(chat.getByText(/Image \(/)).toHaveCount(0, { timeout: 15_000 });
    await testPage.reload();
    const reloadedChat = testPage.getByTestId("session-chat");
    await expect(reloadedChat).toBeVisible({ timeout: 30_000 });
    const sentMessage = reloadedChat.getByTestId("user-message-bubble").filter({ hasText: marker });
    await expect(sentMessage).toHaveCount(1);
    const attachmentButton = sentMessage.getByRole("button", {
      name: "Open Attachment 1",
      exact: true,
    });
    await expect(attachmentButton).toHaveCount(1);
    const image = attachmentButton.locator(
      `img[src*="/api/v1/attachments/${String(attachment?.attachment_id)}/content"]`,
    );
    await expect(image).toHaveCount(1);
    await expect(image).toBeVisible();
    await expect
      .poll(() => image.evaluate((element) => (element as HTMLImageElement).naturalWidth))
      .toBeGreaterThan(0);
  });
});
