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
  viewport?: { width: number; height: number },
) {
  if (viewport) await testPage.setViewportSize(viewport);
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const directoryPath = `mobile-context-directory-${suffix}-with-a-long-name-that-must-truncate-in-the-file-tree-on-a-narrow-touch-viewport-with-an-extra-long-label-for-overflow-verification`;
  const filePath = `mobile-context-file-${suffix}.md`;
  const searchResultPrefix = `mobile-search-layout-${suffix}`;
  const searchResultPaths = [
    `${searchResultPrefix}-first-result.ts`,
    `${searchResultPrefix}-second-result.ts`,
  ];
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile(`${directoryPath}/nested.txt`, "mobile directory content\n");
  git.createFile(filePath, "mobile file content\n");
  searchResultPaths.forEach((searchResultPath) =>
    git.createFile(searchResultPath, "search result\n"),
  );
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
  return { session, directoryPath, filePath, searchResultPrefix };
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
    const { session, directoryPath, filePath, searchResultPrefix } = await setupMobileContextTask(
      testPage,
      apiClient,
      seedData,
      backend,
      { width: 393, height: 851 },
    );

    await testPage.getByRole("button", { name: "Files" }).tap();
    const directoryNode = session.fileTreeNode(directoryPath);
    await expect(directoryNode).toBeVisible({ timeout: 15_000 });

    const rowGeometry = await directoryNode.evaluate((node) => {
      const name = node.querySelector<HTMLElement>(":scope > span.min-w-0.flex-1");
      return {
        flexWrap: getComputedStyle(node).flexWrap,
        nameWhiteSpace: name ? getComputedStyle(name).whiteSpace : null,
        nameOverflow: name ? getComputedStyle(name).overflow : null,
        nameTextOverflow: name ? getComputedStyle(name).textOverflow : null,
        nameScrollWidth: name?.scrollWidth ?? null,
        nameClientWidth: name?.clientWidth ?? null,
      };
    });
    expect(rowGeometry.flexWrap).toBe("nowrap");
    expect(rowGeometry.nameWhiteSpace).toBe("nowrap");
    expect(rowGeometry.nameOverflow).toBe("hidden");
    expect(rowGeometry.nameTextOverflow).toBe("ellipsis");
    expect(rowGeometry.nameScrollWidth).not.toBeNull();
    expect(rowGeometry.nameClientWidth).not.toBeNull();
    expect(rowGeometry.nameScrollWidth).toBeGreaterThan(rowGeometry.nameClientWidth!);

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

    await session.fileSearchButton().tap();
    await session.fileSearchInput().fill(searchResultPrefix);
    const searchRows = testPage.locator('[data-testid="file-search-result"]:visible');
    await expect(searchRows).toHaveCount(2);
    const searchGeometry = await searchRows.evaluateAll((rows) =>
      rows.map((row) => {
        const trigger = row.querySelector<HTMLElement>('[data-testid="file-tree-node-actions"]');
        const name = row.querySelector<HTMLElement>(":scope > span.min-w-0.flex-1");
        const rowBox = row.getBoundingClientRect();
        const triggerBox = trigger?.getBoundingClientRect();
        return {
          actionBottom: triggerBox?.bottom ?? null,
          actionCenterX: triggerBox ? triggerBox.x + triggerBox.width / 2 : null,
          actionCenterY: triggerBox ? triggerBox.y + triggerBox.height / 2 : null,
          actionTop: triggerBox?.top ?? null,
          path: row.getAttribute("data-path"),
          rowBottom: rowBox.bottom,
          rowLeft: rowBox.left,
          rowRight: rowBox.right,
          rowTop: rowBox.top,
          nameOverflow: name ? getComputedStyle(name).overflow : null,
          nameTextOverflow: name ? getComputedStyle(name).textOverflow : null,
          nameScrollWidth: name?.scrollWidth ?? null,
          nameClientWidth: name?.clientWidth ?? null,
        };
      }),
    );
    for (const [index, geometry] of searchGeometry.entries()) {
      expect(geometry.actionCenterX).toBeGreaterThanOrEqual(geometry.rowLeft);
      expect(geometry.actionCenterX).toBeLessThanOrEqual(geometry.rowRight);
      expect(geometry.actionCenterY).toBeGreaterThanOrEqual(geometry.rowTop);
      expect(geometry.actionCenterY).toBeLessThanOrEqual(geometry.rowBottom);
      expect(geometry.actionTop).toBeGreaterThanOrEqual(geometry.rowTop);
      expect(geometry.actionBottom).toBeLessThanOrEqual(geometry.rowBottom);
      expect(geometry.path).toBeTruthy();
      expect(geometry.nameOverflow).toBe("hidden");
      expect(geometry.nameTextOverflow).toBe("ellipsis");
      expect(geometry.nameScrollWidth).not.toBeNull();
      expect(geometry.nameClientWidth).not.toBeNull();
      expect(geometry.nameScrollWidth).toBeGreaterThan(geometry.nameClientWidth!);
      if (index > 0) {
        expect(searchGeometry[index - 1].actionBottom).toBeLessThanOrEqual(geometry.actionTop);
      }
    }
    const firstSearchTrigger = searchRows.nth(0).getByTestId("file-tree-node-actions");
    const firstSearchTriggerBox = await firstSearchTrigger.boundingBox();
    expect(firstSearchTriggerBox).not.toBeNull();
    await firstSearchTrigger.tap({
      position: {
        x: firstSearchTriggerBox!.width / 2,
        y: firstSearchTriggerBox!.height - 1,
      },
    });
    await expect(session.fileTreeTouchMenu()).toBeVisible();
    await testPage.keyboard.press("Escape");
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
  test("keeps coarse desktop touch actions in separate 44px rows", async ({
    coarseDesktopTestPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    const { session, directoryPath, filePath } = await setupMobileContextTask(
      coarseDesktopTestPage,
      apiClient,
      seedData,
      backend,
    );

    const viewport = await coarseDesktopTestPage.evaluate(() => ({
      width: window.innerWidth,
      hasFinePointer: window.matchMedia("(pointer: fine)").matches,
    }));
    expect(viewport.width).toBeGreaterThanOrEqual(1024);
    expect(viewport.hasFinePointer).toBe(false);

    await session.clickTab("Files");
    const rows = [session.fileTreeNode(directoryPath), session.fileTreeNode(filePath)];
    for (const row of rows) {
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(async () => (await row.boundingBox())?.height ?? 0, { timeout: 2_000 })
        .toBeGreaterThanOrEqual(44);
    }

    const triggers = [
      session.fileTreeNodeActions(directoryPath),
      session.fileTreeNodeActions(filePath),
    ];
    await expect(triggers[0]).toBeVisible();
    await expect(triggers[1]).toBeVisible();
    const triggerBoxes = await Promise.all(triggers.map((trigger) => trigger.boundingBox()));
    expect(triggerBoxes[0]).not.toBeNull();
    expect(triggerBoxes[1]).not.toBeNull();
    expect(triggerBoxes[0]!.width).toBeGreaterThanOrEqual(44);
    expect(triggerBoxes[0]!.height).toBeGreaterThanOrEqual(44);
    expect(triggerBoxes[0]!.y + triggerBoxes[0]!.height).toBeLessThanOrEqual(triggerBoxes[1]!.y);

    await triggers[0].tap({
      position: {
        x: triggerBoxes[0]!.width / 2,
        y: triggerBoxes[0]!.height - 1,
      },
    });
    await expect(session.fileTreeTouchAddToChatContextItem()).toBeVisible();
    await session.fileTreeTouchAddToChatContextItem().tap();
    await session.clickSessionChatTab();
    await expect(session.chatContextFile(directoryPath)).toHaveCount(1);
  });
});
