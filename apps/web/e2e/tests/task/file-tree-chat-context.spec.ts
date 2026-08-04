import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

async function setupDesktopContextTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
) {
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const filePath = `context-file-${suffix}.md`;
  const directoryPath = `context-directory-${suffix}`;
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile(filePath, "# Context file\n");
  git.createFile(`${directoryPath}/nested.txt`, "directory content\n");
  git.stageAll();
  git.commit(`add chat context fixtures ${suffix}`);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `File tree chat context ${suffix}`,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return { session, filePath, directoryPath };
}

test.describe("File tree chat context", () => {
  test("adds files and directories from the desktop context menu and clears them after send", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const { session, filePath, directoryPath } = await setupDesktopContextTask(
      testPage,
      apiClient,
      seedData,
      backend,
    );

    await session.clickTab("Files");
    await expect(session.fileTreeNode(filePath)).toBeVisible({ timeout: 15_000 });
    await expect(session.fileTreeNode(directoryPath)).toBeVisible({ timeout: 15_000 });

    const addNodeToContext = async (nodePath: string) => {
      await session.fileTreeNode(nodePath).click({ button: "right" });
      await expect(session.fileTreeAddToChatContextMenuItem()).toBeVisible();
      await session.fileTreeAddToChatContextMenuItem().click();
    };

    await addNodeToContext(filePath);
    await session.fileTreeNode(directoryPath).click({ button: "right" });
    await expect(session.fileTreeAddToChatContextMenuItem()).toBeVisible();
    await prCapture.screenshot("desktop-file-tree-menu", {
      caption: "Desktop Files tree context menu with Add to chat context",
    });
    await session.fileTreeAddToChatContextMenuItem().click();
    await addNodeToContext(directoryPath);

    // The secondary action must not open a file or expand a directory.
    await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0);
    await expect(session.fileTreeNode(`${directoryPath}/nested.txt`)).toHaveCount(0);
    await session.clickSessionChatTab();
    await expect(session.chatContextFile(filePath)).toHaveCount(1);
    await expect(session.chatContextFile(directoryPath)).toHaveCount(1);
    await expect(session.chatContextFile(directoryPath)).toHaveAttribute(
      "data-is-directory",
      "true",
    );
    await prCapture.screenshot("desktop-chat-context", {
      caption: "Desktop Chat composer with file and directory context chips",
    });

    await session.sendMessage("Please inspect the attached file and directory.");
    await expect(session.sentMessageContextFile(filePath)).toBeVisible({ timeout: 15_000 });
    await expect(session.sentMessageContextFile(directoryPath)).toBeVisible({ timeout: 15_000 });
    await expect(session.chatContextFile(filePath)).toHaveCount(0);
    await expect(session.chatContextFile(directoryPath)).toHaveCount(0);
  });
});
