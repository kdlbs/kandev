import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  LARGE_FILE_TREE_COUNT,
  LARGE_FILE_TREE_FOLDER,
  expectContiguousVisibleFileTreeRows,
  expectVisibleFileTreePaths,
  largeFileTreePath,
  scrollToLastLargeFile,
  setupLargeFileTreeTask,
  visibleFileTreePaths,
  waitForFileTreeLayoutSettle,
} from "./large-file-tree-virtualization-helpers";

test.describe("Mobile large file tree virtualization", () => {
  test("keeps the row window bounded and preserves touch actions", async ({
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
      title: "Mobile large file tree virtualization",
    });

    await testPage.getByRole("button", { name: "Files" }).tap();
    const folder = session.fileTreeNode(LARGE_FILE_TREE_FOLDER);
    const viewport = session.fileTreeScrollViewport();
    await expect(folder).toBeVisible({ timeout: 15_000 });
    await expect(viewport).toBeVisible({ timeout: 15_000 });
    await waitForFileTreeLayoutSettle(testPage);
    await expectContiguousVisibleFileTreeRows(viewport);
    const collapsedPaths = await visibleFileTreePaths(viewport);

    await testPage.getByRole("button", { name: "Chat" }).tap();
    await expect(viewport).toBeHidden();
    await testPage.getByRole("button", { name: "Files" }).tap();
    await expect(folder).toBeVisible({ timeout: 15_000 });
    await expectContiguousVisibleFileTreeRows(viewport);
    await expectVisibleFileTreePaths(viewport, collapsedPaths);

    await folder.tap();
    await expect(session.fileTreeNode(largeFileTreePath(0))).toBeVisible({ timeout: 15_000 });
    await expectContiguousVisibleFileTreeRows(viewport);
    await expect
      .poll(() => session.visibleFileTreeNodes().count(), { timeout: 5_000 })
      .toBeLessThan(80);

    const firstActions = session.fileTreeNodeActions(largeFileTreePath(0));
    await expect(firstActions).toBeVisible();
    const firstActionsBox = await firstActions.boundingBox();
    expect(firstActionsBox).not.toBeNull();
    expect(firstActionsBox!.width).toBeGreaterThanOrEqual(44);
    expect(firstActionsBox!.height).toBeGreaterThanOrEqual(44);

    await scrollToLastLargeFile(
      session.fileTreeNode(largeFileTreePath(LARGE_FILE_TREE_COUNT - 1)),
      viewport,
    );

    const lastFile = largeFileTreePath(LARGE_FILE_TREE_COUNT - 1);
    await expect(session.fileTreeNode(lastFile)).toBeVisible({ timeout: 15_000 });
    const lastActions = session.fileTreeNodeActions(lastFile);
    await expect(lastActions).toBeVisible();
    const lastActionsBox = await lastActions.boundingBox();
    expect(lastActionsBox).not.toBeNull();
    expect(lastActionsBox!.width).toBeGreaterThanOrEqual(44);
    expect(lastActionsBox!.height).toBeGreaterThanOrEqual(44);
    await expectContiguousVisibleFileTreeRows(viewport);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile large file tree");

    await session.fileTreeNode(lastFile).tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await expect(viewer).toContainText(lastFile);
  });
});
