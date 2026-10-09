import fs from "node:fs";
import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { OfficeApiClient } from "../../helpers/office-api-client";
import { waitForOfficeTaskSessionLive } from "../../helpers/office-launch";

const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/6X3pZQAAAAASUVORK5CYII=";

export async function createAssignedOfficeTask(
  apiClient: ApiClient,
  officeApi: OfficeApiClient,
  officeSeed: { workspaceId: string; agentId: string; workflowId: string },
  title: string,
) {
  const task = await officeApi.createTask(officeSeed.workspaceId, title, {
    description: "/e2e:simple-message",
    workflow_id: officeSeed.workflowId,
  });
  const taskId = task.id as string | undefined;
  if (!taskId) throw new Error("Office task creation returned no task ID");

  await officeApi.assignTask(taskId, officeSeed.agentId);
  await waitForOfficeTaskSessionLive(apiClient, taskId);

  let sessionId = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        sessionId = sessions[0]?.id ?? "";
        return sessionId;
      },
      { timeout: 15_000, message: "Wait for the assigned Office task session" },
    )
    .not.toBe("");

  return { taskId, sessionId, workspaceId: officeSeed.workspaceId };
}

export async function openOfficeTaskChat(page: Page, taskId: string) {
  // Read the browser-created multipart field because Playwright exposes no raw form body.
  await page.addInitScript(() => {
    const originalFetch = window.fetch;
    window.fetch = (input, init) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/api/v1/attachments") && init?.body instanceof FormData) {
        window.sessionStorage.setItem(
          "e2e.attachment-upload-workspace-id",
          String(init.body.get("workspace_id") ?? ""),
        );
      }
      return originalFetch(input, init);
    };
  });
  await page.goto(`/office/tasks/${taskId}?mode=advanced`);
  const chat = page.getByTestId("session-chat");
  await expect(chat).toBeVisible({ timeout: 30_000 });
  const editor = chat.getByTestId("chat-input-editor-shell").locator(".tiptap.ProseMirror");
  await expect(editor).toBeVisible();
  await expect(editor).toBeEditable();
  return { chat, editor };
}

export async function pastePng(page: Page, editor: Locator, name: string) {
  const uploadResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    { timeout: 15_000 },
  );
  await editor.evaluate(
    (element, { fileName, bytesBase64 }) => {
      const bytes = Uint8Array.from(atob(bytesBase64), (character) => character.charCodeAt(0));
      const file = new File([bytes], fileName, { type: "image/png" });
      const clipboardData = new DataTransfer();
      clipboardData.items.add(file);
      const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
      element.dispatchEvent(pasteEvent);
    },
    { fileName: name, bytesBase64: PNG_BASE64 },
  );
  return uploadResponse;
}

export async function expectUploadScope(page: Page, workspaceId: string) {
  await expect
    .poll(() =>
      page.evaluate(() => window.sessionStorage.getItem("e2e.attachment-upload-workspace-id")),
    )
    .toBe(workspaceId);
}

export async function waitForReadyDraftAttachment(page: Page, sessionId: string, fileName: string) {
  await expect
    .poll(
      async () =>
        page.evaluate(
          ({ currentSessionId, currentFileName }) => {
            const raw = sessionStorage.getItem(`kandev.chatDraft.attachments.${currentSessionId}`);
            if (!raw) return false;
            try {
              const attachments = JSON.parse(raw) as Array<{
                attachmentId?: string;
                fileName?: string;
              }>;
              return attachments.some(
                (attachment) =>
                  attachment.fileName === currentFileName && Boolean(attachment.attachmentId),
              );
            } catch {
              return false;
            }
          },
          { currentSessionId: sessionId, currentFileName: fileName },
        ),
      { timeout: 15_000, message: `Wait for uploaded draft file ${fileName}` },
    )
    .toBe(true);
}

export async function waitForUserImageAttachment(
  apiClient: ApiClient,
  sessionId: string,
  marker: string,
  fileName: string,
) {
  let attachment: Record<string, unknown> | undefined;
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        const message = messages.find(
          (candidate) => candidate.author_type === "user" && candidate.content.includes(marker),
        );
        const attachments = message?.metadata?.attachments;
        attachment = Array.isArray(attachments)
          ? (attachments.find(
              (candidate) =>
                candidate &&
                typeof candidate === "object" &&
                "name" in candidate &&
                candidate.name === fileName,
            ) as Record<string, unknown> | undefined)
          : undefined;
        return Boolean(attachment);
      },
      { timeout: 20_000, message: `Wait for ${fileName} on the submitted task message` },
    )
    .toBe(true);
  return attachment;
}

export function writePngFile(path: string) {
  const bytes = Buffer.from(PNG_BASE64, "base64");
  fs.writeFileSync(path, bytes);
}
