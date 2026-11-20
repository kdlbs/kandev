import fs from "node:fs";
import path from "node:path";
import type { Locator } from "@playwright/test";
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
  const { session, sessionId, gateway, root, filePath, folderPath, fileName, repoPath, contents } =
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
    await expect(testPage.locator('[data-slot="context-menu-content"]')).toHaveCount(0);
    await expect(row).toHaveAttribute("data-selected", selected!);
    await expect(row).toHaveAttribute("data-active", active!);
  };
  await copyTreeNode(folderPath);
  await expect(session.fileTreeNode(`${folderPath}/nested.txt`)).toHaveCount(0);
  await copyTreeNode(filePath);
  await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0);

  const copyByKeyboard = async (row: Locator, expectedPath: string, captureFocus = false) => {
    const selected = await row.getAttribute("data-selected");
    const active = await row.getAttribute("data-active");
    await row.focus();
    await expect(row).toBeFocused();
    await testPage.keyboard.press("Shift+Tab");
    await testPage.keyboard.press("Tab");
    await expect(row).toBeFocused();
    if (captureFocus) {
      await prCapture.screenshot("desktop-file-tree-keyboard-focus", {
        caption: "Keyboard focus on a Files tree row",
      });
    }
    await testPage.evaluate(() => navigator.clipboard.writeText("clipboard sentinel"));
    await testPage.keyboard.press("Shift+F10");
    const item = testPage.getByRole("menuitem", { name: "Copy path" });
    await expect(item).toBeVisible();
    await expect
      .poll(() =>
        testPage.evaluate(
          () => !!document.activeElement?.closest('[data-slot="context-menu-content"]'),
        ),
      )
      .toBe(true);
    await testPage.keyboard.press("Home");
    for (
      let index = 0;
      index < 12 && !(await item.evaluate((element) => element === document.activeElement));
      index++
    ) {
      await testPage.keyboard.press("ArrowDown");
    }
    await expect(item).toBeFocused();
    await testPage.keyboard.press("Enter");
    await expectClipboard(testPage, expectedPath);
    await expect(testPage.locator('[data-slot="context-menu-content"]')).toHaveCount(0);
    if (selected !== null) await expect(row).toHaveAttribute("data-selected", selected);
    if (active !== null) await expect(row).toHaveAttribute("data-active", active);
  };
  await copyByKeyboard(session.fileTreeNode(filePath), filePath, true);
  await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0);

  await session.fileSearchButton().click();
  const search = gateway.waitForResponse("workspace.files.search", {
    requestWhere: (payload) => payload.session_id === sessionId && payload.query === fileName,
  });
  await session.fileSearchInput().fill(fileName);
  await search;
  const result = session.fileSearchResult(filePath);
  await expect(result).toBeVisible();
  await copyByKeyboard(result, filePath);
  await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0);
  expect(fs.readFileSync(path.join(repoPath, fileName), "utf8")).toBe(contents);
});
