import { test, expect } from "../../fixtures/test-base";
import {
  LARGE_FILE_TREE_COUNT,
  LARGE_FILE_TREE_FOLDER,
  largeFileTreePath,
  setupLargeFileTreeTask,
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
    await expect(folder).toBeVisible({ timeout: 15_000 });
    await folder.tap();
    await expect(session.fileTreeNode(largeFileTreePath(0))).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(() => session.visibleFileTreeNodes().count(), { timeout: 5_000 })
      .toBeLessThan(80);

    const firstActions = session.fileTreeNodeActions(largeFileTreePath(0));
    await expect(firstActions).toBeVisible();
    const firstActionsBox = await firstActions.boundingBox();
    expect(firstActionsBox).not.toBeNull();
    expect(firstActionsBox!.width).toBeGreaterThanOrEqual(44);
    expect(firstActionsBox!.height).toBeGreaterThanOrEqual(44);

    const viewport = session.fileTreeScrollViewport();
    await expect(viewport).toBeVisible();
    await viewport.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });

    const lastFile = largeFileTreePath(LARGE_FILE_TREE_COUNT - 1);
    await expect(session.fileTreeNode(lastFile)).toBeVisible({ timeout: 15_000 });
    const lastActions = session.fileTreeNodeActions(lastFile);
    await expect(lastActions).toBeVisible();
    const lastActionsBox = await lastActions.boundingBox();
    expect(lastActionsBox).not.toBeNull();
    expect(lastActionsBox!.width).toBeGreaterThanOrEqual(44);
    expect(lastActionsBox!.height).toBeGreaterThanOrEqual(44);

    await session.fileTreeNode(lastFile).tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await expect(viewer).toContainText(lastFile);
  });
});
