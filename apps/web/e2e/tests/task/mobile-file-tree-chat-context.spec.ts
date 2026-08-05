import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

async function setupMobileContextTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
) {
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const directoryPath = `mobile-context-directory-${suffix}`;
  const filePath = `mobile-context-file-${suffix}.md`;
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile(`${directoryPath}/nested.txt`, "mobile directory content\n");
  git.createFile(filePath, "mobile file content\n");
  git.stageAll();
  git.commit(`add mobile chat context fixture ${suffix}`);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Mobile file tree chat context ${suffix}`,
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
  return { session, directoryPath, filePath };
}

test.describe("Mobile file tree chat context", () => {
  test("adds a directory through the visible touch menu and sends it from Chat", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const { session, directoryPath, filePath } = await setupMobileContextTask(
      testPage,
      apiClient,
      seedData,
      backend,
    );

    await testPage.getByRole("button", { name: "Files" }).tap();
    const directoryNode = session.fileTreeNode(directoryPath);
    await expect(directoryNode).toBeVisible({ timeout: 15_000 });

    const trigger = session.fileTreeNodeActions(directoryPath);
    await expect(trigger).toBeVisible();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);

    await trigger.tap();
    const menu = session.fileTreeTouchMenu();
    await expect(menu).toBeVisible();
    const menuBox = await menu.boundingBox();
    expect(menuBox).not.toBeNull();
    const viewport = await testPage.evaluate(() => ({
      width: window.innerWidth,
      height: window.innerHeight,
    }));
    expect(menuBox!.x).toBeGreaterThanOrEqual(0);
    expect(menuBox!.y).toBeGreaterThanOrEqual(0);
    expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(menuBox!.y + menuBox!.height).toBeLessThanOrEqual(viewport.height);

    const addItem = session.fileTreeTouchAddToChatContextItem();
    await expect(addItem).toBeVisible();
    await expect
      .poll(async () => (await addItem.boundingBox())?.height ?? 0, { timeout: 2_000 })
      .toBeGreaterThanOrEqual(44);
    const documentWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
    expect(documentWidth).toBeLessThanOrEqual(viewport.width);
    await prCapture.screenshot("mobile-file-tree-menu", {
      caption: "Pixel 5 Files row overflow menu with Add to chat context",
    });

    await addItem.tap();

    // Search results use the same visible touch action and session-bound handler.
    await session.fileSearchButton().tap();
    await session.fileSearchInput().fill(filePath);
    const searchResult = session.fileSearchResult(filePath);
    await expect(searchResult).toBeVisible({ timeout: 15_000 });
    const searchTrigger = session.fileTreeNodeActions(filePath);
    await expect(searchTrigger).toBeVisible();
    await searchTrigger.tap();
    await expect(session.fileTreeTouchAddToChatContextItem()).toBeVisible();
    await session.fileTreeTouchAddToChatContextItem().tap();
    await session.fileSearchInput().press("Escape");

    await expect(testPage.getByTestId("mobile-file-viewer-panel")).toHaveCount(0);
    await testPage.getByRole("button", { name: "Chat" }).tap();
    await expect(session.chatContextFile(directoryPath)).toHaveCount(1);
    await expect(session.chatContextFile(directoryPath)).toHaveAttribute(
      "data-is-directory",
      "true",
    );
    await prCapture.screenshot("mobile-chat-context", {
      caption: "Pixel 5 Chat composer with a folder context chip",
    });

    await expect(session.chatContextFile(filePath)).toHaveCount(1);
    await session.sendMessageViaButton("Please inspect this directory and file.");
    await expect(session.sentMessageContextFile(directoryPath)).toBeVisible({ timeout: 15_000 });
    await expect(session.sentMessageContextFile(filePath)).toBeVisible({ timeout: 15_000 });
    await expect(session.chatContextFile(directoryPath)).toHaveCount(0);
    await expect(session.chatContextFile(filePath)).toHaveCount(0);
  });
});
