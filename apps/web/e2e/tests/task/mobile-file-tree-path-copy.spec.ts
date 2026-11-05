import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { expectClipboard, setupPathCopyTask } from "./file-tree-path-copy-helpers";

test("copies multi-root Files paths from the visible phone row menu", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const { session, sessionId, gateway, root, filePath, folderPath, fileName, repoPath, contents } =
    await setupPathCopyTask(testPage, apiClient, seedData, backend);
  await testPage.getByRole("button", { name: "Files", exact: true }).tap();
  await session.fileTree.waitForFileTreeNode(root);
  await session.fileTreeNode(root).tap();
  await session.fileTree.waitForFileTreeNode(folderPath);
  await session.fileTree.waitForFileTreeNode(filePath);

  const copyNode = async (nodePath: string, search = false) => {
    const row = search
      ? session.fileSearchResult(nodePath)
      : await session.fileTree.waitForFileTreeNode(nodePath);
    const selected = search ? null : await row.getAttribute("data-selected");
    const trigger = session.fileTreeNodeActions(nodePath);
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    await trigger.tap();
    const menu = session.fileTreeTouchMenu();
    const item = menu.getByRole("menuitem", { name: "Copy path" });
    await expect(item).toBeVisible();
    await expect
      .poll(async () => (await item.boundingBox())?.height ?? 0, { timeout: 2_000 })
      .toBeGreaterThanOrEqual(44);
    const menuBox = await menu.boundingBox();
    expect(menuBox).not.toBeNull();
    const viewport = testPage.viewportSize()!;
    expect(menuBox!.x).toBeGreaterThanOrEqual(0);
    expect(menuBox!.y).toBeGreaterThanOrEqual(0);
    expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(menuBox!.y + menuBox!.height).toBeLessThanOrEqual(viewport.height);
    if (nodePath === folderPath && !search) {
      await menu.evaluate(async (element) => {
        await Promise.all(
          element.getAnimations({ subtree: true }).map((animation) => animation.finished),
        );
      });
      await prCapture.screenshot("mobile-file-tree-copy-menu", {
        caption: "Files row menu with Copy path",
      });
    }
    await item.tap();
    await expectClipboard(testPage, nodePath);
    if (!search) await expect(row).toHaveAttribute("data-selected", selected!);
  };
  await copyNode(folderPath);
  await expect(session.fileTreeNode(`${folderPath}/nested.txt`)).toHaveCount(0);
  await copyNode(filePath);
  await expect(testPage.getByTestId("mobile-file-viewer-panel")).toHaveCount(0);
  await session.fileSearchButton().tap();
  const search = gateway.waitForResponse("workspace.files.search", {
    requestWhere: (payload) => payload.session_id === sessionId && payload.query === fileName,
  });
  await session.fileSearchInput().fill(fileName);
  await search;
  await expect(session.fileSearchResult(filePath)).toBeVisible();
  await testPage.evaluate(() => navigator.clipboard.writeText("clipboard sentinel"));
  await copyNode(filePath, true);
  await expect(testPage.getByTestId("mobile-file-viewer-panel")).toHaveCount(0);
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  expect(fs.readFileSync(path.join(repoPath, fileName), "utf8")).toBe(contents);
});
