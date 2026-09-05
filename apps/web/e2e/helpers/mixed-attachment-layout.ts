import fs from "node:fs";
import path from "node:path";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { openTaskSession, waitForLatestSessionDone } from "./session";

type MixedAttachmentLayoutOptions = {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  testInfo: TestInfo;
  taskTitle: string;
};

export async function assertMixedAttachmentLayout({
  testPage,
  apiClient,
  seedData,
  testInfo,
  taskTitle,
}: MixedAttachmentLayoutOptions): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    taskTitle,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for mixed attachment task");
  const session = await openTaskSession(testPage, task.id);
  await session.waitForChatIdle({ timeout: 30_000 });
  fs.mkdirSync(testInfo.outputDir, { recursive: true });

  const imagePath = path.join(testInfo.outputDir, "mixed-attachment.png");
  fs.copyFileSync(path.join(process.cwd(), "public/web-app-manifest-192x192.png"), imagePath);
  const textPath = path.join(testInfo.outputDir, "notes.txt");
  fs.writeFileSync(textPath, "plain text attachment");

  const fileInput = testPage.getByTestId("chat-input-editor-shell").locator('input[type="file"]');
  // The chat input appends each change event to React attachment state.
  await fileInput.setInputFiles(imagePath);
  const imagePreview = testPage.getByText(/Image \(/);
  await expect(imagePreview).toHaveCount(1, { timeout: 10_000 });
  await expect(imagePreview).toBeVisible();
  // This second change event appends the text file, preserving both attachments.
  await fileInput.setInputFiles(textPath);
  await session.sendMessageViaButton("send image and file together");
  await session.waitForChatIdle({ timeout: 30_000 });

  const fileAttachment = testPage.getByTestId("message-file-attachment", { exact: true });
  const imageAttachment = testPage.getByRole("button", { name: "Open Attachment 1" });
  await expect(fileAttachment).toContainText("notes.txt");
  const [fileBox, imageBox] = await Promise.all([
    fileAttachment.boundingBox(),
    imageAttachment.boundingBox(),
  ]);

  expect(fileBox).not.toBeNull();
  expect(imageBox).not.toBeNull();
  expect(fileBox!.height).toBeLessThan(imageBox!.height);
}
