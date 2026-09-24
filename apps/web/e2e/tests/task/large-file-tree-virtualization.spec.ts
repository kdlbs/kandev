import { test, expect } from "../../fixtures/test-base";
import {
  LARGE_FILE_TREE_FOLDER,
  LARGE_FILE_TREE_COUNT,
  expectContiguousVisibleFileTreeRows,
  expectVisibleFileTreePaths,
  largeFileTreePath,
  scrollToLastLargeFile,
  setupLargeFileTreeTask,
  visibleFileTreePaths,
  waitForFileTreeLayoutSettle,
} from "./large-file-tree-virtualization-helpers";

test.describe("Large file tree virtualization", () => {
  test("mounts a bounded row window and reaches the last file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const session = await setupLargeFileTreeTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "Large file tree virtualization",
    });

    await session.clickTab("Files");
    const folder = await session.fileTree.waitForFileTreeNode(LARGE_FILE_TREE_FOLDER);
    await expect(session.fileTreeNode(largeFileTreePath(0))).toHaveCount(0);

    await folder.click();
    await session.fileTree.waitForFileTreeNode(largeFileTreePath(0));
    await expect
      .poll(() => session.visibleFileTreeNodes().count(), { timeout: 5_000 })
      .toBeLessThan(80);

    const viewport = session.fileTreeScrollViewport();
    await expect(viewport).toBeVisible();
    await scrollToLastLargeFile(
      session.fileTreeNode(largeFileTreePath(LARGE_FILE_TREE_COUNT - 1)),
      viewport,
    );

    const lastFile = largeFileTreePath(LARGE_FILE_TREE_COUNT - 1);
    await expect(session.fileTreeNode(lastFile)).toBeVisible({ timeout: 15_000 });
    await session.fileTreeNode(lastFile).click();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 15_000 });
  });

  test("reveals the create input after scrolling away from its folder", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const session = await setupLargeFileTreeTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "Large file tree create reveal",
    });

    await session.clickTab("Files");
    const folder = await session.fileTree.waitForFileTreeNode(LARGE_FILE_TREE_FOLDER);
    await folder.click();
    await session.fileTree.waitForFileTreeNode(largeFileTreePath(0));

    const viewport = session.fileTreeScrollViewport();
    await viewport.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });

    const createMenu = testPage.getByTestId("files-create-menu");
    await expect(createMenu).toBeVisible({ timeout: 15_000 });
    await createMenu.click();
    await testPage.getByRole("menuitem", { name: "New file" }).click();
    const input = testPage.getByPlaceholder("filename...");
    await expect(input).toBeVisible({ timeout: 15_000 });
    await expect(input).toBeFocused({ timeout: 5_000 });
  });

  test("restores contiguous rows after Files is hidden and reopened", async ({
    apiClient,
    seedData,
    backend,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const session = await setupLargeFileTreeTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "Large file tree restoration",
    });

    await session.clickTab("Files");
    const folder = await session.fileTree.waitForFileTreeNode(LARGE_FILE_TREE_FOLDER);
    const viewport = session.fileTreeScrollViewport();
    await expect(viewport).toBeVisible({ timeout: 15_000 });
    await waitForFileTreeLayoutSettle(testPage);
    await expectContiguousVisibleFileTreeRows(viewport);
    const collapsedPaths = await visibleFileTreePaths(viewport);

    await session.clickTab("Changes");
    await expect(session.files).toBeHidden();
    await waitForFileTreeLayoutSettle(testPage);
    await session.clickTab("Files");
    await expect(viewport).toBeVisible({ timeout: 15_000 });
    await session.fileTree.waitForFileTreeNode(LARGE_FILE_TREE_FOLDER);
    await expectContiguousVisibleFileTreeRows(viewport);
    await expectVisibleFileTreePaths(viewport, collapsedPaths);

    await folder.click();
    const firstFile = largeFileTreePath(0);
    await session.fileTree.waitForFileTreeNode(firstFile);
    await expectContiguousVisibleFileTreeRows(viewport);
    const expandedTopPaths = await visibleFileTreePaths(viewport);

    await session.clickTab("Changes");
    await expect(session.files).toBeHidden();
    await waitForFileTreeLayoutSettle(testPage);
    await session.clickTab("Files");
    await session.fileTree.waitForFileTreeNode(firstFile);
    await expectContiguousVisibleFileTreeRows(viewport);
    await expectVisibleFileTreePaths(viewport, expandedTopPaths);

    const lastFile = largeFileTreePath(LARGE_FILE_TREE_COUNT - 1);
    await scrollToLastLargeFile(session.fileTreeNode(lastFile), viewport);
    await expect(session.fileTreeNode(lastFile)).toBeVisible({ timeout: 15_000 });
    await expectContiguousVisibleFileTreeRows(viewport);
    const scrolledPaths = await visibleFileTreePaths(viewport);

    await session.clickTab("Changes");
    await expect(session.files).toBeHidden();
    await waitForFileTreeLayoutSettle(testPage);
    await session.clickTab("Files");
    await expect(session.fileTreeNode(lastFile)).toBeVisible({ timeout: 15_000 });
    await expectContiguousVisibleFileTreeRows(viewport);
    await expectVisibleFileTreePaths(viewport, scrolledPaths);
  });
});
