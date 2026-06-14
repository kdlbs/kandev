import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { openTaskSession, waitForLatestSessionDone } from "../../helpers/session";

type PersistedAttachment = {
  type?: string;
  name?: string;
  mime_type?: string;
  delivery_mode?: string;
};

async function waitForUserAttachment(
  apiClient: ApiClient,
  sessionId: string,
  content: string,
): Promise<PersistedAttachment> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        const message = messages.find(
          (m) => m.author_type === "user" && m.content.includes(content),
        );
        const attachments = message?.metadata?.attachments;
        return Array.isArray(attachments) ? attachments.length : 0;
      },
      { timeout: 15_000, message: `Wait for persisted attachment on "${content}"` },
    )
    .toBeGreaterThan(0);

  const { messages } = await apiClient.listSessionMessages(sessionId);
  const message = messages.find((m) => m.author_type === "user" && m.content.includes(content));
  const attachments = message?.metadata?.attachments;
  if (!Array.isArray(attachments)) throw new Error("Persisted user message has no attachments");
  return attachments[0] as PersistedAttachment;
}

test.describe("chat attachment delivery mode", () => {
  test("shows a Prompt/File selector for supported images and sends unsupported files by path", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Attachment delivery selector",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for initial attachment task");
    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 30_000 });
    fs.mkdirSync(testInfo.outputDir, { recursive: true });

    const imagePath = path.join(testInfo.outputDir, "tiny.png");
    fs.writeFileSync(
      imagePath,
      Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/6X3pZQAAAAASUVORK5CYII=",
        "base64",
      ),
    );

    await testPage.locator('input[type="file"]').setInputFiles(imagePath);
    const imageChip = testPage.getByText(/Image \(/).first();
    await expect(imageChip).toBeVisible({ timeout: 10_000 });
    await imageChip.hover();

    const promptButton = testPage.getByTestId("attachment-delivery-prompt");
    const pathButton = testPage.getByTestId("attachment-delivery-path");
    await expect(promptButton).toBeVisible({ timeout: 10_000 });
    await expect(pathButton).toBeVisible();
    await expect(promptButton).toHaveAttribute("data-selected", "true");

    await pathButton.click();
    await expect(pathButton).toHaveAttribute("data-selected", "true");
    await session.sendMessageViaButton("send image as file path");
    const imageAttachment = await waitForUserAttachment(
      apiClient,
      task.session_id,
      "send image as file path",
    );
    expect(imageAttachment).toMatchObject({
      type: "image",
      name: "tiny.png",
      mime_type: "image/png",
      delivery_mode: "path",
    });

    await session.waitForChatIdle({ timeout: 30_000 });
    const textPath = path.join(testInfo.outputDir, "notes.txt");
    fs.writeFileSync(textPath, "plain text attachment");

    await testPage.locator('input[type="file"]').setInputFiles(textPath);
    await expect(testPage.getByText("notes.txt", { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await testPage.getByText("notes.txt", { exact: true }).hover();
    await expect(testPage.getByTestId("attachment-delivery-prompt")).toHaveCount(0);
    await expect(testPage.getByTestId("attachment-delivery-path")).toHaveCount(0);

    await session.sendMessageViaButton("send unsupported file");
    const textAttachment = await waitForUserAttachment(
      apiClient,
      task.session_id,
      "send unsupported file",
    );
    expect(textAttachment).toMatchObject({
      type: "resource",
      name: "notes.txt",
      mime_type: "text/plain",
      delivery_mode: "path",
    });
  });
});
