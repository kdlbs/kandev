import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { openTaskSession, waitForLatestSessionDone } from "../../helpers/session";

test.describe("mobile message attachment layout", () => {
  test("keeps a file attachment compact next to an image preview", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile mixed attachment layout",
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

    const imagePath = path.join(testInfo.outputDir, "tiny.png");
    fs.copyFileSync(path.join(process.cwd(), "public/web-app-manifest-192x192.png"), imagePath);
    const textPath = path.join(testInfo.outputDir, "notes.txt");
    fs.writeFileSync(textPath, "plain text attachment");

    const fileInput = testPage.locator('input[type="file"]');
    await fileInput.setInputFiles(imagePath);
    await expect(testPage.getByText(/Image \(/).first()).toBeVisible({ timeout: 10_000 });
    await fileInput.setInputFiles(textPath);
    await session.sendMessageViaButton("send image and file together");
    await session.waitForChatIdle({ timeout: 30_000 });

    const fileAttachment = testPage.getByTestId("message-file-attachment", { exact: true });
    const imageAttachment = testPage.getByRole("button", { name: "Open Attachment 1" });
    await expect(fileAttachment).toHaveText("notes.txt");
    const [fileBox, imageBox] = await Promise.all([
      fileAttachment.boundingBox(),
      imageAttachment.boundingBox(),
    ]);

    expect(fileBox).not.toBeNull();
    expect(imageBox).not.toBeNull();
    expect(fileBox!.height).toBeLessThan(imageBox!.height);
  });
});
