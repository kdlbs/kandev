import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import {
  expectStoredMarkdownComment,
  MARKDOWN_SELECTION_CONTENT,
  MARKDOWN_LAST_SELECTION_SENTINEL,
  openMarkdownPreviewForSelectionComment,
  selectMarkdownPreviewTextWithTouch,
} from "../../helpers/markdown-preview-comments";
import {
  restoreSelectionCommentVisualViewport,
  setSelectionCommentVisualViewport,
} from "../../helpers/selection-comment-visual-viewport";

const SELECTED_TEXT = "Second source paragraph for a comment.";
const COMMENT_TEXT = "Clarify this source paragraph.";

test.describe("Markdown selection comments on mobile", () => {
  test("shows Comment after a touch selection is captured", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fileName = `mobile-long-press-selection-${Date.now()}.md`;
    const { preview, diskPath } = await openMarkdownPreviewForSelectionComment({
      page: testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      title: "Mobile Markdown Long Press Selection",
    });

    try {
      const paragraph = preview.getByTestId("markdown-preview-source-block").filter({
        hasText: SELECTED_TEXT,
      });
      await expect(paragraph).toBeVisible();
      await selectMarkdownPreviewTextWithTouch(paragraph, SELECTED_TEXT);
      const selectedText = await testPage.evaluate(() => window.getSelection()?.toString() ?? "");
      expect(selectedText.trim()).toBe(SELECTED_TEXT);
      const action = testPage.getByTestId("markdown-preview-comment-button");
      await expect(action).toBeVisible({ timeout: 5_000 });
      await action.tap();
      const commentDialog = testPage.getByRole("dialog").filter({
        has: testPage.getByPlaceholder("Add your comment or instruction..."),
      });
      await expect(commentDialog).toBeVisible();
      await expect(commentDialog).toContainText(selectedText.trim());
    } finally {
      fs.rmSync(diskPath, { force: true });
    }
  });

  test("docks touch selection and creates a pending comment with its source lines", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(90_000);
    const fileName = `mobile-selection-comment-${Date.now()}.md`;
    const { session, sessionId, preview, diskPath } = await openMarkdownPreviewForSelectionComment({
      page: testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      title: "Mobile Markdown Selection Comment",
    });

    const paragraph = preview.getByTestId("markdown-preview-source-block").filter({
      hasText: SELECTED_TEXT,
    });
    await expect(paragraph).toBeVisible();
    await selectMarkdownPreviewTextWithTouch(paragraph, SELECTED_TEXT);

    const dock = testPage.getByTestId("selection-action-dock");
    const action = testPage.getByTestId("markdown-preview-comment-button");
    await expect(action).toBeVisible({ timeout: 5_000 });
    const [dockBox, ownerBox, actionBox] = await Promise.all([
      dock.boundingBox(),
      preview.locator(".flex-1.overflow-auto").boundingBox(),
      action.boundingBox(),
    ]);
    expect(dockBox).not.toBeNull();
    expect(ownerBox).not.toBeNull();
    expect(actionBox).not.toBeNull();
    if (!dockBox || !ownerBox || !actionBox) throw new Error("Expected touch dock geometry");
    expect(actionBox.height).toBeGreaterThanOrEqual(44);
    expect(actionBox.width).toBeGreaterThanOrEqual(44);
    expect(dockBox.x).toBeGreaterThanOrEqual(ownerBox.x - 1);
    expect(dockBox.x + dockBox.width).toBeLessThanOrEqual(ownerBox.x + ownerBox.width + 1);
    expect(dockBox.y + dockBox.height).toBeLessThanOrEqual(ownerBox.y + ownerBox.height + 1);
    const reservedPadding = await preview
      .locator(".flex-1.overflow-auto")
      .evaluate((owner) => Number.parseFloat(window.getComputedStyle(owner).paddingBottom));
    expect(reservedPadding).toBeGreaterThanOrEqual(56);

    const owner = preview.locator(".flex-1.overflow-auto");
    const beforeViewportChange = await owner.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
    });
    await setSelectionCommentVisualViewport(testPage, {
      height: Math.max(320, Math.floor(beforeViewportChange.innerHeight * 0.62)),
      offsetTop: 24,
    });
    const clippedViewport = Math.max(320, Math.floor(beforeViewportChange.innerHeight * 0.62));
    const afterViewportChange = await owner.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
    });
    expect(afterViewportChange.innerHeight).toBe(beforeViewportChange.innerHeight);
    expect(afterViewportChange.top).toBeCloseTo(beforeViewportChange.top, 1);
    expect(afterViewportChange.bottom).toBeCloseTo(beforeViewportChange.bottom, 1);
    await expect
      .poll(async () => {
        const clippedDockBox = await dock.boundingBox();
        return Boolean(
          clippedDockBox &&
          clippedDockBox.y >= 24 - 1 &&
          clippedDockBox.y + clippedDockBox.height <= 24 + clippedViewport + 1,
        );
      })
      .toBe(true);
    const finalParagraph = preview.getByText(MARKDOWN_LAST_SELECTION_SENTINEL, { exact: true });
    await owner.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await finalParagraph.scrollIntoViewIfNeeded();
    const [clippedDockBox, finalParagraphBox] = await Promise.all([
      dock.boundingBox(),
      finalParagraph.boundingBox(),
    ]);
    expect(clippedDockBox).not.toBeNull();
    expect(finalParagraphBox).not.toBeNull();
    if (!clippedDockBox || !finalParagraphBox) throw new Error("Expected clipped preview geometry");
    expect(finalParagraphBox.y + finalParagraphBox.height).toBeLessThanOrEqual(
      clippedDockBox.y + 1,
    );

    await testPage.screenshot({ path: testInfo.outputPath("markdown-selection-comment-dock.png") });
    await restoreSelectionCommentVisualViewport(testPage);
    await action.tap();

    const commentDialog = testPage.getByRole("dialog").filter({
      has: testPage.getByPlaceholder("Add your comment or instruction..."),
    });
    await expect(commentDialog).toBeVisible();
    await expect(commentDialog).toContainText(SELECTED_TEXT);
    await expect
      .poll(async () => {
        const box = await commentDialog.boundingBox();
        const size = testPage.viewportSize();
        return Boolean(
          box &&
          size &&
          box.x >= 0 &&
          box.y >= 0 &&
          box.x + box.width <= size.width + 1 &&
          box.y + box.height <= size.height + 1,
        );
      })
      .toBe(true);
    const dialogBox = await commentDialog.boundingBox();
    const viewport = testPage.viewportSize();
    expect(dialogBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    if (!dialogBox || !viewport) throw new Error("Expected mobile comment editor geometry");
    expect(dialogBox.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox.x + dialogBox.width).toBeLessThanOrEqual(viewport.width + 1);
    expect(dialogBox.y + dialogBox.height).toBeLessThanOrEqual(viewport.height + 1);

    await commentDialog.getByPlaceholder("Add your comment or instruction...").fill(COMMENT_TEXT);
    await commentDialog.getByRole("button", { name: "Add", exact: true }).tap();
    await expect(preview.getByTestId("markdown-preview-commented-range")).toBeVisible();
    await expectStoredMarkdownComment({
      page: testPage,
      sessionId,
      fileName,
      quote: SELECTED_TEXT,
      commentText: COMMENT_TEXT,
      startLine: 5,
      endLine: 5,
      repositoryId: seedData.repositoryId,
    });

    await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
    await expect(session.activeChat().getByText(`${fileName} (1)`, { exact: true })).toBeVisible();
    expect(fs.readFileSync(diskPath, "utf8")).toBe(MARKDOWN_SELECTION_CONTENT);
    await restoreSelectionCommentVisualViewport(testPage);
  });
});
