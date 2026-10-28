import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { expectClipboard, setupPathCopyTask } from "./file-tree-path-copy-helpers";

test("copies multi-root Files paths from desktop without activating rows", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const { session, root, filePath, folderPath, fileName, repoPath, contents } =
    await setupPathCopyTask(testPage, apiClient, seedData, backend);
  await session.clickTab("Files");
  await session.fileTree.waitForFileTreeNode(root);
  await session.fileTreeNode(root).click();
  await session.fileTree.waitForFileTreeNode(folderPath);
  await session.fileTree.waitForFileTreeNode(filePath);

  const copyTreeNode = async (nodePath: string) => {
    const row = await session.fileTree.waitForFileTreeNode(nodePath);
    const selected = await row.getAttribute("data-selected");
    const active = await row.getAttribute("data-active");
    await row.click({ button: "right" });
    const item = testPage.getByRole("menuitem", { name: "Copy path" });
    await expect(item).toBeVisible();
    if (nodePath === folderPath) {
      await item.evaluate(async (element) => {
        await Promise.all(
          element
            .closest('[data-slot="context-menu-content"]')!
            .getAnimations({ subtree: true })
            .map((animation) => animation.finished),
        );
      });
      await prCapture.screenshot("desktop-file-tree-copy-menu", {
        caption: "Files context menu with Copy path",
      });
    }
    await item.click();
    await expectClipboard(testPage, nodePath);
    await expect(row).toHaveAttribute("data-selected", selected!);
    await expect(row).toHaveAttribute("data-active", active!);
  };
  await copyTreeNode(folderPath);
  await expect(session.fileTreeNode(`${folderPath}/nested.txt`)).toHaveCount(0);
  await copyTreeNode(filePath);
  await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0);

  await session.fileSearchButton().click();
  await session.fileSearchInput().fill(fileName);
  const result = session.fileSearchResult(filePath);
  await expect(result).toBeVisible({ timeout: 30_000 });
  await result.click({ button: "right" });
  await testPage.getByRole("menuitem", { name: "Copy path" }).click();
  await expectClipboard(testPage, filePath);
  await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0);
  expect(fs.readFileSync(path.join(repoPath, fileName), "utf8")).toBe(contents);
});
